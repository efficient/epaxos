-------------------------- MODULE EgalitarianPaxos --------------------------

EXTENDS Naturals, FiniteSets

-----------------------------------------------------------------------------

Max(S) == IF S = {} THEN 0 ELSE CHOOSE i \in S : \A j \in S : j <= i


(*********************************************************************************)
(* Constant parameters:                                                          *)
(*       Commands: the set of all possible commands                              *)
(*       Replicas: the set of all EPaxos replicas                                *)
(*       FastQuorums(r): the set of all fast quorums where r is a command leader *)
(*       SlowQuorums(r): the set of all slow quorums where r is a command leader *)
(*********************************************************************************)

CONSTANTS Commands, Replicas, FastQuorums(_), SlowQuorums(_), MaxBallot

ASSUME IsFiniteSet(Replicas)

(***************************************************************************)
(* Quorum conditions:                                                      *)
(*  (simplified)                                                           *)
(***************************************************************************)

ASSUME \A r \in Replicas:
  /\ SlowQuorums(r) \subseteq SUBSET Replicas
  /\ \A SQ \in SlowQuorums(r): 
    /\ r \in SQ
    /\ Cardinality(SQ) = (Cardinality(Replicas) \div 2) + 1

ASSUME \A r \in Replicas:
  /\ FastQuorums(r) \subseteq SUBSET Replicas
  /\ \A FQ \in FastQuorums(r):
    /\ r \in FQ
    /\ Cardinality(FQ) = (Cardinality(Replicas) \div 2) + 
                         ((Cardinality(Replicas) \div 2) + 1) \div 2
    
    
(***************************************************************************)
(* Special none command                                                    *)
(***************************************************************************)

none == CHOOSE c : c \notin Commands


(***************************************************************************)
(* The instance space                                                      *)
(***************************************************************************)

Instances == Replicas \X (1..Cardinality(Commands))

(***************************************************************************)
(* The possible status of a command in the log of a replica.               *)
(***************************************************************************)

Status == {"not-seen", "pre-accepted", "accepted", "committed"}


(***************************************************************************)
(* All possible protocol messages:                                         *)
(***************************************************************************)

Message ==
        [type: {"pre-accept"}, src: Replicas, dst: Replicas,
        inst: Instances, ballot: Nat \X Replicas,
        cmd: Commands \cup {none}, deps: SUBSET Instances, seq: Nat]
  \cup  [type: {"accept"}, src: Replicas, dst: Replicas,
        inst: Instances, ballot: Nat \X Replicas,
        cmd: Commands \cup {none}, deps: SUBSET Instances, seq: Nat]
  \cup  [type: {"commit"},
        inst: Instances, ballot: Nat \X Replicas,
        cmd: Commands \cup {none}, deps: SUBSET Instances, seq: Nat]
  \cup  [type: {"prepare"}, src: Replicas, dst: Replicas,
        inst: Instances, ballot: Nat \X Replicas]
  \cup  [type: {"pre-accept-reply"}, src: Replicas, dst: Replicas,
        inst: Instances, ballot: Nat \X Replicas,
        deps: SUBSET Instances, seq: Nat, committed: SUBSET Instances]
  \cup  [type: {"accept-reply"}, src: Replicas, dst: Replicas,
        inst: Instances, ballot: Nat \X Replicas]
  \cup  [type: {"prepare-reply"}, src: Replicas, dst: Replicas,
        inst: Instances, ballot: Nat \X Replicas, prev_ballot: Nat \X Replicas,
        status: Status,
        cmd: Commands \cup {none}, deps: SUBSET Instances, seq: Nat]
  \cup  [type: {"try-pre-accept"}, src: Replicas, dst: Replicas,
        inst: Instances, ballot: Nat \X Replicas,
        cmd: Commands \cup {none}, deps: SUBSET Instances, seq: Nat]
  \cup  [type: {"try-pre-accept-reply"}, src: Replicas, dst: Replicas,
        inst: Instances, ballot: Nat \X Replicas, status: Status \cup {"OK"}]
        
        
 

