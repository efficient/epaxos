#!/bin/bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

if [ "${TYPE}" == "" ];
then
    echo "usage: define env variables, as listed below
    TYPE = [master,server,client] # type of instance
    MPORT, MADDR # master instance
    MPORT, MADDR, ADDR, SPORT, SERVER_EXTRA_ARGS # server instance
    MPORT, MADDR, CLIENT_EXTRA_ARGS # client instance
    ";
    exit 0
fi;

# Usage of ./bin/master:
#   -N int
#     	Number of replicas. Defaults to 3. (default 3)
#   -port int
#     	Port # to listen on. Defaults to 7087 (default 7087)

if [ "${TYPE}" == "master" ];
then
    args="-port ${MPORT} -N ${NREPLICAS}"
    echo "master mode: ${args}"
    ${DIR}/master ${args};
fi;

# Usage of ./bin/server:
#   -addr string
#     	Server address (this machine). Defaults to localhost.
#   -beacon
#     	Send beacons to other replicas to compare their relative speeds.
#   -cpuprofile string
#     	write cpu profile to file
#   -dreply
#     	Reply to client only after command has been executed.
#   -durable
#     	Log to a stable store (i.e., a file in the current dir).
#   -e	Use EPaxos as the replication protocol. Defaults to false.
#   -exec
#     	Execute commands.
#   -g	Use Generalized Paxos as the replication protocol. Defaults to false.
#   -m	Use Mencius as the replication protocol. Defaults to false.
#   -maddr string
#     	Master address. Defaults to localhost.
#   -mport int
#     	Master port.  Defaults to 7087. (default 7087)
#   -p int
#     	GOMAXPROCS. Defaults to 2 (default 2)
#   -port int
#     	Port # to listen on. Defaults to 7070 (default 7070)
#   -thrifty
#     	Use only as many messages as strictly required for inter-replica communication.

if [ "${TYPE}" == "server" ];
then
    args="-addr ${ADDR} -port ${SPORT} -maddr ${MADDR} -mport ${MPORT} ${SERVER_EXTRA_ARGS}"; 
    echo "server mode: ${args}"
    ${DIR}/server ${args}
fi;

# Usage of ./bin/client:
#   -c int
#     	Percentage of conflicts. Defaults to 0% (default -1)
#   -check
#     	Check that every expected reply was received exactly once.
#   -e	Egalitarian (no leader). Defaults to false.
#   -eps int
#     	Send eps more messages per round than the client will wait for (to discount stragglers). Defaults to 0.
#   -f	Fast Paxos: send message directly to all replicas. Defaults to false.
#   -maddr string
#     	Master address. Defaults to localhost
#   -mport int
#     	Master port.  (default 7087)
#   -p int
#     	GOMAXPROCS. Defaults to 2 (default 2)
#   -q int
#     	Total number of requests. Defaults to 5000. (default 5000)
#   -r int
#     	Split the total number of requests into this many rounds, and do rounds sequentially. Defaults to 1. (default 1)
#   -s float
#     	Zipfian s parameter (default 2)
#   -v float
#     	Zipfian v parameter (default 1)
#   -w int
#     	Percentage of updates (writes). Defaults to 100%. (default 100)

if [ "${TYPE}" == "client" ];
then
    args="-maddr ${MADDR} -mport ${MPORT} ${CLIENT_EXTRA_ARGS}"; 
    echo "client mode: ${args}"
    ${DIR}/client ${args}
fi;
