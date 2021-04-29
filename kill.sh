pgrep -af /root/go/src/epaxos/bin/master | while read -r pid cmd ; do
     echo "pid: $pid, cmd: $cmd"
    kill -9 $pid > /dev/null 2>&1
done
pgrep -af /root/go/src/epaxos/bin/server | while read -r pid cmd ; do
     echo "pid: $pid, cmd: $cmd"
    kill -9 $pid > /dev/null 2>&1
done
pgrep -af /root/go/src/epaxos/bin/client | while read -r pid cmd ; do
     echo "pid: $pid, cmd: $cmd"
    kill -9 $pid > /dev/null 2>&1
done