# docker-reload

`docker-reload` builds the Dockerfile in the current directory and runs the image.
If any files change, it rebuilds and reruns the image.
Additionally, it will proxy HTTP requests to the container that it's running.
In this way, it's useful for developing web services inside of a Docker container.

## Installation

```shell
go get github.com/azylman/docker-reload
```

## Usage

`docker-reload` only takes one argument: a port mapping from your host to the port in the container that your service is listening on.

```
docker-reload -p 3000:3000
```
