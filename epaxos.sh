ServerIps=(10.142.0.27 10.142.0.37 10.142.0.63) # 3
#ClientIps=(10.142.0.70 10.142.0.71 10.142.0.72 10.142.0.73 10.142.0.74)
ClientIps=(10.142.0.11 10.142.0.103 10.142.0.104)
#ClientIps=(10.142.0.70)
MasterIp=10.142.0.27
FirstServerPort=17070 # change it when only necessary (i.e., firewall blocking, port in use)
NumOfServerInstances=3 # before recompiling, try no more than 5 servers. See Known Issue # 4
NumOfClientInstances=10 #20,40,60,80,100,200
reqsNb=100000
writes=50
dlog=false
conflicts=0
thrifty=false
serverP=2       # server's GOMAXPROCS (sp)
clientP=1       # client's GOMAXPROCS (cp)

# if closed-loop, uncomment two lines below
clientBatchSize=10
rounds=$((reqsNb / clientBatchSize))
# if open-loop, uncomment the line below
#rounds=1 # open-loop

# some constants
SSHKey=/root/go/src/rc3/deployment/install/id_rsa # RC project has it
EPaxosFolder=/root/go/src/epaxos # where the epaxos' bin folder is located
LogFolder=/root/go/src/epaxos/logs

function prepareRun() {
    for ip in "${ServerIps[@]}"
    do
        ssh -o StrictHostKeyChecking=no -i ${SSHKey} root@"$ip" "mkdir -p ${LogFolder}; rm -rf ${LogFolder}/*; cd ${EPaxosFolder} && chmod 777 epaxos.sh" 2>&1
        sleep 0.3
    done
    for ip in "${ClientIps[@]}"
    do
        ssh -o StrictHostKeyChecking=no -i ${SSHKey} root@"$ip" "mkdir -p ${LogFolder}; rm -rf ${LogFolder}/*; cd ${EPaxosFolder} && chmod 777 epaxos.sh" 2>&1
        sleep 0.3
    done
    wait
}

function runMaster() {
    "${EPaxosFolder}"/bin/master -N ${NumOfServerInstances} 2>&1 &
}

function runServersOneMachine() {
    for idx in $(seq 0 $(($NumOfServerInstances - 1)))
    do
        svrIpIdx=$((idx % ${#ServerIps[@]}))
        svrIp=${ServerIps[svrIpIdx]}
        svrPort=$((FirstServerPort + $idx))
        if [[ ${svrIpIdx} -eq ${EPMachineIdx} ]]
        then
            "${EPaxosFolder}"/bin/server -port ${svrPort} -maddr ${MasterIp} -addr ${svrIp} -p 4 -thrifty=${thrifty} -e=true 2>&1 &
        fi
    done
}

function runClientsOneMachine() {
    ulimit -n 65536
    mkdir -p ${LogFolder}
    for idx in $(seq 0 $((NumOfClientInstances - 1)))
    do
        cliIpIdx=$((idx % ${#ClientIps[@]}))
        cliIp=${ClientIps[cliIpIdx]}
        if [[ ${cliIpIdx} -eq ${EPMachineIdx} ]]
        then
            "${EPaxosFolder}"/bin/client -maddr ${MasterIp} -q ${reqsNb} -w ${writes} -r ${rounds} -p 30 -c ${conflicts} 2>&1 &# >${LogFolder}/S${NumOfServerInstances}-C${NumOfClientInstances}-q${reqsNb}-w${writes}-r${rounds}-c${conflicts}--client${idx}.out
        fi
    done
}

function runServersAllMachines() {
    runMaster
    sleep 2

    MachineIdx=0
    for ip in "${ServerIps[@]}"
    do
        ssh -o StrictHostKeyChecking=no -i ${SSHKey} root@"$ip" "cd ${EPaxosFolder} && EPScriptOption=StartServers EPMachineIdx=${MachineIdx} /bin/bash epaxos.sh" 2>&1 &
        sleep 0.3
        ((MachineIdx++))
    done
}

function runClientsAllMachines() {
    MachineIdx=0
    for ip in "${ClientIps[@]}"
    do
        ssh -o StrictHostKeyChecking=no -i ${SSHKey} root@"$ip" "cd ${EPaxosFolder} && EPScriptOption=StartClients EPMachineIdx=${MachineIdx} /bin/bash epaxos.sh" 2>&1 &
        sleep 0.3
        ((MachineIdx++))
    done
}

function runServersAndClientsAllMachines() {
    runServersAllMachines
    sleep 5 # TODO(highlight): add wait time here
    runClientsAllMachines
}

function SendEPaxosFolder() {
    for ip in "${ServerIps[@]}"
    do
        scp -o StrictHostKeyChecking=no -i ${SSHKey} -r ${EPaxosFolder} root@"$ip":~  2>&1 &
        sleep 0.3
    done
    for ip in "${ClientIps[@]}"
    do
        scp -o StrictHostKeyChecking=no -i ${SSHKey} -r ${EPaxosFolder} root@"$ip":~  2>&1 &
        sleep 0.3
    done
    wait
}

function SSHCheckClientProgress() {
    for ip in "${ClientIps[@]}"
    do
        ssh -o StrictHostKeyChecking=no -i ${SSHKey} root@"$ip" "ps -fe | grep bin/client" 2>&1 &
    done
}

function EpKillAll() {
    for ip in "${ServerIps[@]}"
    do
        ssh -o StrictHostKeyChecking=no -i ${SSHKey} root@"$ip" "cd ${EPaxosFolder} && chmod 777 kill.sh && /bin/bash kill.sh" 2>&1 &
        sleep 0.3
    done
    for ip in "${ClientIps[@]}"
    do
        ssh -o StrictHostKeyChecking=no -i ${SSHKey} root@"$ip" "cd ${EPaxosFolder} && chmod 777 kill.sh && /bin/bash kill.sh" 2>&1 &
        sleep 0.3
    done
    wait
}

function DownloadLogs() {
    mkdir -p ${LogFolder}

#    for ip in "${ServerIps[@]}"
#    do
#        scp -o StrictHostKeyChecking=no -i ${SSHKey} root@"$ip":${LogFolder}/*.out ${LogFolder} 2>&1 &
#        sleep 0.3
#    done

    for ip in "${ClientIps[@]}"
    do
        scp -o StrictHostKeyChecking=no -i ${SSHKey} root@"$ip":${LogFolder}/*.out ${LogFolder} 2>&1 &
        sleep 0.3
    done
}

function RemoveLogs(){
  for ip in "${ClientIps[@]}"
  do
        ssh -o StrictHostKeyChecking=no -i ${SSHKey} root@"$ip" "rm -rf ${LogFolder}/*" 2>&1 &
        sleep 0.3
  done

  for ip in "${ServerIps[@]}"
  do
        ssh -o StrictHostKeyChecking=no -i ${SSHKey} root@"$ip" "rm -rf ${LogFolder}/*" 2>&1 &
        sleep 0.3
  done
}

function Analysis() {
    sleep 3
#    cat ${LogFolder}/*.out  # for visual inspection
    python3.8 analysis_paxos.py ${LogFolder} print-title
}

function Main() {
    case ${EPScriptOption} in
        "StartServers")
            runServersOneMachine
            ;;
        "StartClients")
            runClientsOneMachine
            ;;
        "killall")
            EpKillAll
            ;;
        *)
            runServersAndClientsAllMachines
            ;;
    esac
}

#SendEPaxosFolder
#prepareRun;
RemoveLogs
wait
Main
wait
DownloadLogs
wait
EpKillAll