(*******************************************************************************)
(* Variables:                                                                  *)
(*                                                                             *)
(*          comdLog = the commands log at each replica                         *)
(*          proposed = command that have been proposed                         *)
(*          executed = the log of executed commands at each replica            *)
(*          sentMsg = sent (but not yet received) messages                     *)
(*          crtInst = the next instance available for a command                *)
(*                    leader                                                   *)
(*          leaderOfInst = the set of instances each replica has               *)
(*                         started but not yet finalized                       *)
(*          committed = maps commands to set of commit attributs               *)
(*                      tuples                                                 *)
(*          ballots = largest ballot number used by any                        *)
(*                    replica                                                  *)
(*          preparing = set of instances that each replica is                  *)
(*                      currently preparing (i.e. recovering)                  *) 
(*                                                                             *)
(*                                                                             *)
(*******************************************************************************)

 
VARIABLES cmdLog, proposed, executed, sentMsg, crtInst, leaderOfInst,
          committed, ballots, preparing

TypeOK ==
    /\ cmdLog \in [Replicas -> SUBSET [inst: Instances, 
                                       status: Status,
                                       ballot: Nat \X Replicas,
                                       cmd: Commands \cup {none},
                                       deps: SUBSET Instances,
                                       seq: Nat]]
    /\ proposed \in SUBSET Commands
    /\ executed \in [Replicas -> SUBSET (Nat \X Commands)]
    /\ sentMsg \in SUBSET Message
    /\ crtInst \in [Replicas -> Nat]
    /\ leaderOfInst \in [Replicas -> SUBSET Instances]
    /\ committed \in [Instances -> SUBSET ((Commands \cup {none}) \X
                                           (SUBSET Instances) \X 
                                           Nat)]
    /\ ballots \in Nat
    /\ preparing \in [Replicas -> SUBSET Instances]
    
vars == << cmdLog, proposed, executed, sentMsg, crtInst, leaderOfInst, 
           committed, ballots, preparing >>

(***************************************************************************)
(* Initial state predicate                                                 *)
(***************************************************************************)

Init ==
  /\ sentMsg = {}
  /\ cmdLog = [r \in Replicas |-> {}]
  /\ proposed = {}
  /\ executed = [r \in Replicas |-> {}]
  /\ crtInst = [r \in Replicas |-> 1]
  /\ leaderOfInst = [r \in Replicas |-> {}]
  /\ committed = [i \in Instances |-> {}]
  /\ ballots = 1
  /\ preparing = [r \in Replicas |-> {}]



(***************************************************************************)
(* Actions                                                                 *)
(***************************************************************************)

StartPhase1(C, cleader, Q, inst, ballot, oldMsg) ==
    LET newDeps == {rec.inst: rec \in cmdLog[cleader]} 
        newSeq == 1 + Max({t.seq: t \in cmdLog[cleader]}) 
        oldRecs == {rec \in cmdLog[cleader] : rec.inst = inst} IN
        /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ oldRecs) \cup 
                                {[inst   |-> inst,
                                  status |-> "pre-accepted",
                                  ballot |-> ballot,
                                  cmd    |-> C,
                                  deps   |-> newDeps,
                                  seq    |-> newSeq ]}]
        /\ leaderOfInst' = [leaderOfInst EXCEPT ![cleader] = @ \cup {inst}]
        /\ sentMsg' = (sentMsg \ oldMsg) \cup 
                                [type  : {"pre-accept"},
                                  src   : {cleader},
                                  dst   : Q \ {cleader},
                                  inst  : {inst},
                                  ballot: {ballot},
                                  cmd   : {C},
                                  deps  : {newDeps},
                                  seq   : {newSeq}]

Propose(C, cleader) ==
    LET newInst == <<cleader, crtInst[cleader]>> 
        newBallot == <<0, cleader>> 
    IN  /\ proposed' = proposed \cup {C}
        /\ (\E Q \in FastQuorums(cleader):
                 StartPhase1(C, cleader, Q, newInst, newBallot, {}))
        /\ crtInst' = [crtInst EXCEPT ![cleader] = @ + 1]
        /\ UNCHANGED << executed, committed, ballots, preparing >>


