job "gitdepth" {
  type        = "batch"
  datacenters = ["dc1"]

  group "gitdepth" {
    task "gitdepth" {
      driver = "docker"

      restart {
        attempts = 0
      }

      config {
        image   = "bitnami/git:2.33.0"
        command = "/bin/sh"

        # Logs should contain "1"
        args = ["-c", "cd /local/nomad && git rev-list HEAD --count"]
      }

      artifact {
        source = "github.com/hashicorp/nomad"
        options {
          ref   = "main"
          depth = "1"
        }
        destination = "local/nomad"
      }

      resources {
        cpu    = 200
        memory = 200
      }
    }
  }
}
