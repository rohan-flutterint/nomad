package dns

import (
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/miekg/dns"
)

type Server struct {
	domain string
	logger hclog.Logger

	//
	srv *dns.Server
	mux *dns.ServeMux

	// rpc
	rpc rpcFunc

	//
	region string

	//
	cfg atomic.Value

	// selfNS
	selfNS string
}

type Config struct {
	BindAddr    string
	BindPort    uint
	BindNetwork string
	Domain      string

	EnabledTaskResolution bool

	DisableCompression bool

	EnableRecusor bool
	RecursorAddrs []string

	AllowStale bool
}

// rpcFunc is the function signature that the agent.RPC() function uses and is
// only used to keep the NewServer args short.
type rpcFunc func(string, interface{}, interface{}) error

func NewServer(log hclog.Logger, region, name string, rpcFunc rpcFunc) (*Server, error) {

	dnsCfg := &Config{
		BindAddr:           "10.0.2.15",
		BindPort:           53,
		BindNetwork:        "udp",
		Domain:             "nomad.",
		EnableRecusor:      true,
		DisableCompression: false,
		RecursorAddrs:      []string{"1.1.1.1:53", "8.8.8.8:53"},
		AllowStale:         false,
	}

	srv := &Server{
		domain: dns.Fqdn(strings.ToLower(dnsCfg.Domain)),
		logger: log.Named("dns"),
		region: region,
		rpc:    rpcFunc,
		selfNS: dns.Fqdn(strings.ToLower(fmt.Sprintf("%s.node.%s", name, dnsCfg.Domain))),
	}

	//
	srv.cfg.Store(dnsCfg)

	return srv, nil
}

func (s *Server) ListenAndServe() error {

	s.mux = dns.NewServeMux()
	s.mux.HandleFunc("arpa.", s.handlePtr)
	s.mux.HandleFunc(s.domain, s.handleQuery)

	cfg := s.cfg.Load().(*Config)

	if cfg.EnableRecusor {
		s.mux.HandleFunc(".", s.handleRecurse)
	}

	s.srv = &dns.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.BindAddr, cfg.BindPort),
		Net:     cfg.BindNetwork,
		Handler: s.mux,
	}
	if s.srv.Net == "udp" {
		s.srv.UDPSize = 65535
	}
	return s.srv.ListenAndServe()
}

func (s *Server) handleQuery(resp dns.ResponseWriter, req *dns.Msg) {

	q := req.Question[0]

	//
	t := time.Now()
	defer s.handleQueryObservability(q, resp.RemoteAddr(), t)

	cfg := s.cfg.Load().(*Config)

	// Setup the message response
	m := new(dns.Msg)
	m.SetReply(req)
	m.Compress = !cfg.DisableCompression
	m.Compress = false
	m.Authoritative = true
	m.RecursionAvailable = len(cfg.RecursorAddrs) > 0

	ecsGlobal := true

	switch req.Question[0].Qtype {
	case dns.TypeNS:
		m.Answer = s.generateNameservers()
		m.SetRcode(req, dns.RcodeSuccess)
	default:
		ecsGlobal = s.resolveQuery(cfg, req, m)
	}

	setEDNS(req, m, ecsGlobal)

	// Write out the complete response.
	if err := resp.WriteMsg(m); err != nil {
		s.logger.Warn("failed to write response", "error", err)
	}
}

func (s *Server) handleQueryObservability(q dns.Question, addr net.Addr, t time.Time) {
	s.logger.Debug("handled request",
		"name", q.Name,
		"type", dns.Type(q.Qtype),
		"class", dns.Class(q.Qclass),
		"latency", time.Since(t).String(),
		"client", addr.String(),
		"client_network", addr.Network(),
	)
}

func (s *Server) generateNameservers() []dns.RR {
	return []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   s.domain,
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    uint32(5 / time.Second),
			},
			Ns: s.selfNS,
		},
	}
}

func (s *Server) resolveQuery(cfg *Config, req, resp *dns.Msg) bool {

	// Normalise and remove the domain from the question.
	qName := strings.ToLower(dns.Fqdn(req.Question[0].Name))
	qName = strings.TrimSuffix(qName, "."+s.domain)

	// Split into the label parts so we can properly identify the question.
	labels := strings.Split(qName, ".")

	// Track whether we have identified what the query type is so we can
	// quickly move on and hand off the query to the correct function.
	var (
		done      bool
		queryType string
	)

	// Iterate backwards to identify what query we want to make.
	for i := len(labels) - 1; i >= 0 && !done; i-- {
		switch labels[i] {
		case "service", "node":
			queryType = labels[i]
			done = true
		}
	}

	if queryType == "" {
		s.logger.Error("unable to handle query type", "query_type", queryType)
		return false
	}

	switch queryType {
	case "service":
		return s.handleServiceQuery(cfg, qName, labels, req, resp)
	case "node":
		return s.handleNodeQuery(cfg, qName, labels, req, resp)
	}
	return true
}