Phase1Reply(replica) ==
    \E msg \in sentMsg:
        /\ msg.type = "pre-accept"
        /\ msg.dst = replica
        /\ LET oldRec == {rec \in cmdLog[replica]: rec.inst = msg.inst} IN
            /\ (\A rec \in oldRec : 
                (rec.ballot = msg.ballot \/rec.ballot[1] < msg.ballot[1]))
            /\ LET newDeps == msg.deps \cup 
                            ({t.inst: t \in cmdLog[replica]} \ {msg.inst})
                   newSeq == Max({msg.seq, 
                                  1 + Max({t.seq: t \in cmdLog[replica]})})
                   instCom == {t.inst: t \in {tt \in cmdLog[replica] :
                              tt.status \in {"committed", "executed"}}} IN
                /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                                    {[inst   |-> msg.inst,
                                      status |-> "pre-accepted",
                                      ballot |-> msg.ballot,
                                      cmd    |-> msg.cmd,
                                      deps   |-> newDeps,
                                      seq    |-> newSeq]}]
                /\ sentMsg' = (sentMsg \ {msg}) \cup
                                    {[type  |-> "pre-accept-reply",
                                      src   |-> replica,
                                      dst   |-> msg.src,
                                      inst  |-> msg.inst,
                                      ballot|-> msg.ballot,
                                      deps  |-> newDeps,
                                      seq   |-> newSeq,
                                      committed|-> instCom]}
                /\ UNCHANGED << proposed, crtInst, executed, leaderOfInst,
                                committed, ballots, preparing >>

Phase1Fast(cleader, i, Q) ==
    /\ i \in leaderOfInst[cleader]
    /\ Q \in FastQuorums(cleader)
    /\ \E record \in cmdLog[cleader]:
        /\ record.inst = i
        /\ record.status = "pre-accepted"
        /\ record.ballot[1] = 0
        /\ LET replies == {msg \in sentMsg: 
                                /\ msg.inst = i
                                /\ msg.type = "pre-accept-reply"
                                /\ msg.dst = cleader
                                /\ msg.src \in Q
                                /\ msg.ballot = record.ballot} IN
            /\ (\A replica \in (Q \ {cleader}): 
                    \E msg \in replies: msg.src = replica)
            /\ (\A r1, r2 \in replies:
                /\ r1.deps = r2.deps
                /\ r1.seq = r2.seq)
            /\ LET r == CHOOSE r \in replies : TRUE IN
                /\ LET localCom == {t.inst: 
                            t \in {tt \in cmdLog[cleader] : 
                                 tt.status \in {"committed", "executed"}}}
                       extCom == UNION {msg.committed: msg \in replies} IN
                       (r.deps \subseteq (localCom \cup extCom))    
                /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {record}) \cup 
                                        {[inst   |-> i,
                                          status |-> "committed",
                                          ballot |-> record.ballot,
                                          cmd    |-> record.cmd,
                                          deps   |-> r.deps,
                                          seq    |-> r.seq ]}]
                /\ sentMsg' = (sentMsg \ replies) \cup
                            {[type  |-> "commit",
                            inst    |-> i,
                            ballot  |-> record.ballot,
                            cmd     |-> record.cmd,
                            deps    |-> r.deps,
                            seq     |-> r.seq]}
                /\ leaderOfInst' = [leaderOfInst EXCEPT ![cleader] = @ \ {i}]
                /\ committed' = [committed EXCEPT ![i] = 
                                            @ \cup {<<record.cmd, r.deps, r.seq>>}]
                /\ UNCHANGED << proposed, executed, crtInst, ballots, preparing >>

