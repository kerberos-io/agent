# Kerberos Open Source

<a target="_blank" href="https://kerberos.io"><img src="https://img.shields.io/badge/kerberos-website-gray.svg?longCache=true&colorB=brightgreen" alt="Kerberos Open Source"></a>
<a target="_blank" href="https://doc.kerberos.io"><img src="https://img.shields.io/badge/kerberos-documentation-gray.svg?longCache=true&colorB=brightgreen" alt="Kerberos Open Source"></a>
<a target="_blank" href="https://twitter.com/kerberosio?ref_src=twsrc%5Etfw"><img src="https://img.shields.io/twitter/url.svg?label=Follow%20%40kerberosio&style=social&url=https%3A%2F%2Ftwitter.com%2Fkerberosio" alt="Twitter Widget"></a>
<a target="_blank" href="https://join.slack.com/t/kerberosio/shared_invite/zt-kfj36t7m-iLelioSPfg5~1e2qBBjhBw"><img src="https://img.shields.io/badge/join-slack-gray.svg?longCache=true&colorB=blue" alt="Kerberos Open Source"></a>

<a target="_blank" href="https://circleci.com/gh/kerberos-io/opensource"><img src="https://circleci.com/gh/kerberos-io/opensource.svg?style=svg"/></a>
<a target="_blank" href="https://travis-ci.org/kerberos-io/opensource"><img src="https://travis-ci.org/kerberos-io/opensource.svg?branch=master" alt="Build Status"></a>
<img src="https://github.com/kerberos-io/opensource/workflows/Go/badge.svg"/>
<img src="https://github.com/kerberos-io/opensource/workflows/React/badge.svg"/>
<img src="https://github.com/kerberos-io/opensource/workflows/CodeQL/badge.svg"/>

<a target="_blank" href="https://pkg.go.dev/github.com/kerberos-io/opensource/machinery"><img src="https://pkg.go.dev/badge/github.com/kerberos-io/opensource/machinery" alt="PkgGoDev"></a>
<a target="_blank" href="https://codecov.io/gh/kerberos-io/opensource"><img src="https://codecov.io/gh/kerberos-io/opensource/branch/master/graph/badge.svg" alt="Coverage Status"></a>
<a target="_blank" href="https://goreportcard.com/report/github.com/kerberos-io/opensource"><img src="https://goreportcard.com/badge/github.com/kerberos-io/opensource" alt="Coverage Status"></a>
<a target="_blank" href="https://app.codacy.com/gh/kerberos-io/opensource?utm_source=github.com&utm_medium=referral&utm_content=kerberos-io/opensource&utm_campaign=Badge_Grade"><img src="https://api.codacy.com/project/badge/Grade/83d79d3092c040acb8c51ee0dfddf4b9"/>
<a target="_blank" href="https://www.figma.com/proto/msuYC6sv2cOCqZeDtBxNy7/%5BNEW%5D-Kerberos.io-Apps?node-id=1%3A1788&viewport=-490%2C191%2C0.34553584456443787&scaling=min-zoom&page-id=1%3A2%3Ffuid%3D449684443467913607" alt="Kerberos Open Source"></a>

<a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>

