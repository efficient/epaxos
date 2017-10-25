#!/usr/bin/env bash

LOGS=logs

NSERVERS=5
NCLIENTS=10
CMDS=10000
PSIZE=32
TOTAL_OPS=$(( NCLIENTS * CMDS ))

MASTER=bin/master
SERVER=bin/server
CLIENT=bin/client

master() {
  ${MASTER} -N ${NSERVERS} &
}

servers() {
  echo ">>>>> Starting servers..."
  for i in $(seq 1 ${NSERVERS}); do
    port=$(( 7000 + $i ))
    ${SERVER} -e \
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
    ${CLIENT} -e -v \
              -q ${CMDS} \
              -w 100 \
              -c 100 \
              -psize ${PSIZE} > "${LOGS}/c_$i.txt" 2>&1 &
  done

  ended=-1
  while [ ${ended} != ${NCLIENTS} ]; do
    ended=$(cat logs/c_*.txt  | grep "Disconnected" | wc -l)
    sleep 2
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
    cat "${LOGS}/s_$i.txt" | grep "Executing" |
                             grep -oE "\[.*\]" > ${f}
    all+=(${f})
  done

  echo ">>>>> Will check total order..."
  for i in $(seq 2 ${NSERVERS}); do
      diff "${LOGS}/1.ops" "${LOGS}/$i.ops"
  done

}

start_exp
master
servers
clients
end_exp
