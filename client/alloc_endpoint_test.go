package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-msgpack/codec"
	"github.com/hashicorp/nomad/acl"
	"github.com/hashicorp/nomad/client/config"
	cstructs "github.com/hashicorp/nomad/client/structs"
	"github.com/hashicorp/nomad/helper/pluginutils/catalog"
	"github.com/hashicorp/nomad/nomad"
	"github.com/hashicorp/nomad/nomad/mock"
	nstructs "github.com/hashicorp/nomad/nomad/structs"
	nconfig "github.com/hashicorp/nomad/nomad/structs/config"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/testutil"
	"github.com/stretchr/testify/require"
)

// TestAlloc_ExecStreaming_ACL_WithIsolation_None asserts that token needs
// alloc-node-exec acl policy as well when no isolation is used
func TestAlloc_ExecStreaming_ACL_WithIsolation_None(t *testing.T) {

	isolation := drivers.FSIsolationNone

	// Start a server and client
	s, root, cleanupS := nomad.TestACLServer(t, nil)
	defer cleanupS()
	testutil.WaitForLeader(t, s.RPC)

	client, cleanup := TestClient(t, func(c *config.Config) {
		c.ACLEnabled = true
		c.Servers = []string{s.GetConfig().RPCAddr.String()}

		pluginConfig := []*nconfig.PluginConfig{
			{
				Name: "mock_driver",
				Config: map[string]interface{}{
					"fs_isolation": string(isolation),
				},
			},
		}

		c.PluginLoader = catalog.TestPluginLoaderWithOptions(t, "", map[string]string{}, pluginConfig)
	})
	defer cleanup()

	// Create a bad token
	policyBad := mock.NamespacePolicy("other", "", []string{acl.NamespaceCapabilityDeny})
	tokenBad := mock.CreatePolicyAndToken(t, s.State(), 1005, "invalid", policyBad)

	policyAllocExec := mock.NamespacePolicy(nstructs.DefaultNamespace, "",
		[]string{acl.NamespaceCapabilityAllocExec})
	tokenAllocExec := mock.CreatePolicyAndToken(t, s.State(), 1009, "alloc-exec", policyAllocExec)

	policyAllocNodeExec := mock.NamespacePolicy(nstructs.DefaultNamespace, "",
		[]string{acl.NamespaceCapabilityAllocExec, acl.NamespaceCapabilityAllocNodeExec})
	tokenAllocNodeExec := mock.CreatePolicyAndToken(t, s.State(), 1009, "alloc-node-exec", policyAllocNodeExec)

	job := mock.BatchJob()
	job.TaskGroups[0].Count = 1
	job.TaskGroups[0].Tasks[0].Config = map[string]interface{}{
		"run_for": "20s",
		"exec_command": map[string]interface{}{
			"run_for":       "1ms",
			"stdout_string": "some output",
		},
	}

	// Wait for client to be running job
	testutil.WaitForRunningWithToken(t, s.RPC, job, root.SecretID)

	// Get the allocation ID
	args := nstructs.AllocListRequest{}
	args.Region = "global"
	args.AuthToken = root.SecretID
	args.Namespace = nstructs.DefaultNamespace
	resp := nstructs.AllocListResponse{}
	require.NoError(t, s.RPC("Alloc.List", &args, &resp))
	require.Len(t, resp.Allocations, 1)
	allocID := resp.Allocations[0].ID

	cases := []struct {
		Name          string
		Token         string
		ExpectedError string
	}{
		{
			Name:          "bad token",
			Token:         tokenBad.SecretID,
			ExpectedError: nstructs.ErrPermissionDenied.Error(),
		},
		{
			Name:          "alloc-exec token",
			Token:         tokenAllocExec.SecretID,
			ExpectedError: nstructs.ErrPermissionDenied.Error(),
		},
		{
			Name:          "alloc-node-exec token",
			Token:         tokenAllocNodeExec.SecretID,
			ExpectedError: "",
		},
		{
			Name:          "root token",
			Token:         root.SecretID,
			ExpectedError: "",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {

			// Make the request
			req := &cstructs.AllocExecRequest{
				AllocID: allocID,
				Task:    job.TaskGroups[0].Tasks[0].Name,
				Tty:     true,
				Cmd:     []string{"placeholder command"},
				QueryOptions: nstructs.QueryOptions{
					Region:    "global",
					AuthToken: c.Token,
					Namespace: nstructs.DefaultNamespace,
				},
			}

			// Get the handler
			handler, err := client.StreamingRpcHandler("Allocations.Exec")
			require.Nil(t, err)

			// Create a pipe
			p1, p2 := net.Pipe()
			defer p1.Close()
			defer p2.Close()

			errCh := make(chan error)
			frames := make(chan *drivers.ExecTaskStreamingResponseMsg)

			// Start the handler
			go handler(p2)
			go decodeFrames(t, p1, frames, errCh)

			// Send the request
			encoder := codec.NewEncoder(p1, nstructs.MsgpackHandle)
			require.Nil(t, encoder.Encode(req))

			select {
			case <-time.After(3 * time.Second):
			case err := <-errCh:
				if c.ExpectedError == "" {
					require.NoError(t, err)
				} else {
					require.Contains(t, err.Error(), c.ExpectedError)
				}
			case f := <-frames:
				// we are good if we don't expect an error
				if c.ExpectedError != "" {
					require.Fail(t, "unexpected frame", "frame: %#v", f)
				}
			}
		})
	}
}

func decodeFrames(t *testing.T, p1 net.Conn, frames chan<- *drivers.ExecTaskStreamingResponseMsg, errCh chan<- error) {
	// Start the decoder
	decoder := codec.NewDecoder(p1, nstructs.MsgpackHandle)

	for {
		var msg cstructs.StreamErrWrapper
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF || strings.Contains(err.Error(), "closed") {
				return
			}
			t.Logf("received error decoding: %#v", err)

			errCh <- fmt.Errorf("error decoding: %v", err)
			return
		}

		if msg.Error != nil {
			errCh <- msg.Error
			continue
		}

		var frame drivers.ExecTaskStreamingResponseMsg
		if err := json.Unmarshal(msg.Payload, &frame); err != nil {
			errCh <- err
			return
		}
		t.Logf("received message: %#v", msg)
		frames <- &frame
	}
}