Phase1Slow(cleader, i, Q) ==
    /\ i \in leaderOfInst[cleader]
    /\ Q \in SlowQuorums(cleader)
    /\ \E record \in cmdLog[cleader]:
        /\ record.inst = i
        /\ record.status = "pre-accepted"
        /\ LET replies == {msg \in sentMsg: 
                                /\ msg.inst = i 
                                /\ msg.type = "pre-accept-reply" 
                                /\ msg.dst = cleader 
                                /\ msg.src \in Q
                                /\ msg.ballot = record.ballot} IN
            /\ (\A replica \in (Q \ {cleader}): \E msg \in replies: msg.src = replica)
            /\ LET finalDeps == UNION {msg.deps : msg \in replies}
                   finalSeq == Max({msg.seq : msg \in replies}) IN    
                /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {record}) \cup 
                                        {[inst   |-> i,
                                          status |-> "accepted",
                                          ballot |-> record.ballot,
                                          cmd    |-> record.cmd,
                                          deps   |-> finalDeps,
                                          seq    |-> finalSeq ]}]
                /\ \E SQ \in SlowQuorums(cleader):
                   (sentMsg' = (sentMsg \ replies) \cup
                            [type : {"accept"},
                            src : {cleader},
                            dst : SQ \ {cleader},
                            inst : {i},
                            ballot: {record.ballot},
                            cmd : {record.cmd},
                            deps : {finalDeps},
                            seq : {finalSeq}])
                /\ UNCHANGED << proposed, executed, crtInst, leaderOfInst,
                                committed, ballots, preparing >>
                
Phase2Reply(replica) ==
    \E msg \in sentMsg: 
        /\ msg.type = "accept"
        /\ msg.dst = replica
        /\ LET oldRec == {rec \in cmdLog[replica]: rec.inst = msg.inst} IN
            /\ (\A rec \in oldRec: (rec.ballot = msg.ballot \/ 
                                    rec.ballot[1] < msg.ballot[1]))
            /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                                {[inst   |-> msg.inst,
                                  status |-> "accepted",
                                  ballot |-> msg.ballot,
                                  cmd    |-> msg.cmd,
                                  deps   |-> msg.deps,
                                  seq    |-> msg.seq]}]
            /\ sentMsg' = (sentMsg \ {msg}) \cup
                                {[type  |-> "accept-reply",
                                  src   |-> replica,
                                  dst   |-> msg.src,
                                  inst  |-> msg.inst,
                                  ballot|-> msg.ballot]}
            /\ UNCHANGED << proposed, crtInst, executed, leaderOfInst,
                            committed, ballots, preparing >>


Phase2Finalize(cleader, i, Q) ==
    /\ i \in leaderOfInst[cleader]
    /\ Q \in SlowQuorums(cleader)
    /\ \E record \in cmdLog[cleader]:
        /\ record.inst = i
        /\ record.status = "accepted"
        /\ LET replies == {msg \in sentMsg: 
                                /\ msg.inst = i 
                                /\ msg.type = "accept-reply" 
                                /\ msg.dst = cleader 
                                /\ msg.src \in Q 
                                /\ msg.ballot = record.ballot} IN
            /\ (\A replica \in (Q \ {cleader}): \E msg \in replies: 
                                                        msg.src = replica)
            /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {record}) \cup 
                                    {[inst   |-> i,
                                      status |-> "committed",
                                      ballot |-> record.ballot,
                                      cmd    |-> record.cmd,
                                      deps   |-> record.deps,
                                      seq    |-> record.seq ]}]
            /\ sentMsg' = (sentMsg \ replies) \cup
                        {[type  |-> "commit",
                        inst    |-> i,
                        ballot  |-> record.ballot,
                        cmd     |-> record.cmd,
                        deps    |-> record.deps,
                        seq     |-> record.seq]}
            /\ committed' = [committed EXCEPT ![i] = @ \cup 
                               {<<record.cmd, record.deps, record.seq>>}]
            /\ leaderOfInst' = [leaderOfInst EXCEPT ![cleader] = @ \ {i}]
            /\ UNCHANGED << proposed, executed, crtInst, ballots, preparing >>

