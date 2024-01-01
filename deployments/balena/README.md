# Deployment with Balena

Balena Cloud provide a seamless way of building and deploying applications at scale through the conceps of `blocks`, `apps` and `fleets`. Once you have your `app` deployed, for example our Kerberos Agent, you can benefit from features such as: remote access, over the air updates, an encrypted public `https` endpoint and many more.

We provide two mechanisms to deploy Kerberos Agent to a Balena Cloud fleet:

1. Use Kerberos Agent as [a block part of your application](https://github.com/kerberos-io/balena-agent-block).
2. Use Kerberos Agent as [a stand-alone application](https://github.com/kerberos-io/balena-agent).

## Block

Within Balena you can build the concept of a block, which is the equivalent of container image or a function in a typical programming language. The idea of blocks, you can find a more thorough explanation [here](https://docs.balena.io/learn/develop/blocks/), is that you can compose and combine multiple `blocks` to level up to the concept an `app`.

You as a developer can choose which `blocks` you would like to use, to build the desired `application` state you prefer. For example you can use the [Kerberos Agent block](https://hub.balena.io/blocks/2064662/agent) to compose a video surveillance system as part of your existing set of blocks.

You can the `Kerberos Agent` block by defining following elements in your `compose` file.

    agent:
        image: bh.cr/kerberos_io/agent

## App

Next to building individual `blocks` you as a developer can also decide to build up an application, composed of one or more `blocks` or third-party containers, and publish it as an `app` to the Balena Hub. This is exactly [what we've done..](https://hub.balena.io/apps/2064752/video-surveillance)

On Balena Hub we have created the []`video-surveillance` application](https://hub.balena.io/apps/2064752/video-surveillance) that utilises the [Kerberos Agent `block`](https://hub.balena.io/blocks/2064662/agent). The idea of this application is that utilises the foundation of our Kerberos Agent, but that it might include more `blocks` over time to increase and improve functionalities from other community projects.

To deploy the application you can simply press below `Deploy button` or you can navigate to the [Balena Hub apps page](https://hub.balena.io/apps/2064752/video-surveillance).

[![deploy with balena](https://balena.io/deploy.svg)](https://dashboard.balena-cloud.com/deploy?repoUrl=https://github.com/kerberos-io/agent)

You can find the source code, `balena.yaml` and `docker-compose.yaml` files in the [`balena-agent` repository](https://github.com/kerberos-io/balena-agent).
