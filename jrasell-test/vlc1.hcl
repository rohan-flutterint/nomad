bind_addr  = "0.0.0.0"
data_dir   = "/tmp/nomad-cluster/nomad-vlc-1"
name       = "nomad-vlc-1"
region     = "valencia"
datacenter = "vlc1"
log_level  = "TRACE"

server {
  enabled          = true
  bootstrap_expect = 1
}

client {
  enabled           = true
  network_interface = "eth0"

  server_join {
    retry_join = ["127.0.0.1:4647"]
  }
}
