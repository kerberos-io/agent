# Kerberos Open Source

<a href="https://doc.kerberos.io"><img src="https://img.shields.io/badge/kerberos-opensource-gray.svg?longCache=true&colorB=brightgreen" alt="Kerberos Open Source"></a>
<a href="https://twitter.com/kerberosio?ref_src=twsrc%5Etfw"><img src="https://img.shields.io/twitter/url.svg?label=Follow%20%40kerberosio&style=social&url=https%3A%2F%2Ftwitter.com%2Fkerberosio" alt="Twitter Widget"></a>
<br>
<a href="https://circleci.com/gh/kerberos-io/opensource"><img src="https://circleci.com/gh/kerberos-io/opensource.svg?style=svg"/></a>
<a href="https://travis-ci.org/kerberos-io/opensource"><img src="https://travis-ci.org/kerberosio/opensource.svg?branch=master" alt="Build Status"></a>
<a href="https://pkg.go.dev/github.com/pion/webrtc/v3"><img src="https://pkg.go.dev/badge/github.com/kerberos-io/opensource/v3" alt="PkgGoDev"></a>
<a href="https://codecov.io/gh/kerberos-io/opensource"><img src="https://codecov.io/gh/kerberos-io/opensource/branch/master/graph/badge.svg" alt="Coverage Status"></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/License-Commons Clause-yellow.svg" alt="License: Commons Clause"></a>

[**Docker Hub**](https://hub.docker.com/r/kerberos/opensource) | [**Documentation**](https://doc.kerberos.io)

Kerberos Open source (v3) is a cutting edge video surveillance management system made available as Open source (Apache 2.0) with a Commons Clause License on top. This means that all the source code is available for you or your company, and you can use and transform it as long it is for non commercial usage. Read more [about the license here](LICENSE).

## Previous releases

This repository contains the next generation of Kerberos.io, **Kerberos Open Source (v3)**, and is the successor of the machinery and web repositories. A switch in technologies and architecture has been made.

This version is still under active development and can be followed on the [develop branch](https://github.com/kerberos-io/opensource/tree/develop) and [project overview](https://github.com/kerberos-io/opensource/projects/1).

![Kerberos version 2 vs version 3](images/kerberos-agent-v2-v3.png)

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

Kerberos.io applies the concept of agents. An agent is running next to or on your camera, and is processing a single camera feed. It applies motion based recording and make those recordings available through a user friendly web interface. Kerberos Open Source allows you to connect to other cloud services or custom applications. Kerberos Open Source is perfect for personal usage and/or is a great tool you only have a couple of surveillance cameras to be managed.

If you are looking for a solution that scales with your video surveillance or video analytics well, [Kerberos Enterprise might be a better fit](https://doc.kerberos.io/enterprise/introduction).

## Installation
Kerberos Open Source comes with different installation flavours (it includes both the machinery and web repository). The reason is because depending on the use case one option is better than another. A short list of recommendations:

- KiOS: You have a Raspberry Pi, and you only want to run a Kerberos agent on it.
- Raspbian: You have a Raspberry Pi, but you want other services running next to the Kerberos agent.
- Docker: You have a lot of IP cameras, and/or don't want to mess with dependencies.
- Generic: You want to develop/extend Kerberos with your own features, or you want to run a Kerberos agent on a not supported OS/architecure.