func (s *Server) handleServiceQuery(cfg *Config, qName string, labels []string, req, resp *dns.Msg) bool {

	// A service query can be formed of 4 or 5 elements once the domain has
	// been stripped off. This is the same for both standard and RFC 2782 style
	// lookups.
	elems := len(labels)
	if elems < 4 || elems > 5 {
		s.logger.Error("incorrect number of elements in question", "num", elems)
		return false
	}

	var portLabel, jobID, groupName string

	if strings.HasPrefix(labels[1], "_") && strings.HasPrefix(labels[0], "_") {
		groupName = strings.TrimPrefix(labels[0], "_")
		portLabel = strings.TrimPrefix(labels[1], "_")
	} else {
		groupName = labels[1]
		portLabel = labels[0]
	}

	if groupName == "" || portLabel == "" {
		s.logger.Error("unable to identify service query format",
			"group_name", groupName, "port_label", portLabel)
		return false
	}

	jobID = labels[2]

	var allocs structs.JobAllocationsResponse

	args := structs.JobSpecificRequest{
		JobID: jobID,
		All:   false,
		QueryOptions: structs.QueryOptions{
			AllowStale: cfg.AllowStale,
			Region:     s.region,
		},
	}

	// If we have five elements in the question, we expect the forth entry to
	// be the region identifier.
	if elems == 5 {
		args.Region = labels[4]
	}

	if err := s.rpc("Job.Allocations", &args, &allocs); err != nil {
		s.logger.Error("failed to list job allocations", "error", err)
		return false
	}

	var found bool

	for _, alloc := range allocs.Allocations {

		if alloc.TaskGroup != groupName {
			continue
		}

		if !isAllocHealthy(alloc.DesiredStatus, alloc.ClientStatus) {
			continue
		}

		allocArgs := structs.AllocSpecificRequest{
			AllocID: alloc.ID,
			QueryOptions: structs.QueryOptions{
				AllowStale: cfg.AllowStale,
				Region:     args.Region,
			},
		}

		var out structs.SingleAllocResponse

		if err := s.rpc("Alloc.GetAlloc", &allocArgs, &out); err != nil {
			s.logger.Error("failed to read allocation", "error", err)
			return false
		}

		for _, groupNetwork := range out.Alloc.Resources.Networks {

			for _, groupPort := range groupNetwork.DynamicPorts {
				if groupPort.Label == portLabel {

					found = true

					switch req.Question[0].Qtype {
					case dns.TypeSRV:
						resp.Answer = append(resp.Answer, &dns.SRV{
							Hdr: dns.RR_Header{
								Name:   qName + "." + s.domain,
								Rrtype: dns.TypeSRV,
								Class:  dns.ClassINET,
								Ttl:    uint32(5 / time.Second),
							},
							Priority: 1,
							Weight:   100,
							Port:     uint16(groupPort.Value),
							Target:   qName + "." + s.domain,
						})
					default:
						resp.Answer = append(resp.Answer, &dns.A{
							Hdr: dns.RR_Header{
								Name:   qName + "." + s.domain,
								Rrtype: dns.TypeA,
								Class:  dns.ClassINET,
								Ttl:    uint32(5 / time.Second),
							},
							A: net.ParseIP(groupNetwork.IP),
						})
					}
				}
			}
		}
	}
	return found
}

func isAllocHealthy(desiredStatus, clientStatus string) bool {
	return desiredStatus == api.AllocDesiredStatusRun && clientStatus == api.AllocClientStatusRunning
}

func (s *Server) handleNodeQuery(cfg *Config, qName string, labels []string, req, resp *dns.Msg) bool {

	elems := len(labels)
	if elems < 2 || elems > 3 {
		s.logger.Error("incorrect number of elements in question", "num", elems)
		return false
	}

	args := structs.NodeListRequest{
		QueryOptions: structs.QueryOptions{
			AllowStale: cfg.AllowStale,
			Region:     s.region,
		},
	}
	if elems == 3 {
		args.Region = labels[2]
	}

	var nodes structs.NodeListResponse

	if err := s.rpc("Node.List", &args, &nodes); err != nil {
		s.logger.Error("failed to list nodes", "error", err)
		return false
	}

	for _, node := range nodes.Nodes {
		if strings.ToLower(node.Name) != labels[0] {
			continue
		}

		dnsRecord := &dns.A{
			Hdr: dns.RR_Header{
				Name:   qName + "." + s.domain,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    uint32(5 / time.Second),
			},
			A: net.ParseIP(node.Address),
		}

		resp.Answer = append(resp.Answer, dnsRecord)
		return true

	}
	return false
}