Commit(replica, cmsg) ==
    LET oldRec == {rec \in cmdLog[replica] : rec.inst = cmsg.inst} IN
        /\ \A rec \in oldRec : (rec.status \notin {"committed", "executed"} /\ 
                                rec.ballot[1] <= cmsg.ballot[1])
        /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup 
                                    {[inst     |-> cmsg.inst,
                                      status   |-> "committed",
                                      ballot   |-> cmsg.ballot,
                                      cmd      |-> cmsg.cmd,
                                      deps     |-> cmsg.deps,
                                      seq      |-> cmsg.seq]}]
        /\ committed' = [committed EXCEPT ![cmsg.inst] = @ \cup 
                               {<<cmsg.cmd, cmsg.deps, cmsg.seq>>}]
        /\ UNCHANGED << proposed, executed, crtInst, leaderOfInst,
                        sentMsg, ballots, preparing >>


(***************************************************************************)
(* Recovery actions                                                        *)
(***************************************************************************)

SendPrepare(replica, i, Q) ==
    /\ i \notin leaderOfInst[replica]
    \*/\ i \notin preparing[replica]
    /\ ballots <= MaxBallot
    /\ ~(\E rec \in cmdLog[replica] :
                        /\ rec.inst = i
                        /\ rec.status \in {"committed", "executed"})
    /\ sentMsg' = sentMsg \cup
                    [type   : {"prepare"},
                     src    : {replica},
                     dst    : Q,
                     inst   : {i},
                     ballot : {<< ballots, replica >>}]
    /\ ballots' = ballots + 1
    /\ preparing' = [preparing EXCEPT ![replica] = @ \cup {i}]
    /\ UNCHANGED << cmdLog, proposed, executed, crtInst,
                    leaderOfInst, committed >>
    
ReplyPrepare(replica) ==
    \E msg \in sentMsg : 
        /\ msg.type = "prepare"
        /\ msg.dst = replica
        /\ \/ \E rec \in cmdLog[replica] : 
                /\ rec.inst = msg.inst
                /\ msg.ballot[1] > rec.ballot[1]
                /\ sentMsg' = (sentMsg \ {msg}) \cup
                            {[type  |-> "prepare-reply",
                              src   |-> replica,
                              dst   |-> msg.src,
                              inst  |-> rec.inst,
                              ballot|-> msg.ballot,
                              prev_ballot|-> rec.ballot,
                              status|-> rec.status,
                              cmd   |-> rec.cmd,
                              deps  |-> rec.deps,
                              seq   |-> rec.seq]}
                 /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                            {[inst  |-> rec.inst,
                              status|-> rec.status,
                              ballot|-> msg.ballot,
                              cmd   |-> rec.cmd,
                              deps  |-> rec.deps,
                              seq   |-> rec.seq]}]
                 /\ IF rec.inst \in leaderOfInst[replica] THEN
                        /\ leaderOfInst' = [leaderOfInst EXCEPT ![replica] = 
                                                                @ \ {rec.inst}]
                        /\ UNCHANGED << proposed, executed, committed,
                                        crtInst, ballots, preparing >>
                    ELSE UNCHANGED << proposed, executed, committed, crtInst,
                                      ballots, preparing, leaderOfInst >>
                        
           \/ /\ ~(\E rec \in cmdLog[replica] : rec.inst = msg.inst)
              /\ sentMsg' = (sentMsg \ {msg}) \cup
                            {[type  |-> "prepare-reply",
                              src   |-> replica,
                              dst   |-> msg.src,
                              inst  |-> msg.inst,
                              ballot|-> msg.ballot,
                              prev_ballot|-> << 0, replica >>,
                              status|-> "not-seen",
                              cmd   |-> none,
                              deps  |-> {},
                              seq   |-> 0]}
              /\ cmdLog' = [cmdLog EXCEPT ![replica] = @ \cup
                            {[inst  |-> msg.inst,
                              status|-> "not-seen",
                              ballot|-> msg.ballot,
                              cmd   |-> none,
                              deps  |-> {},
                              seq   |-> 0]}]
              /\ UNCHANGED << proposed, executed, committed, crtInst, ballots,
                              leaderOfInst, preparing >> 
    
