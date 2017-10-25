#!/usr/bin/env bash

if [ -z "$1" ]; then
  TAG=latest
else
  TAG="$1"
fi

DOCKER_USER=$(docker info |
              grep Username |
              awk '{print $2}')
GIT_USER=$(git config remote.origin.url |
           awk -F/ '{ print $4 }')

DIR=$(dirname "$0")
IMAGE=${DOCKER_USER}/epaxos:${TAG}
DOCKERFILE=${DIR}/../Dockerfile

# build image
docker build \
  --build-arg user=${GIT_USER} \
  --no-cache \
  -t "${IMAGE}" -f "${DOCKERFILE}" .

# push image
docker push "${IMAGE}"
