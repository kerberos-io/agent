# Docker 

The easiest, and let's say most natural, deployment is done by utilising `docker`. Docker can run a stand-alone, single, Kerberos Agent (or container) and a bigger set of Kerberos Agents (or containers) through `docker compose`.

## Running a single container

We are creating Docker images as part of our CI/CD process. You'll find our Docker images on [Docker hub](https://hub.docker.com/r/kerberos/agent). Pick a specific tag of choice, or use latest. Once done run below command, this will open the web interface of your Kerberos agent on port 80.  
    
    docker run -p 80:80 --name mycamera -d kerberos/agent:latest

Or for a develop build:

    docker run -p 80:80 --name mycamera -d kerberos/agent-dev:latest

Feel free to use another port if your host system already has a workload running on `80`. For example `8082`.

    docker run -p 8082:80 --name mycamera -d kerberos/agent:latest

### Attach a volume

By default your Kerberos agent will store all its configuration and recordings inside the container. It might be interesting to store both configuration and your recordings outside the container, on your local disk. This helps persisting your storage even after you decide to wipe out your Kerberos agent.

You attach a volume to your container by leveraging the `-v` option. To mount your own configuration file, execute as following:

1. Decide where you would like to store your configuration and recordings; create a new directory for the config file and recordings folder accordingly.

        mkdir agent
        mkdir agent/config
        mkdir agent/recordings

2. Once you have located your desired directory, copy the latest [`config.json`](https://github.com/kerberos-io/agent/blob/master/machinery/data/config/config.json) file into your config directory.

        wget https://raw.githubusercontent.com/kerberos-io/agent/master/machinery/data/config/config.json -O agent/config/config.json

3. Run the docker command as following to attach your config directory and recording directory.

        docker run -p 80:80 --name mycamera -v $(pwd)/agent/config:/home/agent/data/config  -v $(pwd)/agent/recordings:/home/agent/data/recordings -d kerberos/agent:latest

### Override with environment variables

Next to attaching the configuration file, it is also possible to override the configuration with environment variables. This makes deployments easier when leveraging `docker compose` or `kubernetes` deployments much easier and scalable. Using this approach we simplify automation through `ansible` and `terraform`. You'll find [the full list of environment variables on the main README.md file](https://github.com/kerberos-io/agent#override-with-environment-variables).

### Running multiple containers with Docker compose

When running multiple containers, you could execute the above process multiple times, or a better way is to run a `docker compose` with predefined configuration file, a `docker-compose.yaml`. 

