# icap-service-1

<h1 align="center">icap-service-1</h1>

<p align="center">
    <a href="https://github.com/k8-proxy/go-k8s-srv1/actions/workflows/build.yaml">
        <img src="https://github.com/k8-proxy/go-k8s-srv1/actions/workflows/build.yaml/badge.svg"/>
    </a>
    <a href="https://codecov.io/gh/k8-proxy/go-k8s-srv1">
        <img src="https://codecov.io/gh/k8-proxy/go-k8s-srv1/branch/main/graph/badge.svg"/>
    </a>	    
    <a href="https://goreportcard.com/report/github.com/k8-proxy/go-k8s-srv1">
      <img src="https://goreportcard.com/badge/k8-proxy/go-k8s-srv1" alt="Go Report Card">
    </a>
	<a href="https://github.com/k8-proxy/go-k8s-srv1/pulls">
        <img src="https://img.shields.io/badge/contributions-welcome-brightgreen.svg?style=flat" alt="Contributions welcome">
    </a>
    <a href="https://opensource.org/licenses/Apache-2.0">
        <img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="Apache License, Version 2.0">
    </a>
    <a href="https://github.com/k8-proxy/go-k8s-srv1/releases/latest">
        <img src="https://img.shields.io/github/release/k8-proxy/go-k8s-srv1.svg?style=flat"/>
    </a>
</p>

This is the service 1 of [this repo](https://github.com/k8-proxy/go-k8s-infra) that will get the file from icap-server and upload it to minio

### Steps of processing

When it starts

- Listens on the queue
- Get the file from the queue
- upload it to minio bucket
- Push the file details to the queue

## Configuration

- This pod need to mount the share storage mounted on icap server and that is how they will share the file together
- It's possible to have multiple replica of this service running. Only one will get the file and process it

### Docker build

- To build the docker image

```
git clone https://github.com/k8-proxy/go-k8s-srv1.git
cd k8-proxy/go-k8s-srv1
docker build -t <docker_image_name> .
```

### build

- First make sure that you have rabbitmq and minio running.
- For quick start using docker to run containers for RabbitMQ and MinIO.
- Run Standalone MinIO on Docker.

```
docker run -e "MINIO_ROOT_USER=<minio_root_user_name>" \
-e "MINIO_ROOT_PASSWORD=<minio_root_password>" \
-d -p 9000:9000 minio/minio server /data
```

- Run RabbitMQ on Docker.

```
docker run -d --hostname <host_name> --name <container_name> -p 15672:15672 -p 5672:5672 rabbitmq:3-management
```

- To run the container

```
docker run -e ADAPTATION_REQUEST_QUEUE_HOSTNAME='<rabbit-host>' \
-e ADAPTATION_REQUEST_QUEUE_PORT='<rabbit-port>' \
-e MESSAGE_BROKER_USER='<rabbit-user>' \
-e MESSAGE_BROKER_PASSWORD='<rabbit-password>' \
-e MINIO_ENDPOINT='<minio-endpoint>' \
-e MINIO_ACCESS_KEY='<minio-access>' \
-e MINIO_SECRET_KEY='<minio-secret>' \
-e MINIO_SOURCE_BUCKET='<bucket-to-upload-file>' \
-e MINIO_CLEAN_BUCKET='<bucket-to-upload-file>' \

--name <docker_container_name> <docker_image_name>
```

- Please note that in the previous command:

  - To get the variable `<rabbit-host>` run:

  ```
  docker inspect <rabbitmq_container_name>
  ```

  - It will be the value of key: `IPAddress`

# Testing steps

- Run the containers as mentionned above
- Be sure that there is a bucket with the same name you entered for `<bucket-to-upload-file>` among MinIO buckets.
- Publish data reference to rabbitMq on queue name : adaptation-request-queue with the following data(table):

* file-id : An ID for the file
* source-file-location : The full path to the file
* rebuilt-file-location : A full path representing the location where the rebuilt file will go to

- Check your container logs to see the processing

```
docker logs <container name>
```

# Rebuild flow to implement

![new-rebuild-flow-v2](https://github.com/k8-proxy/go-k8s-infra/raw/main/diagram/go-k8s-infra.png)
