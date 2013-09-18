epaxos
======

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


AUTHORS:

Iulian Moraru, David G. Andersen -- Carnegie Mellon University

Michael Kaminsky -- Intel Labs
