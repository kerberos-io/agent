# Deployment with Terraform

If you are using Terraform as part of your DevOps stack, you might utilise it to deploy your Kerberos Agents. Within this deployment folder we have added an example Terraform file `docker.tf`, which installs the Kerberos Agent `docker` container on a remote system over `SSH`. We might create our own provider in the future, or add additional examples for example `snap`, `kubernetes`, etc.

For this example we will install Kerberos Agent using `docker` on a remote `linux` machine. Therefore we'll make sure we have the `TelkomIndonesia/linux` provider initialised.

     terraform init

Once initialised you should see similar output:

    Initializing the backend...

    Initializing provider plugins...
    - Reusing previous version of telkomindonesia/linux from the dependency lock file
    - Using previously-installed telkomindonesia/linux v0.7.0

Go and open the `docker.tf` file and locate the `linux` provider, modify following credentials accordingly. Make sure they match for creating an `SSH` connection.

    provider "linux" {
        host     = "x.y.z.u"
        port     = 22
        user     = "root"
        password = "password"
    }

Apply the `docker.tf` file, to install `docker` and the `kerberos/agent` docker container.

    terraform apply

Once done you should see following output, and you should be able to reach the remote machine on port `80` or if configured differently the specified port you've defined.

    Do you want to perform these actions?
    Terraform will perform the actions described above.
    Only 'yes' will be accepted to approve.

    Enter a value: yes

    linux_script.install_docker_kerberos_agent: Modifying... [id=a56cf7b0-db66-4f9b-beec-8a4dcef2a0c7]
    linux_script.install_docker_kerberos_agent: Modifications complete after 3s [id=a56cf7b0-db66-4f9b-beec-8a4dcef2a0c7]

    Apply complete! Resources: 0 added, 1 changed, 0 destroyed.
