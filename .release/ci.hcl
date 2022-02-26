schema = "1"

project "nomad" {
  team = "nomad"
  slack {
    // TODO: ask nomad team which feed channel would be best -- perhaps #proj-nomad-releases?
    // TODO: add promote-dev-docker + security-scan-container workflows later
    notification_channel = "C01A3A54G0L"
  }
  github {
    organization = "hashicorp"
    repository = "nomad"
    release_branches = ["nomad-onboard-crt"]
  }
}

event "merge" {
  // "entrypoint" to use if build is not run automatically
  // i.e. send "merge" complete signal to orchestrator to trigger build
}

event "build" {
  depends = ["merge"]
  action "build" {
    organization = "hashicorp"
    repository = "nomad"
    workflow = "build"
  }
}

event "upload-dev" {
  depends = ["build"]
  action "upload-dev" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "upload-dev"
    depends = ["build"]
  }

  notification {
    on = "fail"
  }
}

event "security-scan-binaries" {
  depends = ["upload-dev"]
  action "security-scan-binaries" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "security-scan-binaries"
    config = "security-scan.hcl"
  }

  notification {
    on = "fail"
  }
}

event "notarize-darwin-amd64" {
  depends = ["security-scan-binaries"]
  action "notarize-darwin-amd64" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "notarize-darwin-amd64"
  }

  notification {
    on = "fail"
  }
}

event "notarize-windows-386" {
  depends = ["notarize-darwin-amd64"]
  action "notarize-windows-386" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "notarize-windows-386"
  }

  notification {
    on = "fail"
  }
}

event "notarize-windows-amd64" {
  depends = ["notarize-windows-386"]
  action "notarize-windows-amd64" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "notarize-windows-amd64"
  }

  notification {
    on = "fail"
  }
}

event "sign" {
  depends = ["notarize-windows-amd64"]
  action "sign" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "sign"
  }

  notification {
    on = "fail"
  }
}

event "sign-linux-rpms" {
  depends = ["sign"]
  action "sign-linux-rpms" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "sign-linux-rpms"
  }

  notification {
    on = "fail"
  }
}

event "verify" {
  depends = ["sign-linux-rpms"]
  action "verify" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "verify"
  }

  notification {
    on = "always"
  }
}