PrepareFinalize(replica, i, Q) ==
    /\ i \in preparing[replica]
    /\ \E rec \in cmdLog[replica] :
       /\ rec.inst = i
       /\ rec.status \notin {"committed", "executed"}
       /\ LET replies == {msg \in sentMsg : 
                        /\ msg.inst = i
                        /\ msg.type = "prepare-reply"
                        /\ msg.dst = replica
                        /\ msg.ballot = rec.ballot} IN
            /\ (\A rep \in Q : \E msg \in replies : msg.src = rep)
            /\  \/ \E com \in replies :
                        /\ (com.status \in {"committed", "executed"})
                        /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                        /\ sentMsg' = sentMsg \ replies
                        /\ UNCHANGED << cmdLog, proposed, executed, crtInst, leaderOfInst,
                                        committed, ballots >>
                \/ /\ ~(\E msg \in replies : msg.status \in {"committed", "executed"})
                   /\ \E acc \in replies :
                        /\ acc.status = "accepted"
                        /\ (\A msg \in (replies \ {acc}) : 
                            (msg.prev_ballot[1] <= acc.prev_ballot[1] \/ 
                             msg.status # "accepted"))
                        /\ sentMsg' = (sentMsg \ replies) \cup
                                 [type  : {"accept"},
                                  src   : {replica},
                                  dst   : Q \ {replica},
                                  inst  : {i},
                                  ballot: {rec.ballot},
                                  cmd   : {acc.cmd},
                                  deps  : {acc.deps},
                                  seq   : {acc.seq}]
                        /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                                {[inst  |-> i,
                                  status|-> "accepted",
                                  ballot|-> rec.ballot,
                                  cmd   |-> acc.cmd,
                                  deps  |-> acc.deps,
                                  seq   |-> acc.seq]}]
                         /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                         /\ leaderOfInst' = [leaderOfInst EXCEPT ![replica] = @ \cup {i}]
                         /\ UNCHANGED << proposed, executed, crtInst, committed, ballots >>
                \/ /\ ~(\E msg \in replies : 
                        msg.status \in {"accepted", "committed", "executed"})
                   /\ LET preaccepts == {msg \in replies : msg.status = "pre-accepted"} IN
                       (\/  /\ \A p1, p2 \in preaccepts :
                                    p1.cmd = p2.cmd /\ p1.deps = p2.deps /\ p1.seq = p2.seq
                            /\ ~(\E pl \in preaccepts : pl.src = i[1])
                            /\ Cardinality(preaccepts) >= Cardinality(Q) - 1
                            /\ LET pac == CHOOSE pac \in preaccepts : TRUE IN
                                /\ sentMsg' = (sentMsg \ replies) \cup
                                         [type  : {"accept"},
                                          src   : {replica},
                                          dst   : Q \ {replica},
                                          inst  : {i},
                                          ballot: {rec.ballot},
                                          cmd   : {pac.cmd},
                                          deps  : {pac.deps},
                                          seq   : {pac.seq}]
                                /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                                        {[inst  |-> i,
                                          status|-> "accepted",
                                          ballot|-> rec.ballot,
                                          cmd   |-> pac.cmd,
                                          deps  |-> pac.deps,
                                          seq   |-> pac.seq]}]
                                 /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                                 /\ leaderOfInst' = [leaderOfInst EXCEPT ![replica] = @ \cup {i}]
                                 /\ UNCHANGED << proposed, executed, crtInst, committed, ballots >>
                        \/  /\ \A p1, p2 \in preaccepts : p1.cmd = p2.cmd /\ 
                                                          p1.deps = p2.deps /\
                                                          p1.seq = p2.seq
                            /\ ~(\E pl \in preaccepts : pl.src = i[1])
                            /\ Cardinality(preaccepts) < Cardinality(Q) - 1
                            /\ Cardinality(preaccepts) >= Cardinality(Q) \div 2
                            /\ LET pac == CHOOSE pac \in preaccepts : TRUE IN
                                /\ sentMsg' = (sentMsg \ replies) \cup
                                         [type  : {"try-pre-accept"},
                                          src   : {replica},
                                          dst   : Q,
                                          inst  : {i},
                                          ballot: {rec.ballot},
                                          cmd   : {pac.cmd},
                                          deps  : {pac.deps},
                                          seq   : {pac.seq}]
                                /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                                /\ leaderOfInst' = [leaderOfInst EXCEPT ![replica] = @ \cup {i}]
                                /\ UNCHANGED << cmdLog, proposed, executed,
                                                crtInst, committed, ballots >>
                        \/  /\ \/ \E p1, p2 \in preaccepts : p1.cmd # p2.cmd \/ 
                                                             p1.deps # p2.deps \/
                                                             p1.seq # p2.seq
                               \/ \E pl \in preaccepts : pl.src = i[1]
                               \/ Cardinality(preaccepts) < Cardinality(Q) \div 2
                            /\ preaccepts # {}
                            /\ LET pac == CHOOSE pac \in preaccepts : pac.cmd # none IN
                                /\ StartPhase1(pac.cmd, replica, Q, i, rec.ballot, replies)
                                /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                                /\ UNCHANGED << proposed, executed, crtInst, committed, ballots >>)
                \/  /\ \A msg \in replies : msg.status = "not-seen"
                    /\ StartPhase1(none, replica, Q, i, rec.ballot, replies)
                    /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                    /\ UNCHANGED << proposed, executed, crtInst, committed, ballots >>
                    
