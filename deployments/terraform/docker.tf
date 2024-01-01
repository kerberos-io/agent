terraform {
  required_providers {
    linux = {
      source = "TelkomIndonesia/linux"
      version = "0.7.0"
    }
  }
}

provider "linux" {
    host     = "x.y.z.u"
    port     = 22
    user     = "root"
    password = "password"
}

locals {
    image = "kerberos/agent"
    version = "latest"
    port = 80
}

resource "linux_script" "install_docker" {
    lifecycle_commands {
        create = "apt update && apt install -y $PACKAGE_NAME"
        read = "apt-cache policy $PACKAGE_NAME | grep 'Installed:' | grep -v '(none)' | awk '{ print $2 }' | xargs | tr -d '\n'"
        update = "apt update && apt install -y $PACKAGE_NAME"
        delete = "apt remove -y $PACKAGE_NAME"
    }
    environment = {
        PACKAGE_NAME = "docker"
    }
}

resource "linux_script" "install_docker_kerberos_agent" {
    lifecycle_commands {
        create = "docker pull $IMAGE:$VERSION && docker run -d -p $PORT:80 --name agent $IMAGE:$VERSION"
        read = "docker inspect agent"
        update = "docker pull $IMAGE:$VERSION && docker rm agent --force && docker run -d -p $PORT:80 --name agent $IMAGE:$VERSION"
        delete = "docker rm agent --force"
    }
    environment = {
        IMAGE = local.image
        VERSION = local.version
        PORT = local.port
    }
}