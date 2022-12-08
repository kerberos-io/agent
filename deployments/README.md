# Kerberos Agent Deployments

Great to see you here, you just arrived at the real stuff! As you may have understood Kerberos Agent is a containerized solution. A Kerberos Agent, or container equivalent, is running for each camera. This approach makes it scalable, isolated and probably the most important thing an exceptional workload governance.

Due to it's nature, of acting as a micro service, there are many different ways how to get this Kerberos Agent up and running. This part of the Kerberos Agent repository contains example configurations, for all the different deployments and automations you can leverage to deploy and scale your video landscape.

We will discuss following deployment models.

- [1. Docker](#1-docker)
- [2. Docker Compose](#2-docker-compose)
- [3. Kubernetes](#3-kubernetes)
- [4. RedHat Ansible and OpenShift](#4-redhat-ansible-and-openshift)
- [5. Kerberos Factory](#5-kerberos-factory)
- [6. Terraform](#6-terraform)
- [7. Salt](#7-salt)

## 1. Docker

Leveraging `docker` is probably one of the easiest way to run and test the Kerberos Agent. Thanks to it's multi-architecture images you could run it on almost every machine. The `docker` approach is perfect for running one or two cameras in a (single machine) home deployment, a POC to verify its capabilities, or testing if your old/new IP camera is operational with our Kerberos Agent.

> Learn more [about Kerberos Agent on Docker here](https://github.com/kerberos-io/agent/tree/master/deployments/docker#1-running-a-single-container).

## 2. Docker Compose

If you consider `docker` as "your way to go", but require to run a bigger (single machine) deployment at home or inside your store then `docker compose` would be more suitable. By specifying a single `docker-compose.yaml` file, you can define all your Kerberos Agents (and thus cameras) in a single file, with a custom configuration to fit your needs.

> Learn more [about Kerberos Agent with Docker Compose here](https://github.com/kerberos-io/agent/tree/master/deployments/docker#2-running-multiple-containers-with-docker-compose).

## 3. Kubernetes

To be written

## 4. RedHat Ansible and OpenShift

To be written

## 5. Kerberos Factory

To be written

## 6. Terraform

To be written

## 7. Salt

To be written
