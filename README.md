# Kerberos Agent

<a target="_blank" href="https://kerberos.io"><img src="https://img.shields.io/badge/kerberos-website-gray.svg?longCache=true&colorB=brightgreen" alt="Kerberos Agent"></a>
<a target="_blank" href="https://doc.kerberos.io"><img src="https://img.shields.io/badge/kerberos-documentation-gray.svg?longCache=true&colorB=brightgreen" alt="Kerberos Agent"></a>
<a target="_blank" href="https://twitter.com/kerberosio?ref_src=twsrc%5Etfw"><img src="https://img.shields.io/twitter/url.svg?label=Follow%20%40kerberosio&style=social&url=https%3A%2F%2Ftwitter.com%2Fkerberosio" alt="Twitter Widget"></a>
<a target="_blank" href="https://join.slack.com/t/kerberosio/shared_invite/zt-1a5oj4pwm-O4qCAN9c5r2um0Ns0ge8ww"><img src="https://img.shields.io/badge/slack-@kerberosio-yellow.svg?logo=slack " alt="Kerberos.io"></a>

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
[![donate](
https://brianmacdonald.github.io/Ethonate/svg/eth-donate-blue.svg)](
https://brianmacdonald.github.io/Ethonate/address#0xf4a759C9436E2280Ea9cdd23d3144D95538fF4bE)

[**Docker Hub**](https://hub.docker.com/r/kerberos/agent) | [**Documentation**](https://doc.kerberos.io) | [**Website**](https://kerberos.io)

Kerberos Agent is a cutting edge video surveillance management system made available as Open Source under the MIT License. This means that all the source code is available for you or your company, and you can use, transform and distribute the source code; as long you keep a reference of the original license. Kerberos Agent can be used for commercial usage (which was not the case for v2). Read more [about the license here](LICENSE).

![login-agent](./assets/img/agent-login.png)
    
## Support our project

If you like our product please feel free to execute an Ethereum donation. All donations will flow back and split to our Open Source contributors, as they are the heart of this community.
   
<img width="272" alt="Ethereum donation linke" src="https://user-images.githubusercontent.com/1546779/173443671-3d773068-ae10-4862-a990-dc7c89f3d9c2.png">

Ethereum Address: `0xf4a759C9436E2280Ea9cdd23d3144D95538fF4bE`
    
## Work In Progress

Kerberos Agent (v3) is not yet released, and is actively developed. You can follow the progress [on our project board](https://github.com/kerberos-io/agent/projects/1) and review our designs at [Figma](https://www.figma.com/proto/msuYC6sv2cOCqZeDtBxNy7/%5BNEW%5D-Kerberos.io-Apps?node-id=1%3A1788&viewport=-490%2C191%2C0.34553584456443787&scaling=min-zoom&page-id=1%3A2%3Ffuid%3D449684443467913607). Feel free to give any feedback.

## Previous releases

This repository contains the next generation of Kerberos.io, **Kerberos Agent (v3)**, and is the successor of the machinery and web repositories. A switch in technologies and architecture has been made. This version is still under active development and can be followed on the [develop branch](https://github.com/kerberos-io/agent/tree/develop) and [project overview](https://github.com/kerberos-io/agent/projects/1).

Read more about this [at the FAQ](#faq) below.

![opensource-to-agent](https://user-images.githubusercontent.com/1546779/172066873-7752c979-de63-4417-8d26-34192fdbd1e6.svg)
    
## Introduction

Kerberos.io is a cutting edge video surveillance system with a strong focus on user experience, scalability, resilience, extension and integration. Kerberos.io provides different solutions, but from a high level point of view it comes into two flavours: Kerberos Agent and Kerberos Enterprise Suite. Bottom line Kerberos Enterprise Suite extends Kerberos Agent with additional components such as Kerberos Factory, Kerberos Vault and Kerberos Hub.

### Kerberos Agent

- Installation in seconds (Kerberos Etcher, Docker, Binaries).
- Simplified and modern user interface.
- Multi architecture (ARMv7, ARMv8, amd64, etc).
- Multi camera support: IP Cameras (MJPEG/H264), USB cameras, Raspberry Pi Cameras.
- Single camera per instance (e.g. One Docker container per camera).
- Cloud integration through Webhooks, MQTT, etc.
- Cloud storage through Kerberos Hub.
- MIT License

### Kerberos Factory (part of [Kerberos Enterprise suite](https://doc.kerberos.io/enterprise/first-things-first/))

- Installation on top of Kubernetes (K8S).
- Camera support for IP camera only (RTSP/H264).
- Massive horizontal scaling, thanks to Kubernetes.
- Management of multiple Kerberos Agents through a single pane of glass.
- Low memory and CPU intensive.
- Modular and extensible design for building own extensions and integrations (e.g. a video analytics platform).
- Commercial licensed and closed source.

## How it works: A world of Agents 🕵🏼‍♂️

Kerberos.io applies the concept of agents. An agent is running next to or on your camera, and is processing a single camera feed. It applies motion based recording and make those recordings available through a user friendly web interface. Kerberos Agent allows you to connect to other cloud services or custom applications. Kerberos Agent is perfect for personal usage and/or is a great tool if you only have a couple of surveillance cameras to be processed.

If you are looking for a solution that scales better with your video surveillance and/or video analytics requirements, [Kerberos Enterprise Suite might be a better fit](https://doc.kerberos.io/enterprise/first-things-first).

## Installation
Kerberos Agent **will ship in different formats**: Docker, binary, snap, KiOS. Version 3 is still in active development right now, and not yet released.

## Run and develop

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
    go run main.go run mycameraname 8080

This will launch the Kerberos Agent and run a webserver on port `8080`. You can change the port by your own preference. We strongly support the usage of [Goland](https://www.jetbrains.com/go/) or [Visual Studio Code](https://code.visualstudio.com/), as it comes with all the debugging and linting features builtin.

![run-in-goland](https://user-images.githubusercontent.com/1546779/111139940-0a4d1a80-8582-11eb-8985-ceaf7359f4ee.gif)

## Building for Production

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

Inside the root of this `agent` repository, you will find a `Dockerfile`. This file contains the instructions for building and shipping **Kerberos Agent**. Important to note is that start from a prebuild base image, `kerberos/debian-opencv-ffmpeg:1.0.xxx`.
This base image contains already a couple of tools, such as Golang, FFmpeg and OpenCV. We do this for faster compilation times.

By running the `docker build` command, you will create the Kerberos Agent Docker image. After building you can simply run the image as a Docker container.

    docker build -t kerberos/agent .

## Running as a container

We are creating Docker images as part of our CI/CD process. You'll find our Docker images on [Docker hub](https://hub.docker.com/r/kerberos/agent). Pick a specific tag of choice, or use latest. Once done run below command, this will open the web interface of your Kerberos agent on port 8080.  
    
    docker run -p 8080:8080 --name mycamera -d kerberos/agent:latest

Or for a develop build:

    docker run -p 8080:8080 --name mycamera -d kerberos/agent-dev:latest

Feel free to use another port if your host system already has a workload running on `8080`. For example `8082`.

    docker run -p 8082:8080 --name mycamera -d kerberos/agent:latest

## Attach a volume

By default your Kerberos agent will store all its configuration and recordings inside the container. It might be interesting to store both configuration and your recordings outside the container, on your local disk. This helps persisting your storage even after you decide to wipe out your Kerberos agent.

You attach a volume to your container by leveraging the `-v` option. To mount your own configuration file, execute as following:

1. Decide where you would like to store your configuration and recordings; create a new directory for the config file and recordings folder accordingly.

        mkdir agent
        mkdir agent/config
        mkdir agent/recordings

2. Once you have located your desired directory, copy the latest [`config.json`](https://github.com/kerberos-io/agent/blob/master/machinery/data/config/config.json) file into your config directory.

        wget https://raw.githubusercontent.com/kerberos-io/agent/master/machinery/data/config/config.json -O agent/config/config.json

3. Run the docker command as following to attach your config directory and recording directory.

        docker run -p 8080:8080 --name mycamera -v $(pwd)/agent/config:/home/agent/data/config  -v $(pwd)/agent/recordings:/home/agent/data/recordings -d kerberos/agent:latest

## FAQ

#### 1. Why a mono repo?

We have noticed in the past (v1 and v2) splitting the repositories (machinery and web), created a lot of confusion within our community. People didn't understand the different versions and so on. This caused a lack of collaboration, and made it impossible for some people to collaborate and contribute.

Having a mono repo, which is well organised, simplifies the entry point for new people who would like to use, understand and/or contribute to Kerberos Agent.

#### 2. Why a change in technologies?

In previous versions (v1 and v2) we used technologies like C++, PHP and BackboneJS. 7 years ago this was still acceptable, however time has changed and new technologies such as React and Golang became very popular.

Due to previous reason we have decided to rebuild the Kerberos Agent technology from scratch, taking into account all the feedback we acquired over the years. Having these technologies available, we will enable more people to contribute and use our technology.

#### 3. What is the difference with Kerberos Enterprise?

We started the developments of Kerberos Enterprise a year ago (January, 2020), our focus here was scalability, and fast development and easy deployment. We noticed that with technologies such as Golang and React, we can still provide a highly performant video surveillance system.

Kerberos Agent uses the same technology stack, and some code pieces, of Kerberos Enterprise which we have already build. We have a very clear now, of how a well developed and documented video surveillance system needs to look like.

#### 4. When are we going to be able to install the first version?

We plan to ship the first version by the end of Q1, afterwards we will add more and more features as usual.

#### 5. Change in License

Kerberos Agent (v3) is now available under the MIT license.

## Contributors

This project exists thanks to all the people who contribute.

<a href="https://github.com/kerberos-io/agent/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=kerberos-io/agent" />
</a>
