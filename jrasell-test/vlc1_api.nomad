job "api" {
  datacenters = ["vlc1"]

  group "backend" {
    count = 3

    network {
      port "http" {}
    }

    task "backend" {
      driver = "docker"

      config {
        image = "hashicorp/demo-webapp-lb-guide"
        ports = ["http"]
      }

      env {
        PORT    = "${NOMAD_PORT_http}"
        NODE_IP = "${NOMAD_IP_http}"
      }

      resources {
        cpu    = 100
        memory = 16
      }
    }
  }
}
