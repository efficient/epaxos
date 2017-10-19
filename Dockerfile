FROM golang

WORKDIR /app

ADD https://api.github.com/repos/otrack/epaxos/git/refs/heads/master epaxos-version.json
RUN git clone https://github.com/otrack/epaxos
RUN GOPATH=/app/epaxos go get -u github.com/go-redis/redis
RUN GOPATH=/app/epaxos go get -u github.com/google/uuid
RUN GOPATH=/app/epaxos go install master
RUN GOPATH=/app/epaxos go install server
RUN GOPATH=/app/epaxos go install client

ENV TYPE master
ENV MADDR localhost
ENV MPORT 7087
ENV NREPLICAS 1
ENV SPORT 7001

CMD ["bash", "/app/epaxos/bin/run.sh"]
