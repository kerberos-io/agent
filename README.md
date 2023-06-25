# Kerberos Agent

<a target="_blank" href="https://kerberos.io"><img src="https://img.shields.io/badge/kerberos-website-gray.svg?longCache=true&colorB=brightgreen" alt="Kerberos Agent"></a>
<a target="_blank" href="https://doc.kerberos.io"><img src="https://img.shields.io/badge/kerberos-documentation-gray.svg?longCache=true&colorB=brightgreen" alt="Kerberos Agent"></a>

<a target="_blank" href="https://circleci.com/gh/kerberos-io/agent"><img src="https://circleci.com/gh/kerberos-io/agent.svg?style=svg"/></a>
<img src="https://github.com/kerberos-io/agent/workflows/Go/badge.svg"/>
<img src="https://github.com/kerberos-io/agent/workflows/React/badge.svg"/>
<img src="https://github.com/kerberos-io/agent/workflows/CodeQL/badge.svg"/>

<a target="_blank" href="https://pkg.go.dev/github.com/kerberos-io/agent/machinery"><img src="https://pkg.go.dev/badge/github.com/kerberos-io/agent/machinery" alt="PkgGoDev"></a>
<a target="_blank" href="https://codecov.io/gh/kerberos-io/agent"><img src="https://codecov.io/gh/kerberos-io/agent/branch/master/graph/badge.svg" alt="Coverage Status"></a>
<a target="_blank" href="https://goreportcard.com/report/github.com/kerberos-io/agent/machinery"><img src="https://goreportcard.com/badge/github.com/kerberos-io/agent/machinery" alt="Coverage Status"></a>
<a target="_blank" href="https://app.codacy.com/gh/kerberos-io/agent?utm_source=github.com&utm_medium=referral&utm_content=kerberos-io/agent&utm_campaign=Badge_Grade"><img src="https://api.codacy.com/project/badge/Grade/83d79d3092c040acb8c51ee0dfddf4b9"/>
<a target="_blank" href="https://www.figma.com/proto/msuYC6sv2cOCqZeDtBxNy7/%5BNEW%5D-Kerberos.io-Apps?node-id=1%3A1788&viewport=-490%2C191%2C0.34553584456443787&scaling=min-zoom&page-id=1%3A2%3Ffuid%3D449684443467913607" alt="Kerberos Agent"></a>

