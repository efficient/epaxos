all: compile

deps:
	 GOPATH=`pwd` go get github.com/google/uuid
	 GOPATH=`pwd` go get github.com/emirpasic/gods/maps/treemap

compile: deps
	GOPATH=`pwd` go install master
	GOPATH=`pwd` go install server
	GOPATH=`pwd` go install client

test: compile
	./bin/test.sh
