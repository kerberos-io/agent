# Kerberos Agent Deployments

Great to see you here, you just arrived at the real stuff! As you may have understood Kerberos Agent is a containerized solution. A Kerberos Agent, or container equivalent, is running for each camera. This approach makes it scalable, isolated and probably the most important thing an exceptional workload governance.

Due to it's nature, of acting as a micro service, there are many different ways how to get this Kerberos Agent up and running. This part of the Kerberos Agent repository contains example configurations, for all the different deployments and automations you can leverage to deploy and scale your video landscape.

We will discuss following deployment models.

- [0. Static binary](#0-static-binary)
- [1. Docker](#1-docker)
- [2. Docker Compose](#2-docker-compose)
- [3. Kubernetes](#3-kubernetes)
- [4. Red Hat Ansible and OpenShift](#4-red-hat-ansible-and-openshift)
- [5. Kerberos Factory](#5-kerberos-factory)
- [6. Terraform](#6-terraform)
- [7. Salt](#7-salt)
- [8. Balena](#8-balena)

## 0. Static binary

Kerberos Agents are now also shipped as static binaries. Within the Docker image build, we are extracting the Kerberos Agent binary and are [uploading them to the releases page](https://github.com/kerberos-io/agent/releases) in the repository. By opening a release you'll find a `.tar` with the relevant files.

> Learn more [about the Kerberos Agent binary here](https://github.com/kerberos-io/agent/tree/master/deployments/binary).

## 1. Docker

Leveraging `docker` is probably one of the easiest way to run and test the Kerberos Agent. Thanks to it's multi-architecture images you could run it on almost every machine. The `docker` approach is perfect for running one or two cameras in a (single machine) home deployment, a POC to verify its capabilities, or testing if your old/new IP camera is operational with our Kerberos Agent.

> Learn more [about Kerberos Agent on Docker here](https://github.com/kerberos-io/agent/tree/master/deployments/docker#1-running-a-single-container).

## 2. Docker Compose

If you consider `docker` as "your way to go", but require to run a bigger (single machine) deployment at home or inside your store then `docker compose` would be more suitable. By specifying a single `docker-compose.yaml` file, you can define all your Kerberos Agents (and thus cameras) in a single file, with a custom configuration to fit your needs.

> Learn more [about Kerberos Agent with Docker Compose here](https://github.com/kerberos-io/agent/tree/master/deployments/docker#2-running-multiple-containers-with-docker-compose).

## 3. Kubernetes

As described above, `docker` is a great tool for smaller deployments, where you are just running on a single machine and want to ramp up quickly. As you might expect, this is a not an ideal situation for production deployments. Kubernetes can help you to build a scalable, flexible and resilient deployment.

> Learn more [about Kerberos Agent in a Kubernetes cluster here](https://github.com/kerberos-io/agent/tree/master/deployments/kubernetes).

## 4. Red Hat Ansible and OpenShift

If you running an alternative distribution such as Red Hat OpenShift, things will work out exactly as mentioned before with the `Kubernetes` deployment. You'll have all the benefints of Red Hat OpenShift on top. One of the things we provide here is an Ansible playbook to deploy the Kerberos Agent in the OpenShift cluster.

> Learn more [about Kerberos Agent in OpenShift with Ansible](https://github.com/kerberos-io/agent/tree/master/deployments/ansible-openshift).

## 5. Kerberos Factory

All of the previously deployments, `docker`, `kubernetes` and `openshift` are great for a technical audience. However for business users, it might be more convenient to have a clean web ui, that one can leverage to add one or more cameras (Kerberos Agents), without the hassle of the technical resources.

> Learn more [about Kerberos Agent with Kerberos Factory](https://github.com/kerberos-io/agent/tree/master/deployments/factory).

## 6. Terraform

To be written

## 7. Salt

To be written

## 8. Balena

Balena Cloud provide a seamless way of building and deploying applications at scale through the conceps of `blocks`, `apps` and `fleets`. Once you have your `app` deployed, for example our Kerberos Agent, you can benefit from features such as: remote access, over the air updates, an encrypted public `https` endpoint and many more.

Together with the Balena.io team we've build a Balena App, called [`video-surveillance`](https://hub.balena.io/apps/2064752/video-surveillance), which any can use to deploy a video surveillance system in a matter of minutes with all the expected management features you can think of.

> Learn more [about Kerberos Agent with Balena](https://github.com/kerberos-io/agent/tree/master/deployments/balena).
