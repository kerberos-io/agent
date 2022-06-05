export version=0.0.1
export name=agent

docker build -t $name .

docker tag $name kerberos/$name:$version
docker push kerberos/$name:$version

docker tag $name kerberos/$name:latest
docker push kerberos/$name:latest
