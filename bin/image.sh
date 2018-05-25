#!/usr/bin/env bash

if [ -z "$1" ]; then
    TAG=latest
else
    TAG="$1"
fi

DOCKER_USER=$(docker info |
    grep Username |
    awk '{print $2}')
if [ -z "${DOCKER_USER}" ]; then
    DOCKER_USER=vitorenesduarte
fi

DIR=$(dirname "$0")
IMAGE=${DOCKER_USER}/epaxos:${TAG}
DOCKERFILE=${DIR}/../Dockerfile

# epaxos version: last commit hash
git log -1 --format="%H" >${DIR}/../epaxos-version

# build image
docker build \
    --no-cache \
    -t "${IMAGE}" -f "${DOCKERFILE}" .

# push image
docker push "${IMAGE}"
