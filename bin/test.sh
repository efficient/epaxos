#!/usr/bin/env bash

LOGS=logs

NSERVERS=3
NCLIENTS=30
CMDS=2000
PSIZE=32
TOTAL_OPS=$(( NCLIENTS * CMDS ))

MASTER=bin/master
SERVER=bin/server
CLIENT=bin/client

DIFF_TOOL=diff
#DIFF_TOOL=merge

INJECT_FAILURE=1

master() {
    ${MASTER} -N ${NSERVERS} > "${LOGS}/m.txt" 2>&1 &
    tail -f ${LOGS}/m.txt &
}

servers() {
    echo ">>>>> Starting servers..."
    for i in $(seq 1 ${NSERVERS}); do
	port=$(( 7000 + $i ))
	${SERVER}\
	    -e \
	    -lread \
	    -exec \
	    -thrifty \
	    -port ${port} > "${LOGS}/s_$i.txt" 2>&1 &
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
		  -psize ${PSIZE} > "${LOGS}/c_$i.txt" 2>&1 &
    done

    ended=-1
    while [ ${ended} != ${NCLIENTS} ]; do
	ended=$(cat logs/c_*.txt  | grep "Test took" | wc -l)
	sleep 1
	if [ ${INJECT_FAILURE} == 1 ];
	then
	    sleep 10
	    leader=$(grep "new leader" ${LOGS}/m.txt | awk '{print $7}')
	    port=$(grep "node ${leader}" ${LOGS}/m.txt | sed -n 's/.*\(:.*\]\).*/\1/p' | sed 's/[]:]//g')
	    pid=$(ps -ef | grep "bin/server" | grep "${port}" | awk '{print $2}')
	    echo ">>>>> Injecting failure... (${leader}, ${port}, ${pid})"
	    kill -9 ${pid}
	    INJECT_FAILURE=0
	fi
    done
    echo ">>>>> Client ended!"
}

stop_all() {
    for p in ${CLIENT} ${SERVER} ${MASTER}; do
	ps -aux | grep ${p} | awk '{ print $2 }' | xargs kill -9
    done
}

start_exp() {
    rm -rf ${LOGS}
    mkdir ${LOGS}
    stop_all
}

end_exp() {
    stop_all
    all=()
    for i in $(seq 1 ${NSERVERS}); do
	f="${LOGS}/$i.ops"
	cat "${LOGS}/s_$i.txt" | grep "Executing" | cut -d',' -f 2 | cut -d' ' -f 2 > ${f} 
	all+=(${f})
    done

    echo ">>>>> Will check total order..."
    case ${DIFF_TOOL} in
	diff)
	    for i in $(seq 2 ${NSERVERS}); do
		diff "${LOGS}/1.ops" "${LOGS}/$i.ops" > /dev/null
	    done
	    ;;
	merge)
	    i=1
	    paste -d: ${all[@]} | while read -r line ; do
		if [ $((i % 1000)) == 0 ]; then
		    echo ">>>>> Checked ${i} of ${TOTAL_OPS}..."
		fi
		unique=$(echo ${line} | sed 's/:/\n/g' | sort -u | wc -l)
		if [ "${unique}" != "1" ]; then
		    echo -e "#${i}:\n$(echo ${line} | sed 's/:/\n/g')"
		fi
		i=$(( i + 1 ))
	    done
	    ;;
	*)
	    echo "Invalid diff tool!"
	    exit -1
    esac

}

trap "stop_all; exit 255" SIGINT SIGTERM

start_exp
master
servers
clients
end_exp

stop_all
