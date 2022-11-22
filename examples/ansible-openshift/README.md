# Deploy to a Red Hat OpenShift cluster using ansible

Eithin this directory you'll find an Ansible playbook to install a Kerberos Agent to an existing OpenShift cluster. By providing the cluster url, username and password, you will be able to create several resources in your cluster.

    ansible-playbook  -e '{"oc_cluster_url":"https://api.j5z0adui.westeurope.aroapp.io:6443", "oc_username":"kubeadmin", "oc_password":"xxx"}' playbook.yml 