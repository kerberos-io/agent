# Deployment with Docker

The easiest, and let's say most natural, deployment is done [by utilising `docker`](#1-running-a-single-container). Docker can run a stand-alone, single, Kerberos Agent (or container) and a bigger set of Kerberos Agents (or containers) [through `docker compose`](#2-running-multiple-containers-with-docker-compose).

## 1. Running a single container

We are creating Docker images as part of our CI/CD process. You'll find our Docker images on [Docker hub](https://hub.docker.com/r/kerberos/agent). Pick a specific tag of choice, or use latest. Once done run below command, this will open the web interface of your Kerberos agent on port 80.

    docker run -p 80:80 --name mycamera -d kerberos/agent:latest

Or for a develop build:

    docker run -p 80:80 --name mycamera -d kerberos/agent-dev:latest

Feel free to use another port if your host system already has a workload running on `80`. For example `8082`.

    docker run -p 8082:80 --name mycamera -d kerberos/agent:latest

### Attach a volume

By default your Kerberos agent will store all its configuration and recordings inside the container. It might be interesting to store both configuration and your recordings outside the container, on your local disk. This helps persisting your storage even after you decide to wipe out your Kerberos agent.

You attach a volume to your container by leveraging the `-v` option. To mount your own configuration file, execute as following:

1.  Decide where you would like to store your configuration and recordings; create a new directory for the config file and recordings folder accordingly.

        mkdir agent
        mkdir agent/config
        mkdir agent/recordings

2.  Once you have located your desired directory, copy the latest [`config.json`](https://github.com/kerberos-io/agent/blob/master/machinery/data/config/config.json) file into your config directory.

        wget https://raw.githubusercontent.com/kerberos-io/agent/master/machinery/data/config/config.json -O agent/config/config.json

3.  Run the docker command as following to attach your config directory and recording directory.

        docker run -p 80:80 --name mycamera \
        -v $(pwd)/agent/config:/home/agent/data/config \
        -v $(pwd)/agent/recordings:/home/agent/data/recordings \
        -d --restart=always kerberos/agent:latest

### Override with environment variables

Next to attaching the configuration file, it is also possible to override the configuration with environment variables. This makes deployments when leveraging `docker compose` or `kubernetes` much easier and more scalable. Using this approach we simplify automation through `ansible` and `terraform`. You'll find [the full list of environment variables on the main README.md file](https://github.com/kerberos-io/agent#override-with-environment-variables).

### 2. Running multiple containers with Docker compose

When running multiple containers, you could execute the above process multiple times, or a better way is to run a `docker compose` with predefined configuration file, a `docker-compose.yaml`.

You'll find [an example `docker-compose.yaml` file here](https://github.com/kerberos-io/agent/blob/master/deployments/docker/docker-compose.yaml). This configuration file includes a definition for running 3 Kerberos Agents (or containers). By specifying environment variables you can override the internal configuration. To add more Kerberos Agents to your deployment, just `copy-paste` a `service` block and modify the name, exposed port, and settings accordingly.

    kerberos-agent2:
        image: "kerberos/agent:latest"
        ports:
        - "8082:80"
        environment:
        - AGENT_NAME=agent2
        - AGENT_CAPTURE_IPCAMERA_RTSP=rtsp://x.x.x.x:554/Streaming/Channels/101
        - AGENT_HUB_KEY=yyy
        - AGENT_HUB_PRIVATE_KEY=yyy

#### Attaching volumes

As described in [1. Running a single container](#1-running-a-single-container) you can also assign volumes to your `docker compose` services. A volume can be added to persist the recordings of your Kerberos Agents on the host machine, or to provide more accurate configurations.

When attaching a volume for persisting recordings or mounting configuration files from the host system. the `docker-compose.yaml` would look like this.

Let's start by creating some directories on your host system. We'll consider 3 Kerberos Agents in this example.

    mkdir -p agent1/config agent1/recordings
    mkdir -p agent2/config agent2/recordings
    mkdir -p agent3/config agent3/recordings

Download the configuration file in each Kerberos Agent configuration directory.

    wget https://raw.githubusercontent.com/kerberos-io/agent/master/machinery/data/config/config.json -O agent1/config/config.json
    wget https://raw.githubusercontent.com/kerberos-io/agent/master/machinery/data/config/config.json -O agent2/config/config.json
    wget https://raw.githubusercontent.com/kerberos-io/agent/master/machinery/data/config/config.json -O agent3/config/config.json

Next we'll add a `volumes:` section to each Kerberos Agent (service) in the `docker-compose-with-volumes.yaml` file.

    volumes:
      - ./agent1/config:/home/agent/data/config
      - ./agent1/recordings:/home/agent/data/recordings

We'll repeat that for the other Kerberos Agents as well. You can review [the final result over here](https://github.com/kerberos-io/agent/blob/master/deployments/docker/docker-compose-with-volumes.yaml).

Run the `docker compose` command by providing a different configuration file name.

    docker compose -f docker-compose-with-volumes.yaml up

Please note that you can use a combination of using a configuration file and environment variables at the same time. However environment variables will always override the setting in your configuration file.
