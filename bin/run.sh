#!/bin/bash

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ "${TYPE}" == "" ]; then
    echo "usage: define env variables, as listed below
    TYPE = [master,server,client] # type of instance
    MPORT, MADDR # master instance
    MPORT, MADDR, ADDR, SPORT, SERVER_EXTRA_ARGS # server instance
    MPORT, MADDR, NCLIENTS, CLIENT_EXTRA_ARGS # client instance
    "
    exit 0
fi

# Usage of ./bin/master:
#   -N int
#     	Number of replicas. Defaults to 3. (default 3)
#   -port int
#     	Port # to listen on. Defaults to 7087 (default 7087)

if [ "${TYPE}" == "master" ]; then
    args="-port ${MPORT} -N ${NREPLICAS}"
    echo "master mode: ${args}"
    ${DIR}/master ${args}
fi

# Usage of ./bin/server:
#   -addr string
#     	Server address (this machine). Defaults to localhost.
#   -beacon
#     	Send beacons to other replicas to compare their relative speeds.
#   -cpuprofile string
#     	write cpu profile to file
#   -dreply
#     	Reply to client only after command has been executed. (default true)
#   -durable
#     	Log to a stable store (i.e., a file in the current dir).
#   -e	Use EPaxos as the replication protocol. Defaults to false.
#   -exec
#     	Execute commands. (default true)
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
#     	Use only as many messages as strictly required for inter-replica communication. (default true)

if [ "${TYPE}" == "server" ]; then
    args="-addr ${ADDR} -port ${SPORT} -maddr ${MADDR} -mport ${MPORT} ${SERVER_EXTRA_ARGS}"
    echo "server mode: ${args}"
    ${DIR}/server ${args}
fi

# Usage of ./bin/client:
#   -c int
#     	Percentage of conflicts. Defaults to 0%
#   -e	Egalitarian (no leader).
#   -f	Fast Paxos: send message directly to all replicas.
#   -id string
#     	the id of the client. Default is RFC 4122 nodeID.
#   -maddr string
#     	Master address. Defaults to localhost
#   -mport int
#     	Master port.  (default 7087)
#   -p int
#     	GOMAXPROCS.  (default 2)
#   -psize int
#     	Payload size for writes. (default 100)
#   -q int
#     	Total number of requests.  (default 1000)
#   -v	verbose mode.
#   -w int
#     	Percentage of updates (writes).  (default 100)
#   -s	replace read with short scan (100 elements)

if [ "${TYPE}" == "client" ]; then
    args="-maddr ${MADDR} -mport ${MPORT} ${CLIENT_EXTRA_ARGS}"

    # aggregate all logs in a single file
    ALL=all_logs
    echo "client mode: ${args}" >${ALL}

    mkdir -p logs/

    for i in $(seq 1 ${NCLIENTS}); do
        ${DIR}/client ${args} 2>&1 | tee -a logs/c_${i}.txt ${ALL} >/dev/null &
        echo "> Client $i of ${NCLIENTS} started!"
    done

    echo "Will check if all are started..."
    started=-1
    while [ ${started} != ${NCLIENTS} ]; do
        started=$(ls logs/c_*.txt 2>/dev/null | wc -l)
    done

    echo "Will check if all are connected..."
    connected=-1
    while [ ${connected} != ${NCLIENTS} ]; do
        connected=$(grep "Connected" ${ALL} | wc -l)
    done
    echo "Connect OK!"

    ended=-1
    while [ ${ended} != ${NCLIENTS} ]; do
        ended=$(grep "Test took" ${ALL} | wc -l)
        echo "> Ended ${ended} of ${NCLIENTS}!"
        sleep 10
    done

    pkill -P $$

    echo "Will sleep forever"
    while true; do sleep 10000; done
fi
