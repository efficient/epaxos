FROM golang

ENV NAME=/epaxos

COPY src $NAME/src/
COPY bin $NAME/bin/
COPY Makefile epaxos-version $NAME/

WORKDIR $NAME
RUN make compile

ENV TYPE master
ENV MADDR localhost
ENV MPORT 7087
ENV NREPLICAS 1
ENV SPORT 7001

CMD ["bash", "bin/run.sh"]