ReplyTryPreaccept(replica) ==
    \E tpa \in sentMsg :
        /\ tpa.type = "try-pre-accept" 
        /\ tpa.dst = replica
        /\ LET oldRec == {rec \in cmdLog[replica] : rec.inst = tpa.inst} IN
            /\ \A rec \in oldRec : rec.ballot[1] <= tpa.ballot[1] /\ 
                                   rec.status \notin {"accepted", "committed", "executed"}
            /\ \/ (\E rec \in cmdLog[replica] \ oldRec:
                        /\ tpa.inst \notin rec.deps
                        /\ \/ rec.inst \notin tpa.deps
                           \/ rec.seq >= tpa.seq
                        /\ sentMsg' = (sentMsg \ {tpa}) \cup
                                    {[type  |-> "try-pre-accept-reply",
                                      src   |-> replica,
                                      dst   |-> tpa.src,
                                      inst  |-> tpa.inst,
                                      ballot|-> tpa.ballot,
                                      status|-> rec.status]})
                        /\ UNCHANGED << cmdLog, proposed, executed, committed, crtInst,
                                        ballots, leaderOfInst, preparing >>
               \/ /\ (\A rec \in cmdLog[replica] \ oldRec: 
                            tpa.inst \in rec.deps \/ (rec.inst \in tpa.deps /\
                                                      rec.seq < tpa.seq))
                  /\ sentMsg' = (sentMsg \ {tpa}) \cup
                                    {[type  |-> "try-pre-accept-reply",
                                      src   |-> replica,
                                      dst   |-> tpa.src,
                                      inst  |-> tpa.inst,
                                      ballot|-> tpa.ballot,
                                      status|-> "OK"]}
                  /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                                    {[inst  |-> tpa.inst,
                                      status|-> "pre-accepted",
                                      ballot|-> tpa.ballot,
                                      cmd   |-> tpa.cmd,
                                      deps  |-> tpa.deps,
                                      seq   |-> tpa.seq]}]
                  /\ UNCHANGED << proposed, executed, committed, crtInst, ballots,
                                  leaderOfInst, preparing >>
                      
             
