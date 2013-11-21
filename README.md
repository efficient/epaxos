EPaxos
======


### What is EPaxos?


EPaxos is an efficient, leaderless replication protocol. The name stands for *Egalitarian Paxos* -- EPaxos is based
on the Paxos consensus algorithm. As such, it can tolerate up to F concurrent replica failures with 2F+1 total replicas.

### How does EPaxos differ from Paxos and other Paxos variants?

To function effectively as a replication protocol, Paxos has to rely on a stable leader replica (this optimization is known as Multi-Paxos). The leader can become a bottleneck for performance: it has to handle more messages than the other replicas, and remote clients have to contact the leader, thus experiencing higher latency. Other Paxos variants either also rely on a stable leader, or have a pre-established scheme that allows different replicas to take turns in proposing commands (such as Mencius). This latter scheme
suffers from tight coupling of the performance of the system from that of every replica -- i.e., the system runs at the speed of the slowest replica.

EPaxos is an efficient, leaderless protocol. It provides **strong consistency with optimal wide-area latency, perfect load-balancing across replicas (both in the local and the wide area), and constant availability for up to F failures**. EPaxos also decouples the performance of the slowest replicas from that of the fastest, so it can better tolerate slow replicas than previous protocols.

### How does EPaxos work?

We have [an SOSP 2013 paper](http://dl.acm.org/ft_gateway.cfm?id=2517350&ftid=1403953&dwn=1) that describes EPaxos in detail.

A simpler, more straightforward explanation is coming here soon.


### What is in this repository?

This repository contains the Go implementations of:

* Egalitarian Paxos (EPaxos), a new distributed consensus algorithm based on
Paxos EPaxos achieves three goals: (1) availability *without interruption*
as long as a simple majority of replicas are reachable---its availability is not
interrupted when replicas crash or fail to respond; (2) uniform load balancing
across all replicas---no replicas experience higher load because they have
special roles; and (3) optimal commit latency in the wide-area when tolerating
one and two failures, under realistic conditions. Egalitarian Paxos is to our
knowledge the first distributed consensus protocol to achieve all of these goals
efficiently: requiring only a simple majority of replicas to be non-faulty,
using a number of messages linear in the number of replicas to choose a command,
and committing commands after just one communication round (one round trip) in
the common case or after at most two rounds in any case.

* (classic) Paxos

* Mencius

* Generalized Paxos


The struct marshaling and unmarshaling code was generated automatically using
the tool available at: https://code.google.com/p/gobin-codegen/

The repository also contains a machine-readable (and model-checkable) specification of EPaxos in TLA+.


AUTHORS:

Iulian Moraru, David G. Andersen -- Carnegie Mellon University

Michael Kaminsky -- Intel Labs
