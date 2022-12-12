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

## Configure with volumes

Just like with `docker`, you can also attach `volumes` to the Kerberos Agent deployment, by creating a `Persistent Volume` and mount it to a specific directory.

Depending on where and how you are hosting the Kubernetes cluster, you may need to create a new `storageClass` or use a predefined `storageClass` from your cloud provider (Azure, GCP, AWS, ..). Have a look at `deployment-agent-volume.yml` to review a complete example.

    template:
    metadata:
        labels:
        app: agent
    spec:
        volumes:
        - name: kerberos-data
            persistentVolumeClaim:
            claimName: kerberos-data
        ...
        containers:
        - name: agent
          image: kerberos/agent:latest
          volumeMounts:
            - name: kerberos-data
              mountPath: /home/agent/data/config
              subPath: config
        ...

## Expose with Ingress

In the first example `deployment-agent.yml` we are using a `LoadBalancer` to expose the Kerberos Agent user interface; as shown below. If you are a bit more experienced with Kubernetes, you will know there are other `service types` as well.

    ---
    apiVersion: v1
    kind: Service
    ...
    type: LoadBalancer
    ports:
        - port: 80
    ...

An alternative to `LoadBalancer` is `Ingress`. By leveraging an ingress such as `ingress-nginx` or `traefik` you setup a gateway (single point of contact), through which all communication to your apps (services) will flow.

A huge benefit (there are many others), is that you only allocate 1 public IP address for all your services. So instead of creating a `LoadBalancer` and thus a public IP address for every agent, you will create an `Ingress` service for each agent. Review the complete example at `deployment-agent-with-ingress.yml`.

    apiVersion: networking.k8s.io/v1
    kind: Ingress
    metadata:
    name: agent-ingress
    labels:
        name: agent-ingress
    annotations:
        kubernetes.io/ingress.class: nginx
        kubernetes.io/tls-acme: "true"
        nginx.ingress.kubernetes.io/ssl-redirect: "true"
        cert-manager.io/cluster-issuer: "letsencrypt-prod"
    spec:
    tls:
        - hosts:
            - "myagent.kerberos.io"
        secretName: agent-secret
    rules:
    - host: myagent.kerberos.io
        http:
        paths:
        - pathType: Prefix
            path: "/"
            backend:
            service:
                name: agent-svc
                port:
                number: 80
