# Kerberos Open Source

<a href="https://doc.kerberos.io"><img src="https://img.shields.io/badge/kerberos-opensource-gray.svg?longCache=true&colorB=brightgreen" alt="Kerberos Open Source"></a>
<a href="https://twitter.com/kerberosio?ref_src=twsrc%5Etfw"><img src="https://img.shields.io/twitter/url.svg?label=Follow%20%40kerberosio&style=social&url=https%3A%2F%2Ftwitter.com%2Fkerberosio" alt="Twitter Widget"></a>
<br>
<a href="https://circleci.com/gh/kerberos-io/opensource"><img src="https://circleci.com/gh/kerberos-io/opensource.svg?style=svg"/></a>
<a href="https://travis-ci.org/kerberos-io/opensource"><img src="https://travis-ci.org/kerberosio/opensource.svg?branch=master" alt="Build Status"></a>
<a href="https://pkg.go.dev/github.com/kerberos-io/opensource/v3"><img src="https://pkg.go.dev/badge/github.com/kerberos-io/opensource/v3" alt="PkgGoDev"></a>
<a href="https://codecov.io/gh/kerberos-io/opensource"><img src="https://codecov.io/gh/kerberos-io/opensource/branch/master/graph/badge.svg" alt="Coverage Status"></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/License-Commons Clause-yellow.svg" alt="License: Commons Clause"></a>


[![Kerberos.io - video surveillance](https://kerberos.io/images/kerberos.png)](https://kerberos.io)


[**Docker Hub**](https://hub.docker.com/r/kerberos/opensource) | [**Documentation**](https://doc.kerberos.io)

Kerberos Open source (v3) is a cutting edge video surveillance management system made available as Open source (Apache 2.0) with a Commons Clause License on top. This means that all the source code is available for you or your company, and you can use and transform it as long it is for non commercial usage. Read more [about the license here](LICENSE).

## Previous releases

This repository contains the next generation of Kerberos.io, **Kerberos Open Source (v3)**, and is the successor of the machinery and web repositories. A switch in technologies and architecture has been made. This version is still under active development and can be followed on the [develop branch](https://github.com/kerberos-io/opensource/tree/develop) and [project overview](https://github.com/kerberos-io/opensource/projects/1). Kerberos Open Source (v3) is not yet released.

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
- Non-commercial usage - Commons Clause License (Apache 2.0)

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

## Run

Kerberos Open Source is divided in two parts a backend` and `frontend`. Both parts live in this repository in their relative folders. 

### Backend

The `backend` is a **Golang** project which delivers two functions: it acts as the Kerberos Agent, and does the camera processing, on the other hand it acts as a webserver that communicaties directly with the front-end.

You can simply run the `backend` using following command.

    git clone https://github.com/kerberos-io/opensource
    cd backend
    go run main.go run mycamername 8080
    
 This will launch the Kerberos Agent and run a webserver on port `8080`. You can change the port by your own preference.
 
 ---
 
 ### Frontend
 
 The `frontend` is a **React** project which is the main entry point for an end user to view recordings, a livestream, and modify the configuration of the `backend`
     
    git clone https://github.com/kerberos-io/opensource
    cd frontend
    yarn start

 This will start a webserver on port `3000`.
 
 #### Build
 
 After making changes you can run the `yarn build` command, this will create a build artifact and move it to the `backend/www` folder. By restarting the backend and navigating to `8080` you will see the React webpage (including your changes) visualised.
  

 ## FAQ
 
 #### 1. Why a mono repo?
 
 We have noticed in the past (v1 and v2) the splitting the repositories (machinery and web), created a lot of confusion within our community. People didn't understand the different versions and so on. This caused a lack of collaboration, and made it impossible for some people to collaborate. 
 
 Having a mono repo, which is well organised, simplifies the entry point for new people who would like to understand and/or contribute to Kerberos Open Source.
 
 #### 2. Why a change in technologies?
 
 In previous version (v1 and v2) we used technologies like C++, PHP and BackboneJS. 7 years ago this was still acceptable, however time has changed and new technologies such as React and Golang became very popular.
 
 Due to previous reason we have decided to rebuild the Kerberos Open Source technology from scratch, taking into account all the feedback we acquired over the years. Having these technologies available, we will enable more people to contribute and use our technology.

#### 3. What is the difference with Kerberos Enterprise?

We started the developments of Kerberos Enterprise a year ago, our main focus here was scalability, and fast development and easy deployment. We noticed that with technologies such as Golang and React, we can still provide a highly performant video surveillance system.

Kerberos Open Source will use the same technology stack, and some code pieces, of Kerberos Enterprise which we have already build. We have a very clear now, of how a well developed and documented video surveillance system needs to look like.