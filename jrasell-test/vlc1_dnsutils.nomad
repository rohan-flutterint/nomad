job "dnsutils" {
  datacenters = ["vlc1"]

  group "dnsutils" {

    task "debian" {
      driver = "docker"

      config {
        image   = "debian:unstable-slim"
        command = "sleep"
        args    = ["3600000"]
      }
    }
  }
}