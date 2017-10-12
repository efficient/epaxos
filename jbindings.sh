#!/bin/bash
GOPATH=`pwd` go get github.com/sridharv/gojava
GOPATH=`pwd` gojava -v -o epaxos.jar build bindings
mvn install:install-file -Dfile=epaxos.jar -DgroupId=epaxos \
    -DartifactId=epaxos -Dversion=1.0 -Dpackaging=jar
