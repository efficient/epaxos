#!/usr/bin/env bash

LOGS=logs

NSERVERS=5
NCLIENTS=10
CMDS=10000 # 500k total ~ 5min w. Paxos
PSIZE=32
TOTAL_OPS=$((NCLIENTS * CMDS))

MASTER=bin/master
SERVER=bin/server
CLIENT=bin/client

DIFF_TOOL=diff
#DIFF_TOOL=merge

maxfailures=1
injected_failures=2

master() {
    touch ${LOGS}/m.txt
    ${MASTER} -N ${NSERVERS} >"${LOGS}/m.txt" 2>&1 &
    tail -f ${LOGS}/m.txt &
}

servers() {
    echo ">>>>> Starting servers..."
    for i in $(seq 1 ${NSERVERS}); do
        port=$((7000 + $i))
        ${SERVER} \
            -lread \
            -exec \
            -thrifty \
	    -e \
	    -maxfailures ${maxfailures} \
            -port ${port} >"${LOGS}/s_$i.txt" 2>&1 &
    done

    up=-1
    while [ ${up} != ${NSERVERS} ]; do
        up=$(cat logs/s_*.txt | grep "Waiting for client connections" | wc -l)
        sleep 1
    done
    echo ">>>>> Servers up!"
}

clients() {
    echo ">>>>> Starting clients..."
    for i in $(seq 1 $NCLIENTS); do
        ${CLIENT} -v \
            -q ${CMDS} \
            -w 100 \
            -c 100 \
            -l \
	    -e \
            -psize ${PSIZE} >"${LOGS}/c_$i.txt" 2>&1 &
    done

    ended=-1
    while [ ${ended} != ${NCLIENTS} ]; do
        ended=$(tail -n 1 logs/c_*.txt | grep "Test took" | wc -l)
        sleep 1
        if ((${injected_failures} > 0)); then
            sleep 20
            leader=$(grep "new leader" ${LOGS}/m.txt | tail -n 1 | awk '{print $4}')
            port=$(grep "node ${leader} \[" ${LOGS}/m.txt | sed -n 's/.*\(:.*\]\).*/\1/p' | sed 's/[]:]//g')
            pid=$(ps -ef | grep "bin/server" | grep "${port}" | awk '{print $2}')
            echo ">>>>> Injecting failure... (${leader}, ${port}, ${pid})"
            kill -9 ${pid}
            injected_failures=$((injected_failures - 1))
        fi
    done
    echo ">>>>> Clients ended!"
}

stop_all() {
    echo ">>>>> Stopping All"
    for p in ${CLIENT} ${SERVER} ${MASTER}; do
        ps -aux | grep ${p} | awk '{ print $2 }' | xargs kill -9 >&/dev/null
    done
    ps -aux | grep "tail -f ${LOGS}/m.txt" | awk '{ print $2 }' | xargs kill -9 >&/dev/null
    true
}

start_exp() {
    rm -rf ${LOGS}/*
    mkdir -p ${LOGS}
    stop_all
}

end_exp() {
    all=()
    for i in $(seq 1 ${NSERVERS}); do
        f="${LOGS}/$i.ops"
        cat "${LOGS}/s_$i.txt" | grep "Executing" | cut -d',' -f 2 | cut -d' ' -f 2 >${f}
        all+=(${f})
    done

    echo ">>>>> Will check total order..."
    case ${DIFF_TOOL} in
    diff)
        for i in $(seq 2 ${NSERVERS}); do
            diff "${LOGS}/1.ops" "${LOGS}/$i.ops" >/dev/null
        done
        ;;
    merge)
        i=1
        paste -d: ${all[@]} | while read -r line; do
            if [ $((i % 1000)) == 0 ]; then
                echo ">>>>> Checked ${i} of ${TOTAL_OPS}..."
            fi
            unique=$(echo ${line} | sed 's/:/\n/g' | sort -u | wc -l)
            if [ "${unique}" != "1" ]; then
                echo -e "#${i}:\n$(echo ${line} | sed 's/:/\n/g')"
            fi
            i=$((i + 1))
        done
        ;;
    *)
        echo "Invalid diff tool!"
        exit -1
        ;;
    esac

}

trap "stop_all; exit 255" SIGINT SIGTERM

start_exp
master
servers
clients
end_exp

stop_all
