#!/bin/bash

DIR=`pwd $(dirname "$0")`
rm -f epaxos.jar
GOPATH=${DIR} go get github.com/sridharv/gojava
GOPATH=${DIR} gojava -v -o epaxos.jar build bindings
mvn install:install-file -Dfile=epaxos.jar -DgroupId=epaxos -DartifactId=epaxos -Dversion=1.0 -Dpackaging=jar