[![Kerberos.io - video surveillance](https://kerberos.io/images/kerberos.png)](https://kerberos.io)

[**Docker Hub**](https://hub.docker.com/r/kerberos/opensource) | [**Documentation**](https://doc.kerberos.io) | [**Website**](https://kerberos.io)

Kerberos Open source (v3) is a cutting edge video surveillance management system made available as Open Source under the MIT License. This means that all the source code is available for you or your company, and you can use, transform and distribute the source code; as long you keep a reference of the original license. Kerberos Open Source (v3) can be used for commercial usage (which was not the case for v2). Read more [about the license here](LICENSE).

## Work In Progress

Kerberos Open Source (v3) is not yet released, and is actively developed. You can follow the progress [on our project board](https://github.com/kerberos-io/opensource/projects/1) and review our designs at [Figma](https://www.figma.com/proto/msuYC6sv2cOCqZeDtBxNy7/%5BNEW%5D-Kerberos.io-Apps?node-id=1%3A1788&viewport=-490%2C191%2C0.34553584456443787&scaling=min-zoom&page-id=1%3A2%3Ffuid%3D449684443467913607). Feel free to give any feedback.

## Previous releases

This repository contains the next generation of Kerberos.io, **Kerberos Open Source (v3)**, and is the successor of the machinery and web repositories. A switch in technologies and architecture has been made. This version is still under active development and can be followed on the [develop branch](https://github.com/kerberos-io/opensource/tree/develop) and [project overview](https://github.com/kerberos-io/opensource/projects/1).

Read more about this [at the FAQ](#faq) below.

<img src="https://github.com//kerberos-io/opensource/raw/master/.github/images/kerberos-agent-v2-v3.png" width="500px">

## Introduction

Kerberos.io is a cutting edge video surveillance system with a strong focus on user experience, scalability, resilience, extension and integration. Kerberos.io provides different solutions, but from a high level point of view it comes into two flavours: Kerberos Open Source and Kerberos Enterprise. The main differences, very brief, between Open Source and Enterprise are described below. Both Kerberos Open Source and Kerberos Enterprise can be extended and integrated with Kerberos Storage and/or Kerberos Cloud.

### Kerberos Open Source

- Installation in seconds (Kerberos Etcher, Docker, Binaries).
- Simplified and modern user interface.
- Multi architecture (ARMv7, ARMv8, amd64, etc).
- Multi camera support: IP Cameras (MJPEG/H264), USB cameras, Raspberry Pi Cameras.
- Single camera per instance (e.g. One Docker container per camera).
- Cloud integration through Webhooks, MQTT, etc.
- Cloud storage through Kerberos Cloud.
- MIT License

### Kerberos Enterprise

- Installation on top of Kubernetes (K8S).
- Camera support for IP camera only (RTSP/H264).
- Massive horizontal scaling, thanks to Kubernetes.
- Management of multiple Kerberos Agents through a single pane of glass.
- Low memory and CPU intensive.
- Modular and extensible design for building own extensions and integrations (e.g. a video analytics platform).
- Commercial licensed and closed source.

## How it works: A world of Agents 🕵🏼‍♂️

Kerberos.io applies the concept of agents. An agent is running next to or on your camera, and is processing a single camera feed. It applies motion based recording and make those recordings available through a user friendly web interface. Kerberos Open Source allows you to connect to other cloud services or custom applications. Kerberos Open Source is perfect for personal usage and/or is a great tool if you only have a couple of surveillance cameras to be processed.

If you are looking for a solution that scales with your video surveillance or video analytics well, [Kerberos Enterprise might be a better fit](https://doc.kerberos.io/enterprise/introduction).

## Installation
Kerberos Open Source **will ship in different formats**: Docker, binary, snap, KiOS. Version 3 is still in active development right now, and not yet released.

## Run and develop

Kerberos Open Source is divided in two parts a `machinery` and `web`. Both parts live in this repository in their relative folders. For development or running the application on your local machine, you have to run both the `machinery` and the `web` as described below. When running in production everything is shipped as only one artifact, read more about this at [Building for production](#building-for-production).

### Web

The `web` is a **React** project which is the main entry point for an end user to view recordings, a livestream, and modify the configuration of the `machinery`.

    git clone https://github.com/kerberos-io/opensource
    cd web
    yarn start

This will start a webserver and launches the web app on port `3000`.

### Machinery

The `machinery` is a **Golang** project which delivers two functions: it acts as the Kerberos Agent which is doing all the heavy lifting with camera processing and other kinds of logic, on the other hand it acts as a webserver (Rest API) that allows communication from the web (React) or any other custom application. The API is documented using `swagger`.

You can simply run the `machinery` using following commands.

    git clone https://github.com/kerberos-io/opensource
    cd machinery
    go run main.go run mycameraname 8080

This will launch the Kerberos Agent and run a webserver on port `8080`. You can change the port by your own preference.

## Building for Production

Running Kerberos Open Source in production only require a single binary to run. Nevertheless, we have two parts, the `machinery` and the `web`, we merge them during build time. So this is what happens.

### Web

To build the Kerberos Open Source web app, you simply have to run the `build` command of `yarn`. This will create a `build` directory inside the `web` directory, which contains a minified version of the React application. Other than that, we [also move](https://github.com/kerberos-io/opensource/blob/master/web/package.json#L16) this `build` directory to the `machinery` directory.

    cd web
    yarn build

### Machinery

Building the `machinery` is also super easy 🚀, by using `go build` you can create a single binary which ships it all; thank you Golang. After building you will endup with a binary called `main`, this is what contains everything you need to run Kerberos Open Source.

Remember the build step of the `web` part, during build time we move the build directory to the `machinery` directory. Inside the `machinery` web server [we reference the](https://github.com/kerberos-io/opensource/blob/master/machinery/src/routers/http/Server.go#L44) `build` directory. This makes it possible to just a have single web server that runs it all.  

    cd machinery
    go build

## Building for Docker

Inside the root of this `opensource` repository, you will find a `Dockerfile`. This file contains the instructions for building and shipping **Kerberos Open Source**. Important to note is that start from a prebuild base image, `kerberos/debian-opencv-ffmpeg:1.0.0`.
This base image contains already a couple of tools, such as Golang, FFmpeg and OpenCV. We do this for faster compilation times.

By running the `docker build` command, you will create the Kerberos Open Source Docker image. After building you can simply run the image as a Docker container.

    docker build -t kerberos/opensource .
    docker run -p 8080:8080 --name mycamera -d kerberos/opensource

## FAQ

#### 1. Why a mono repo?

We have noticed in the past (v1 and v2) splitting the repositories (machinery and web), created a lot of confusion within our community. People didn't understand the different versions and so on. This caused a lack of collaboration, and made it impossible for some people to collaborate and contribute.

Having a mono repo, which is well organised, simplifies the entry point for new people who would like to use, understand and/or contribute to Kerberos Open Source.

#### 2. Why a change in technologies?

In previous versions (v1 and v2) we used technologies like C++, PHP and BackboneJS. 7 years ago this was still acceptable, however time has changed and new technologies such as React and Golang became very popular.

Due to previous reason we have decided to rebuild the Kerberos Open Source technology from scratch, taking into account all the feedback we acquired over the years. Having these technologies available, we will enable more people to contribute and use our technology.

#### 3. What is the difference with Kerberos Enterprise?

We started the developments of Kerberos Enterprise a year ago (January, 2020), our focus here was scalability, and fast development and easy deployment. We noticed that with technologies such as Golang and React, we can still provide a highly performant video surveillance system.

Kerberos Open Source will use the same technology stack, and some code pieces, of Kerberos Enterprise which we have already build. We have a very clear now, of how a well developed and documented video surveillance system needs to look like.

#### 4. When are we going to be able to install the first version?

We plan to ship the first version by the end of Q1, afterwards we will add more and more features as usual.

#### 5. Change in License

Kerberos Open Source (v3) is now available under the MIT license.

## Contributors

This project exists thanks to all the people who contribute.

<a href="https://github.com/kerberos-io/opensource/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=kerberos-io/opensource" />
</a>
