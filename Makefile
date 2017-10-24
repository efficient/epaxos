all: compile

deps:
	go get github.com/google/uuid

compile: deps
	cd src/
	go install master
	go install server
	go install client
	cd -

test: compile
	test.sh
