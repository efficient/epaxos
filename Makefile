
STOREDIR = $(CURDIR)/storage
FLAGS    = -ldflags "-X genericsmr.storage=$(STOREDIR)"

all: compile

deps:
	GOPATH=`pwd` go get github.com/google/uuid
	GOPATH=`pwd` go get github.com/emirpasic/gods/maps/treemap

compile: | $(STOREDIR)
compile: deps
	GOPATH=`pwd` go install $(FLAGS) master
	GOPATH=`pwd` go install $(FLAGS) server
	GOPATH=`pwd` go install $(FLAGS) client

$(STOREDIR):
	mkdir -p $(STOREDIR)

race: FLAGS += -race
race: compile

test: compile
	./bin/test.sh
