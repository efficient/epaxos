#!/usr/bin/env bash

# install https://github.com/mvdan/sh

DIR=$(dirname "${BASH_SOURCE[0]}")

for f in $(ls -d ${DIR}/*.sh); do
    shfmt -i 4 -w ${f}
done
