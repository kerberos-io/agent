# Deployment with Balena

Balena Cloud provide a seamless way of building and deploying applications at scale through the conceps of `blocks`, `apps` and `fleets`. Once you have your `app` deployed, for example our Kerberos Agent, you can benefit from features such as: remote access, over the air updates, an encrypted public `https` endpoint and many more.

We provide two mechanisms to deploy Kerberos Agent to a Balena Cloud fleet:

1. Use Kerberos Agent as [a block part of your larger application](https://github.com/kerberos-io/balena-agent-block).
2. Use Kerberos Agent as [a stand-a-lone application](https://github.com/kerberos-io/balena-agent).

## Block

Within Balena you can build the concept of a block, which is the equivalent of container image or a function in a typical programming language.

The idea of blocks, you can find a more thorough explanation [here](https://docs.balena.io/learn/develop/blocks/), is that you can compose and combine multiple `blocks` to level up to the concept an `app`.

You as a developer can choose which `blocks` you would like to use, to build the desired `application` state you prefer. For example you can use the [Kerberos Agent block](https://hub.balena.io/blocks/2064662/agent) to compose a video surveillance system as part of your existing set of blocks.

You can the `Kerberos Agent` block by defining following elements in your `compose` file.

    agent:
        image: bh.cr/kerberos_io/agent
