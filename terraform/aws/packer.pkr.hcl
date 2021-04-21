source "amazon-ebs" "hashistack" {
  ami_name      = "hashistack {{timestamp}}"
  region        = "us-east-1"
  instance_type = "t2.medium"

  source_ami_filter {
    filters = {
      virtualization-type                = "hvm"
      architecture                       = "x86_64"
      name                               = "ubuntu/images/*ubuntu-focal-20.04-amd64-server-*"
      "block-device-mapping.volume-type" = "gp2"
      root-device-type                   = "ebs"
    }
    owners      = ["099720109477"] # Canonical's owner ID
    most_recent = true
  }

  #  launch_block_device_mappings {
  #    device_name = "/dev/sda1"
  #    volume_size = 16
  #  }

  communicator         = "ssh"
  ssh_username         = "ubuntu"
  ssh_keypair_name     = "laoqui"
  ssh_private_key_file = "~/.ssh/laoqui.pem"
}

build {
  sources = [
    "source.amazon-ebs.hashistack"
  ]

  provisioner "shell" {
    inline = [
      "sudo mkdir -p /ops",
      "sudo chmod 777 /ops"
    ]
  }

  provisioner "file" {
    source      = "../shared"
    destination = "/ops"
  }

  provisioner "file" {
    source      = "../examples"
    destination = "/ops"
  }

  provisioner "shell" {
    script = "../shared/scripts/setup.sh"
    #    environment_vars = [
    #      "INSTALL_NVIDIA_DOCKER=true",
    #    ]
  }
}
