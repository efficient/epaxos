FROM golang

WORKDIR /app

ADD . /app

RUN GOPATH=/app go install master
RUN GOPATH=/app go install server
RUN GOPATH=/app go install client

ENV TYPE master
ENV MPORT 7087
ENV NREPLICAS 1
ENV SPORT 7001
ENV MADDR localhost 

CMD ["bash", "/app/bin/run.sh"]
