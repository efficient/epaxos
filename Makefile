all: compile

deps:
	 GOPATH=`pwd` go get github.com/google/uuid
	 GOPATH=`pwd` go get github.com/emirpasic/gods/maps/treemap

compile: deps
	GOPATH=`pwd` go install $(FLAGS) master
	GOPATH=`pwd` go install $(FLAGS) server
	GOPATH=`pwd` go install $(FLAGS) client

race: FLAGS += -race
race: compile

test: compile
	./bin/test.sh
