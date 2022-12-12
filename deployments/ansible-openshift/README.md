# Deploy to a Red Hat OpenShift cluster with Ansible

Kubernetes is great, but you might love OpenShift even more. In this directory you'll find some resources to deploy your Kerberos Agent in an OpenShift cluster using Ansible playbook. We'll review the different tasks of the Ansible playbook step by step; find the complete `playbook.yaml` here.

## Variabeles

We'll have a few `variables` in our `playbook.yml` that will help us to setup secure connection with the OpenShift cluster. We need the `cluster_url` and the `username` and `password` of the OpenShift cluster. If you don't know where to find this, you can find this in the OpenShift web ui.

    vars:
    - oc_cluster_url: ""
    - oc_username: ""
    - oc_password: ""

## Tasks

Once we have supplied the `variables` we will define following tasks:

    - name: Print Variables
    - name: Try to login to OCP cluster
    - name: Create a Namespace
    - name: Create a Persistent volume claim
    - name: Deploy Kerberos Agent
    - name: Expose Kerberos Agent

1. Print variables: this is a validation step, where we make sure we have the correct variables supplied to the `ansible-playbook` command. This confirms we are using the right credentials to setup a secure connection with the OpenShift cluster.

2. Setup a connection with OpenShift using the defined variabeles. If successfull an `api_key` will become available in the `k8s_auth_result` variable. This variabele will be used with every subsequent operation against the OpenShift cluster.

3. A best practice is to isolate your workloads in namespaces. Therefore we'll create a new namespace in our OpenShift cluster.

4. (Optional) Create a persistent volume to persist the configuration file and recordings in a volume.

5. Deploy Kerberos Agent through a `deployment`.

6. Expose the Kerberos Agent web interface through a `LoadBalancer`; public internet accessible IP address.

## Run the playbook

Now you understand what is happening in the playbook, let's run it. Make sure you have `ansible` install on your `host` or `deploy` machine.

Specify the `environment` input variable as a `JSON` with all required variables defined in step 1. Reference the `playbook.yml` file and execute.

    ansible-playbook  -e '{ \
    "oc_cluster_url":"https://api.j5z0adui.westeurope.aroapp.io:6443", \
    "oc_username":"kubeadmin",\
    "oc_password":"xxx" \
    }' playbook.yml

If everything runs as expected you should see you Kerberos Agent deployed, together with an assigned public ip address. Paste the ip address in your browser, the Kerberos Agent web interface will show up. You can use [the default username and password to sign-in](https://github.com/kerberos-io/agent#access-the-kerberos-agent), or if changed to your own (which is recommended).
