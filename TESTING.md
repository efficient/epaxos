## Testing Epaxos and Paxos on GCP
### Installation(***In Each VM***)
1. Make sure Rabia is properly installed. Follow the instructions in the repo. This step is critical as it provides the go binary and python3.8 needed for testing.
2. SSH into each of the VMs and do the following inside /root:
    1. ```git clone https://github.com/rabia-consensus/epaxos.git && cd epaxos```
    2. ```git checkout epaxos-paxos-logging```
    3. ```. compile.sh```
    4. After step 3, you should see:
       </br>
        ```Built Master```
       </br>
        ```Built Server```
       </br>
       ```Built Client```
       



### Configuration(***In Each VM***)
1. Inside ```epaxos.sh``` configure:
   1. Run Configs (NClients, etc.)
   2. External/Internal IPs (Depending on zonage)
   3. Paths to your folders/keys. I usually install in ``root``.
2. For further debugging, you may also turn on the ```dlog``` which is the native logging service of Epaxos.
    1. Inside your epaxos folder, ```cd src/dlog && nano dlog.go```
    2. Edit this ```dlog = false```
    3. Make sure you then recompile binaries with, ```. compile.sh```

### Run(***In Controller VM***)
1. Finally, run ```. epaxos.sh > run.txt``` in the terminal of your controller vm.
2. If all works correctly, you should see n client logs inside the /logs directory in your controller VM.
3. For throughput/latency analysis, run:
    1. ```python3.8 analysis.py ./logs```
    
### Switching Between Paxos and Epaxos(***In Each VM***)
#### Paxos
1. Inside ```epaxos.sh```, remove the ```-e=true``` flag occurences in both the client and server binary calls.
#### Epaxos
1. Similarly, insert ```-e=true``` into these client/server binary calls, but, ***note that the order in which flags are placed matters***.
2. To investigate order, open ```src/client/client.go``` and ```src/client/server.go``` to see ordering for config flags.