<a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
[![donate](https://brianmacdonald.github.io/Ethonate/svg/eth-donate-blue.svg)](https://brianmacdonald.github.io/Ethonate/address#0xf4a759C9436E2280Ea9cdd23d3144D95538fF4bE)
<a target="_blank" href="https://twitter.com/kerberosio?ref_src=twsrc%5Etfw"><img src="https://img.shields.io/twitter/url.svg?label=Follow%20%40kerberosio&style=social&url=https%3A%2F%2Ftwitter.com%2Fkerberosio" alt="Twitter Widget"></a>
[![Discord Shield](https://discordapp.com/api/guilds/1039619181731135499/widget.png?style=shield)](https://discord.gg/Bj77Vqfp2G)

[**Docker Hub**](https://hub.docker.com/r/kerberos/agent) | [**Documentation**](https://doc.kerberos.io) | [**Website**](https://kerberos.io) | [**View Demo**](https://demo.kerberos.io)

Kerberos Agent is an isolated and scalable video (surveillance) management agent made available as Open Source under the MIT License. This means that all the source code is available for you or your company, and you can use, transform and distribute the source code; as long you keep a reference of the original license. Kerberos Agent can be used for commercial usage (which was not the case for v2). Read more [about the license here](LICENSE).

![Kerberos Agent go through UI](./assets/img/kerberos-agent-overview.gif)

## :thinking: Prerequisites

- An IP camera which supports a RTSP H264 encoded stream,
  - (or) a USB camera, Raspberry Pi camera or other camera, that [you can tranform to a valid RTSP H264 stream](https://github.com/kerberos-io/camera-to-rtsp).
- Any hardware (ARMv6, ARMv7, ARM64, AMD) that can run a binary or container, for example: a Raspberry Pi, NVidia Jetson, Intel NUC, a VM, Bare metal machine or a full blown Kubernetes cluster.

## :video_camera: Is my camera working?

There are a myriad of cameras out there (USB, IP and other cameras), and it might be daunting to know if Kerberos Agent will work for your camera. [Therefore we are listing all the camera models that are acknowlegded by the community](https://github.com/kerberos-io/agent/issues/59). Feel free to add your camera to the list as well!

## :books: Overview

### Up and running in no time

1. [Quickstart - Docker](#quickstart---docker)
2. [Quickstart - Balena](#quickstart---balena)

### Introduction

3. [A world of Kerberos Agents](#a-world-of-kerberos-agents)

### Running and automation

4. [How to run and deploy a Kerberos Agent](#how-to-run-and-deploy-a-kerberos-agent)
5. [Access the Kerberos Agent](#access-the-kerberos-agent)
6. [Configure and persist with volume mounts](#configure-and-persist-with-volume-mounts)
7. [Configure with environment variables](#configure-with-environment-variables)

### Contributing

8. [Contribute with Codespaces](#contribute-with-codespaces)
9. [Develop and build](#develop-and-build)
10. [Building from source](#building-from-source)
11. [Building for Docker](#building-for-docker)

### Varia

12. [Support our project](#support-our-project)
13. [What is new?](#what-is-new)
14. [Contributors](#contributors)

## Quickstart - Docker

The easiest to get your Kerberos Agent up and running is to use our public image on [Docker hub](https://hub.docker.com/r/kerberos/agent). Once you have selected a specific tag, run below `docker` command, which will open the web interface of your Kerberos agent on port `80`, and off you go. For a more configurable and persistent deployment have a look at [Running and automating a Kerberos Agent](#running-and-automating-a-kerberos-agent).

    docker run -p 80:80 --name mycamera -d --restart=always kerberos/agent:latest

If you want to connect to an USB or Raspberry Pi camera, [you'll need to run our side car container](https://github.com/kerberos-io/camera-to-rtsp) which proxy the camera to an RTSP stream. In that case you'll want to configure the Kerberos Agent container to run in the host network, so it can connect directly to the RTSP sidecar.

    docker run --network=host --name mycamera -d --restart=always kerberos/agent:latest

## Quickstart - Balena

Run Kerberos Agent with [Balena Cloud](https://www.balena.io/) super powers. Monitor your Kerberos Agent with seamless remote access, over the air updates, an encrypted public `https` endpoint and many more. Checkout our application `video-surveillance` on [Balena Hub](https://hub.balena.io/apps/2064752/video-surveillance), and create your first or fleet of Kerberos Agent(s).

[![deploy with balena](https://balena.io/deploy.svg)](https://dashboard.balena-cloud.com/deploy?repoUrl=https://github.com/kerberos-io/agent)

## A world of Kerberos Agents

The Kerberos Agent is an isolated and scalable video (surveillance) management agent with a strong focus on user experience, scalability, resilience, extension and integration. Next to the Kerberos Agent, Kerberos.io provides many other tools such as [Kerberos Factory](https://github.com/kerberos-io/factory), [Kerberos Vault](https://github.com/kerberos-io/vault) and [Kerberos Hub](https://github.com/kerberos-io/hub) to provide additional capabilities: bring your own cloud, bring your own storage, central overview, live streaming, machine learning etc.

As mentioned above Kerberos.io applies the concept of agents. An agent is running next to (or on) your camera, and is processing a single camera feed. It applies motion based or continuous recording and make those recordings available through a user friendly web interface. A Kerberos Agent allows you to connect to other cloud services or integrates with custom applications. Kerberos Agent is used for personal usage and scales to enterprise production level deployments.

This repository contains everything you'll need to know about our core product, Kerberos Agent. Below you'll find a brief list of features and functions.

- Low memory and CPU usage.
- Simplified and modern user interface.
- Multi architecture (ARMv7, ARMv8, amd64, etc).
- Multi camera support: IP Cameras (H264), USB cameras and Raspberry Pi Cameras [through a RTSP proxy](https://github.com/kerberos-io/camera-to-rtsp).
- Single camera per instance (e.g. one container per camera).
- Primary and secondary stream setup (record full-res, stream low-res).
- Low resolution streaming through MQTT and full resolution streaming through WebRTC.
- Ability to specifiy conditions: offline mode, motion region, time table, continuous recording, etc.
- Post- and pre-recording on motion detection.
- Ability to create fragmented recordings, and streaming though HLS fMP4.
- [Deploy where you want](#how-to-run-and-deploy-a-kerberos-agent) with the tools you use: `docker`, `docker compose`, `ansible`, `terraform`, `kubernetes`, etc.
- Cloud storage/persistance: Kerberos Hub, Kerberos Vault and Dropbox. [(WIP: Minio, Storj, Google Drive, FTP etc.)](https://github.com/kerberos-io/agent/issues/95)
- WIP: Integrations (Webhooks, MQTT, Script, etc).
- REST API access and documentation through Swagger (trigger recording, update configuration, etc).
- MIT License

## How to run and deploy a Kerberos Agent

As described before a Kerberos Agent is a container, which can be deployed through various ways and automation tools such as `docker`, `docker compose`, `kubernetes` and the list goes on. To simplify your life we have come with concrete and working examples of deployments to help you speed up your Kerberos.io journey.

We have documented the different deployment models [in the `deployments` directory](https://github.com/kerberos-io/agent/tree/master/deployments) of this repository. There you'll learn and find how to deploy using:

- [Static binary](https://github.com/kerberos-io/agent/tree/master/deployments#0-static-binary)
- [Docker](https://github.com/kerberos-io/agent/tree/master/deployments#1-docker)
- [Docker Compose](https://github.com/kerberos-io/agent/tree/master/deployments#2-docker-compose)
- [Kubernetes](https://github.com/kerberos-io/agent/tree/master/deployments#3-kubernetes)
- [Red Hat OpenShift with Ansible](https://github.com/kerberos-io/agent/tree/master/deployments#4-red-hat-ansible-and-openshift)
- [Terraform](https://github.com/kerberos-io/agent/tree/master/deployments#5-terraform)
- [Salt](https://github.com/kerberos-io/agent/tree/master/deployments#6-salt)
- [Balena](https://github.com/kerberos-io/agent/tree/master/deployments#1-balena)

By default your Kerberos Agents will store all its configuration and recordings inside the container. To help you automate and have a more consistent data governance, you can attach volumes to configure and persist data of your Kerberos Agents, and/or configure each Kerberos Agent through environment variables.

## Access the Kerberos Agent

Once you have deployed the Kerberos Agent, using one of the deployment models described above, you will be able to access the Kerberos Agent user interface. A login page is presented asking for some credentials.

The default username and password for the Kerberos Agent is:

- Username: `root`
- Password: `root`

**_Please note that you change the username and password for a final installation, see [Configure with environment variables](#configure-with-environment-variables) below._**

## Configure and persist with volume mounts

An example of how to mount a host directory is shown below using `docker`, but is applicable for [all the deployment models and tools described above](#running-and-automating-a-kerberos-agent).

You attach a volume to your container by leveraging the `-v` option. To mount your own configuration file and recordings folder, execute as following:

    docker run -p 80:80 --name mycamera \
    -v $(pwd)/agent/config:/home/agent/data/config \
    -v $(pwd)/agent/recordings:/home/agent/data/recordings \
    -d --restart=always kerberos/agent:latest

More example [can be found in the deployment section](https://github.com/kerberos-io/agent/tree/master/deployments) for each deployment and automation tool. Please note to verify the permissions of the directory/volume you are attaching. More information in [this issue](https://github.com/kerberos-io/agent/issues/80).

    chmod -R 755 kerberos-agent/
    chown 100:101 kerberos-agent/ -R

## Configure with environment variables

Next to attaching the configuration file, it is also possible to override the configuration with environment variables. This makes deployments easier when leveraging `docker compose` or `kubernetes` deployments much easier and scalable. Using this approach we simplify automation through `ansible` and `terraform`.

    docker run -p 80:80 --name mycamera \
    -e AGENT_NAME=mycamera \
    -e AGENT_TIMEZONE=Europe/Brussels \
    -e AGENT_CAPTURE_IPCAMERA_RTSP=rtsp://fake.kerberos.io/stream \
    -e AGENT_CAPTURE_CONTINUOUS=true \
    -d --restart=always kerberos/agent:latest

| Name                                    | Description                                                                                     | Default Value                  |
| --------------------------------------- | ----------------------------------------------------------------------------------------------- | ------------------------------ |
| `AGENT_MODE`                            | You can choose to run this in 'release' for production, and or 'demo' for showcasing.           | "release"                      |
| `AGENT_USERNAME`                        | The username used to authenticate against the Kerberos Agent login page.                        | "root"                         |
| `AGENT_PASSWORD`                        | The password used to authenticate against the Kerberos Agent login page.                        | "root"                         |
| `AGENT_KEY`                             | A unique identifier for your Kerberos Agent, this is auto-generated but can be overriden.       | ""                             |
| `AGENT_NAME`                            | The agent friendly-name.                                                                        | "agent"                        |
| `AGENT_TIMEZONE`                        | Timezone which is used for converting time.                                                     | "Africa/Ceuta"                 |
| `AGENT_REMOVE_AFTER_UPLOAD`             | When enabled, recordings uploaded successfully to a storage will be removed from disk.          | "true"                         |
| `AGENT_OFFLINE`                         | Makes sure no external connection is made.                                                      | "false"                        |
| `AGENT_AUTO_CLEAN`                      | Cleans up the recordings directory.                                                             | "true"                         |
| `AGENT_AUTO_CLEAN_MAX_SIZE`             | If `AUTO_CLEAN` enabled, set the max size of the recordings directory in (MB).                  | "100"                          |
| `AGENT_TIME`                            | Enable the timetable for Kerberos Agent                                                         | "false"                        |
| `AGENT_TIMETABLE`                       | A (weekly) time table to specify when to make recordings "start1,end1,start2,end2;start1..      | ""                             |
| `AGENT_REGION_POLYGON`                  | A single polygon set for motion detection: "x1,y1;x2,y2;x3,y3;...                               | ""                             |
| `AGENT_CAPTURE_IPCAMERA_RTSP`           | Full-HD RTSP endpoint to the camera you're targetting.                                          | ""                             |
| `AGENT_CAPTURE_IPCAMERA_SUB_RTSP`       | Sub-stream RTSP endpoint used for livestreaming (WebRTC).                                       | ""                             |
| `AGENT_CAPTURE_IPCAMERA_ONVIF`          | Mark as a compliant ONVIF device.                                                               | ""                             |
| `AGENT_CAPTURE_IPCAMERA_ONVIF_XADDR`    | ONVIF endpoint/address running on the camera.                                                   | ""                             |
| `AGENT_CAPTURE_IPCAMERA_ONVIF_USERNAME` | ONVIF username to authenticate against.                                                         | ""                             |
| `AGENT_CAPTURE_IPCAMERA_ONVIF_PASSWORD` | ONVIF password to authenticate against.                                                         | ""                             |
| `AGENT_CAPTURE_RECORDING`               | Toggle for enabling making recordings.                                                          | "true"                         |
| `AGENT_CAPTURE_CONTINUOUS`              | Toggle for enabling continuous or motion based recording.                                       | "false"                        |
| `AGENT_CAPTURE_PRERECORDING`            | If `CONTINUOUS` set to `false`, specify the recording time (seconds) before after motion event. | "10"                           |
| `AGENT_CAPTURE_POSTRECORDING`           | If `CONTINUOUS` set to `false`, specify the recording time (seconds) after motion event.        | "20"                           |
| `AGENT_CAPTURE_MAXLENGTH`               | The maximum length of a single recording (seconds).                                             | "30"                           |
| `AGENT_CAPTURE_PIXEL_CHANGE`            | If `CONTINUOUS` set to `false`, the number of pixel require to change before motion triggers.   | "150"                          |
| `AGENT_CAPTURE_FRAGMENTED`              | Set the format of the recorded MP4 to fragmented (suitable for HLS).                            | "false"                        |
| `AGENT_CAPTURE_FRAGMENTED_DURATION`     | If `AGENT_CAPTURE_FRAGMENTED` set to `true`, define the duration (seconds) of a fragment.       | "8"                            |
| `AGENT_MQTT_URI`                        | A MQTT broker endpoint that is used for bi-directional communication (live view, onvif, etc)    | "tcp://mqtt.kerberos.io:1883"  |
| `AGENT_MQTT_USERNAME`                   | Username of the MQTT broker.                                                                    | ""                             |
| `AGENT_MQTT_PASSWORD`                   | Password of the MQTT broker.                                                                    | ""                             |
| `AGENT_STUN_URI`                        | When using WebRTC, you'll need to provide a STUN server.                                        | "stun:turn.kerberos.io:8443"   |
| `AGENT_TURN_URI`                        | When using WebRTC, you'll need to provide a TURN server.                                        | "turn:turn.kerberos.io:8443"   |
| `AGENT_TURN_USERNAME`                   | TURN username used for WebRTC.                                                                  | "username1"                    |
| `AGENT_TURN_PASSWORD`                   | TURN password used for WebRTC.                                                                  | "password1"                    |
| `AGENT_CLOUD`                           | Store recordings in Kerberos Hub (s3), Kerberos Vault (kstorage) or Dropbox (dropbox).          | "s3"                           |
| `AGENT_HUB_URI`                         | The Kerberos Hub API, defaults to our Kerberos Hub SAAS.                                        | "https://api.hub.domain.com"   |
| `AGENT_HUB_KEY`                         | The access key linked to your account in Kerberos Hub.                                          | ""                             |
| `AGENT_HUB_PRIVATE_KEY`                 | The secret access key linked to your account in Kerberos Hub.                                   | ""                             |
| `AGENT_HUB_REGION`                      | The Kerberos Hub region, to which you want to upload.                                           | ""                             |
| `AGENT_HUB_SITE`                        | The site ID of a site you've created in your Kerberos Hub account.                              | ""                             |
| `AGENT_KERBEROSVAULT_URI`               | The Kerberos Vault API url.                                                                     | "https://vault.domain.com/api" |
| `AGENT_KERBEROSVAULT_ACCESS_KEY`        | The access key of a Kerberos Vault account.                                                     | ""                             |
| `AGENT_KERBEROSVAULT_SECRET_KEY`        | The secret key of a Kerberos Vault account.                                                     | ""                             |
| `AGENT_KERBEROSVAULT_PROVIDER`          | A Kerberos Vault provider you have created (optional).                                          | ""                             |
| `AGENT_KERBEROSVAULT_DIRECTORY`         | The directory, in the provider, where the recordings will be stored in.                         | ""                             |
| `AGENT_DROPBOX_ACCESS_TOKEN`            | The Access Token from your Dropbox app, that is used to leverage the Dropbox SDK.               | ""                             |
| `AGENT_DROPBOX_DIRECTORY`               | The directory, in the provider, where the recordings will be stored in.                         | ""                             |

## Contribute with Codespaces

One of the major blockers for letting you contribute to an Open Source project is to setup your local development machine. Why? Because you might have already some tools and libraries installed that are used for other projects, and the libraries you would need for Kerberos Agent, for example FFmpeg, might require a different version. Welcome to the dependency hell..

By leveraging codespaces, which the Kerberos Agent repo supports, you will be able to setup the required development environment in a few minutes. By opening the `<> Code` tab on the top of the page, you will be able to create a codespace, [using the Kerberos Devcontainer](https://github.com/kerberos-io/devcontainer) base image. This image requires all the relevant dependencies: FFmpeg, OpenCV, Golang, Node, Yarn, etc.

![Kerberos Agent codespace](assets/img/codespace.png)

After a few minutes, you will see a beautiful `Visual Studio Code` shown in your browser, and you are ready to code!

![Kerberos Agent VSCode](assets/img/codespace-vscode.png)

On opening of the GitHub Codespace, some dependencies will be installed. Once this is done go ahead to the `ui/src/config.json` file, and (un)comment following section. Make sure to replace the `externalHost` variable with the DNS name you will retrieve from the next step.

    // Uncomment this when using codespaces or other special DNS names (which you can't control)
    // replace this with the DNS name of the kerberos agent server (the codespace url)
    const externalHost = 'cedricve-automatic-computing-machine-v647rxvj4whx9qp-80.preview.app.github.dev';

    const dev = {
      ENV: 'dev',
      HOSTNAME: externalHost,
      //API_URL: `${protocol}//${hostname}:8080/api`,
      //URL: `${protocol}//${hostname}:8080`,
      //WS_URL: `${websocketprotocol}//${hostname}:8080/ws`,

      // Uncomment, and comment the above lines, when using codespaces or other special DNS names (which you can't control)
      API_URL: `${protocol}//${externalHost}/api`,
      URL: `${protocol}//${externalHost}`,
      WS_URL: `${websocketprotocol}//${externalHost}/ws`,
    };

Go and open two terminals one for the `ui` project and one for the `machinery` project.

1.  Terminal A:

        cd machinery/
        go run main.go run camera 80

2.  Terminal B:

        cd ui/
        yarn start

Once executed, a popup will show up mentioning `portforwarding`. You should see two ports being opened, one for the ui `3000` and one for the machinery `80`. `Right-click` on the port `80` and change visibility from `private` to `public`, this is required to avoid `CORS` errors.

![Codespace make public](./assets/img/codespace-make-public.png)

As mentioned above, copy the hostname of the `machinery` DNS name, and past it in the `ui/src/config.json` file. Once done reload, the `ui` page in your browser, and you should be able to access the login page with the default credentials `root` and `root`.

## Develop and build

Kerberos Agent is divided in two parts a `machinery` and `web`. Both parts live in this repository in their relative folders. For development or running the application on your local machine, you have to run both the `machinery` and the `web` as described below. When running in production everything is shipped as only one artifact, read more about this at [Building for production](#building-for-production).

### UI

The `web` is a **React** project which is the main entry point for an end user to view recordings, a livestream, and modify the configuration of the `machinery`.

    git clone https://github.com/kerberos-io/agent
    cd ui
    yarn start

This will start a webserver and launches the web app on port `3000`.

![login-agent](./assets/img/agent-login.gif)

Once signed in you'll see the dashboard page showing up. After successfull configuration of your agent, you'll should see a live view and possible events recorded to disk.

![dashboard-agent](./assets/img/agent-dashboard.png)

### Machinery

The `machinery` is a **Golang** project which delivers two functions: it acts as the Kerberos Agent which is doing all the heavy lifting with camera processing and other kinds of logic, on the other hand it acts as a webserver (Rest API) that allows communication from the web (React) or any other custom application. The API is documented using `swagger`.

You can simply run the `machinery` using following commands.

    git clone https://github.com/kerberos-io/agent
    cd machinery
    go run main.go run mycameraname 80

This will launch the Kerberos Agent and run a webserver on port `80`. You can change the port by your own preference. We strongly support the usage of [Goland](https://www.jetbrains.com/go/) or [Visual Studio Code](https://code.visualstudio.com/), as it comes with all the debugging and linting features builtin.

![VSCode desktop](./assets/img/vscode-desktop.png)

## Building from source

Running Kerberos Agent in production only require a single binary to run. Nevertheless, we have two parts, the `machinery` and the `web`, we merge them during build time. So this is what happens.

### UI

To build the Kerberos Agent web app, you simply have to run the `build` command of `yarn`. This will create a `build` directory inside the `web` directory, which contains a minified version of the React application. Other than that, we [also move](https://github.com/kerberos-io/agent/blob/master/web/package.json#L16) this `build` directory to the `machinery` directory.

    cd ui
    yarn build

### Machinery

Building the `machinery` is also super easy 🚀, by using `go build` you can create a single binary which ships it all; thank you Golang. After building you will endup with a binary called `main`, this is what contains everything you need to run Kerberos Agent.

Remember the build step of the `web` part, during build time we move the build directory to the `machinery` directory. Inside the `machinery` web server [we reference the](https://github.com/kerberos-io/agent/blob/master/machinery/src/routers/http/Server.go#L44) `build` directory. This makes it possible to just a have single web server that runs it all.

    cd machinery
    go build

## Building for Docker

Inside the root of this `agent` repository, you will find a `Dockerfile`. This file contains the instructions for building and shipping **Kerberos Agent**. Important to note is that start from a prebuild base image, `kerberos/base:xxx`.
This base image contains already a couple of tools, such as Golang, FFmpeg and OpenCV. We do this for faster compilation times.

By running the `docker build` command, you will create the Kerberos Agent Docker image. After building you can simply run the image as a Docker container.

    docker build -t kerberos/agent .

## What is new?

This repository contains the next generation of Kerberos.io, **Kerberos Agent (v3)**, and is the successor of the machinery and web repositories. A switch in technologies and architecture has been made. This version is still under active development and can be followed on the [develop branch](https://github.com/kerberos-io/agent/tree/develop) and [project overview](https://github.com/kerberos-io/agent/projects/1).

Read more about this [at the FAQ](#faq) below.

![opensource-to-agent](https://user-images.githubusercontent.com/1546779/172066873-7752c979-de63-4417-8d26-34192fdbd1e6.svg)

## Contributors

This project exists thanks to all the people who contribute.

<a href="https://github.com/kerberos-io/agent/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=kerberos-io/agent" />
</a>
