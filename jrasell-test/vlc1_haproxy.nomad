job "haproxy" {
  datacenters = ["vlc1"]

  group "haproxy" {
    network {
      port "http" {
        static = 8080
        to     = 80
      }
    }

    task "haproxy" {
      driver = "docker"

      config {
        image   = "haproxy:2.4.0"
        ports   = ["http"]
        command = "haproxy"
        args    = ["-f", "local/haproxy.cfg"]
      }

      template {
        data = <<EOH
resolvers nomad
  nameserver nomad 10.0.2.15:53
  resolve_retries 30
  timeout retry 2s
  hold valid 100s
  accepted_payload_size 8192

frontend api
   bind *:80
   default_backend api

backend api
  balance roundrobin
  server-template api 5 _backend._http.api.service.nomad resolvers nomad resolve-opts allow-dup-ip resolve-prefer ipv4 check
  EOH

        destination = "local/haproxy.cfg"
      }

      resources {
        cpu    = 500
        memory = 256
      }
    }
  }
}
