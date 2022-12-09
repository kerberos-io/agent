# Deployment with Kubernetes

As described in the [Deployment with Docker](https://github.com/kerberos-io/agent/tree/master/deployments/docker), `docker` is a great tool for smaller deployments, where you are just running on a single machine and want to ramp up quickly. As you might expect, this is a not an ideal situation for production deployments.

Kubernetes can help you to build a scalable, flexible and resilient deployment. By introducing the concept of multi-nodes and deployments, you can make sure your Kerberos Agents are evenly distributed across your different machines, and you can add more nodes when you need to scale out.

We've provided an example deployment `deployment-agent.yml` in this directory, which show case you have to create a deployment (and under the hood a pod), to run a Kerberos Agent workload.

## Create a Kerberos Agent deployment

It's always a best practices to isolate and structure your workloads in Kubernetes. To achieve this we are utilising the concept of namespaces. For this example we will create a new namespace `demo`.

    kubectl create namespace demo

Now we have a namespace, have a look at `deployment-agent.yml` in this folder. This configuration file describes the Kubernetes resources we would like to create, and how the Kerberos Agent needs to behave: environment variables, container ports, etc. At the bottom of the file, we find a `service` part, this tells Kubernetes to expose the Kerberos Agent user interface on a publicly accessible IP address. **_Please note that you don't need to expose this, as you can configure the Kerberos Agent with a volume and/or environment variables._**

Let's move on, and apply the Kerberos Agent deployment and service.

    kubectl apply -f deployment-agent.yml -n demo

Watch deployment and service to be ready.

    watch kubectl get all -n demo

When the deployment and service is created successfully, you should see something like this.

    Every 2.0s: kubectl get all -n demo                     Fri Dec  9 16:33:17 2022

    NAME                         READY   STATUS    RESTARTS   AGE
    pod/agent-7c75c4dbcf-zxrb5   1/1     Running   0          19s

    NAME                TYPE           CLUSTER-IP    EXTERNAL-IP       PORT(S)        AGE
    service/agent-svc   LoadBalancer   10.x.x.x   108.x.x.x   80:32664/TCP   20s

    NAME                    READY   UP-TO-DATE   AVAILABLE   AGE
    deployment.apps/agent   1/1     1            1           20s

    NAME                               DESIRED   CURRENT   READY   AGE
    replicaset.apps/agent-7c75c4dbcf   1         1         1       20s

When copying the `EXTERNAL-IP` and pasting it in your browser, you should see the Kerberos Agent user interface. You can use [the default username and password to sign-in](https://github.com/kerberos-io/agent#access-the-kerberos-agent), or if changed to your own (which is recommended).