func (s *Server) handleRecurse(resp dns.ResponseWriter, req *dns.Msg) {

	q := req.Question[0]
	network := "udp"

	// Switch to TCP if the client is
	if _, ok := resp.RemoteAddr().(*net.TCPAddr); ok {
		network = "tcp"
	}

	cfg := s.cfg.Load().(*Config)

	// Recursively resolve
	c := &dns.Client{Net: network, Timeout: 5 * time.Second}
	var r *dns.Msg
	var rtt time.Duration
	var err error
	for _, recursor := range cfg.RecursorAddrs {
		r, rtt, err = c.Exchange(req, recursor)
		// Check if the response is valid and has the desired Response code
		if r != nil && (r.Rcode != dns.RcodeSuccess && r.Rcode != dns.RcodeNameError) {
			s.logger.Debug("recurse failed for question",
				"question", q,
				"rtt", rtt,
				"recursor", recursor,
				"rcode", dns.RcodeToString[r.Rcode],
			)
			// If we still have recursors to forward the query to,
			// we move forward onto the next one else the loop ends
			continue
		} else if err == nil || (r != nil && r.Truncated) {
			// Compress the response; we don't know if the incoming
			// response was compressed or not, so by not compressing
			// we might generate an invalid packet on the way out.
			r.Compress = false

			// Forward the response
			s.logger.Debug("recurse succeeded for question",
				"question", q,
				"rtt", rtt,
				"recursor", recursor,
			)
			if err := resp.WriteMsg(r); err != nil {
				s.logger.Warn("failed to respond", "error", err)
			}
			return
		}
		s.logger.Error("recurse failed", "error", err)
	}

	// If all resolvers fail, return a SERVFAIL message
	s.logger.Error("all resolvers failed for question from client",
		"question", q,
		"client", resp.RemoteAddr().String(),
		"client_network", resp.RemoteAddr().Network(),
	)
	m := &dns.Msg{}
	m.SetReply(req)
	m.Compress = false
	m.RecursionAvailable = true
	m.SetRcode(req, dns.RcodeServerFailure)
	if edns := req.IsEdns0(); edns != nil {
		setEDNS(req, m, true)
	}
	resp.WriteMsg(m)
}

func (s *Server) handlePtr(resp dns.ResponseWriter, req *dns.Msg) {

	q := req.Question[0]

	t := time.Now()
	defer s.handleQueryObservability(q, resp.RemoteAddr(), t)

	cfg := s.cfg.Load().(*Config)

	// Setup the message response
	m := new(dns.Msg)
	m.SetReply(req)
	m.Compress = !cfg.DisableCompression
	m.Authoritative = true
	m.RecursionAvailable = len(cfg.RecursorAddrs) > 0

	// Only add the SOA if requested

	// Get the QName without the domain suffix
	qName := strings.ToLower(dns.Fqdn(q.Name))

	args := structs.NodeListRequest{
		QueryOptions: structs.QueryOptions{
			AllowStale: cfg.AllowStale,
			Region:     s.region,
		},
	}

	var nodes structs.NodeListResponse

	if err := s.rpc("Node.List", &args, &nodes); err != nil {
		s.logger.Error("failed to list nodes", "error", err)
		return
	}

	for _, n := range nodes.Nodes {
		arpa, _ := dns.ReverseAddr(n.Address)
		if arpa == qName {
			ptr := &dns.PTR{
				Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: 0},
				Ptr: fmt.Sprintf("%s.node.%s.%s", strings.ToLower(n.Name), s.region, s.domain),
			}
			m.Answer = append(m.Answer, ptr)
			break
		}
	}

	// Recurse the query if we have not found a result and recursion is
	// enabled.
	if len(m.Answer) == 0 {
		if cfg.EnableRecusor {
			s.handleRecurse(resp, req)
		}
		return
	}

	// PTR record responses are globally valid.
	setEDNS(req, m, true)

	// Write out the complete response.
	if err := resp.WriteMsg(m); err != nil {
		s.logger.Warn("failed to respond", "error", err)
	}
}

func setEDNS(request *dns.Msg, response *dns.Msg, ecsGlobal bool) {

	// Enable EDNS if enabled
	edns := request.IsEdns0()
	if edns == nil {
		return
	}

	// cannot just use the SetEdns0 function as we need to embed
	// the ECS option as well
	ednsResp := new(dns.OPT)
	ednsResp.Hdr.Name = "."
	ednsResp.Hdr.Rrtype = dns.TypeOPT
	ednsResp.SetUDPSize(edns.UDPSize())

	// Setup the ECS option if present
	if subnet := ednsSubnetForRequest(request); subnet != nil {
		subOp := new(dns.EDNS0_SUBNET)
		subOp.Code = dns.EDNS0SUBNET
		subOp.Family = subnet.Family
		subOp.Address = subnet.Address
		subOp.SourceNetmask = subnet.SourceNetmask
		if c := response.Rcode; ecsGlobal || c == dns.RcodeNameError || c == dns.RcodeServerFailure || c == dns.RcodeRefused || c == dns.RcodeNotImplemented {
			// reply is globally valid and should be cached accordingly
			subOp.SourceScope = 0
		} else {
			// reply is only valid for the subnet it was queried with
			subOp.SourceScope = subnet.SourceNetmask
		}
		ednsResp.Option = append(ednsResp.Option, subOp)
	}

	response.Extra = append(response.Extra, ednsResp)
}

func ednsSubnetForRequest(req *dns.Msg) *dns.EDNS0_SUBNET {
	edns := req.IsEdns0()

	if edns == nil {
		return nil
	}

	for _, o := range edns.Option {
		if subnet, ok := o.(*dns.EDNS0_SUBNET); ok {
			return subnet
		}
	}

	return nil
}