FinalizeTryPreAccept(cleader, i, Q) ==
    \E rec \in cmdLog[cleader]:
        /\ rec.inst = i
        /\ LET tprs == {msg \in sentMsg : msg.type = "try-pre-accept-reply" /\
                            msg.dst = cleader /\ msg.inst = i /\
                            msg.ballot = rec.ballot} IN
            /\ \A r \in Q: \E tpr \in tprs : tpr.src = r
            /\ \/ /\ \A tpr \in tprs: tpr.status = "OK"
                  /\ sentMsg' = (sentMsg \ tprs) \cup
                             [type  : {"accept"},
                              src   : {cleader},
                              dst   : Q \ {cleader},
                              inst  : {i},
                              ballot: {rec.ballot},
                              cmd   : {rec.cmd},
                              deps  : {rec.deps},
                              seq   : {rec.seq}]
                  /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {rec}) \cup
                            {[inst  |-> i,
                              status|-> "accepted",
                              ballot|-> rec.ballot,
                              cmd   |-> rec.cmd,
                              deps  |-> rec.deps,
                              seq   |-> rec.seq]}]
                  /\ UNCHANGED << proposed, executed, committed, crtInst, ballots,
                                  leaderOfInst, preparing >>
               \/ /\ \E tpr \in tprs: tpr.status \in {"accepted", "committed", "executed"}
                  /\ StartPhase1(rec.cmd, cleader, Q, i, rec.ballot, tprs)
                  /\ UNCHANGED << proposed, executed, committed, crtInst, ballots,
                                  leaderOfInst, preparing >>
               \/ /\ \E tpr \in tprs: tpr.status = "pre-accepted"
                  /\ \A tpr \in tprs: tpr.status \in {"OK", "pre-accepted"}
                  /\ sentMsg' = sentMsg \ tprs
                  /\ leaderOfInst' = [leaderOfInst EXCEPT ![cleader] = @ \ {i}]
                  /\ UNCHANGED << cmdLog, proposed, executed, committed, crtInst,
                                  ballots, preparing >>
         


(***************************************************************************)
(* Action groups                                                           *)
(***************************************************************************)        

CommandLeaderAction ==
    \/ (\E C \in (Commands \ proposed) :
            \E cleader \in Replicas : Propose(C, cleader))
    \/ (\E cleader \in Replicas : \E inst \in leaderOfInst[cleader] :
            \/ (\E Q \in FastQuorums(cleader) : Phase1Fast(cleader, inst, Q))
            \/ (\E Q \in SlowQuorums(cleader) : Phase1Slow(cleader, inst, Q))
            \/ (\E Q \in SlowQuorums(cleader) : Phase2Finalize(cleader, inst, Q))
            \/ (\E Q \in SlowQuorums(cleader) : FinalizeTryPreAccept(cleader, inst, Q)))
            
ReplicaAction ==
    \E replica \in Replicas :
        (\/ Phase1Reply(replica)
         \/ Phase2Reply(replica)
         \/ \E cmsg \in sentMsg : (cmsg.type = "commit" /\ Commit(replica, cmsg))
         \/ \E i \in Instances : 
            /\ crtInst[i[1]] > i[2] (* This condition states that the instance has *) 
                                    (* been started by its original owner          *)
            /\ \E Q \in SlowQuorums(replica) : SendPrepare(replica, i, Q)
         \/ ReplyPrepare(replica)
         \/ \E i \in preparing[replica] :
            \E Q \in SlowQuorums(replica) : PrepareFinalize(replica, i, Q)
         \/ ReplyTryPreaccept(replica))


(***************************************************************************)
(* Next action                                                             *)
(***************************************************************************)

Next == 
    \/ CommandLeaderAction
    \/ ReplicaAction


(***************************************************************************)
(* The complete definition of the algorithm                                *)
(***************************************************************************)

Spec == Init /\ [][Next]_vars


(***************************************************************************)
(* Theorems                                                                *)
(***************************************************************************)

Nontriviality ==
    \A i \in Instances :
        [](\A C \in committed[i] : C \in proposed \/ C = none)

Stability ==
    \A replica \in Replicas :
        \A i \in Instances :
            \A C \in Commands :
                []((\E rec1 \in cmdLog[replica] :
                    /\ rec1.inst = i
                    /\ rec1.cmd = C
                    /\ rec1.status \in {"committed", "executed"}) =>
                    [](\E rec2 \in cmdLog[replica] :
                        /\ rec2.inst = i
                        /\ rec2.cmd = C
                        /\ rec2.status \in {"committed", "executed"}))

Consistency ==
    \A i \in Instances :
        [](Cardinality(committed[i]) <= 1)

THEOREM Spec => ([]TypeOK) /\ Nontriviality /\ Stability /\ Consistency
                                                   





=============================================================================
\* Modification History
\* Last modified Sat Aug 24 12:25:28 EDT 2013 by iulian
\* Created Tue Apr 30 11:49:57 EDT 2013 by iulian
