------------------------- MODULE EPaxosHO_Orca_5r -------------------------
EXTENDS Naturals, FiniteSets, Sequences

-----------------------------------------------------------------------------

Max(S) == IF S = {} THEN 0 ELSE CHOOSE i \in S : \A j \in S : j <= i


(*********************************************************************************)
(* Constant parameters:                                                          *)
(*       Commands: the set of all commands                                       *)
(*       Replicas: the set of all EPaxos replicas                                *)
(*       FastQuorums(r): the set of all fast quorums where r is a command leader *)
(*       SlowQuorums(r): the set of all slow quorums where r is a command leader *)
(*       Consistency_level: the set of consistency level of all commands         *)
(*       Ctx_id: the set of context id of all commands                           *)
(*       Keys: the set of keys of all commands                                   *)
(*********************************************************************************)

CONSTANTS Commands,  Replicas, MaxBallot, Consistency_level, Ctx_id, Keys
 
 TwoElementSubsetsR(r) == {s \in SUBSET (Replicas \ {r}) : Cardinality(s) = 2}
 OneElementSubsetsR(r) == {s \in SUBSET (Replicas \ {r}) : Cardinality(s) = 1}
 SlowQuorums(r) == {{r} \cup s : s \in TwoElementSubsetsR(r)} (*for 3 replicas use OneElementSubsetsR().
 for 5 replicas use TwoElementsSubsetsR().*)
 
 FastQuorums(r) == {{r} \cup s : s \in TwoElementSubsetsR(r)}

ASSUME IsFiniteSet(Replicas)


(* { [op |-> [key |-> "x", type |-> "w"]], [op |-> [key |-> "y", type |-> "r"]], [op |-> [key |-> "x", type |-> "r"]], [op |-> [key |-> "y", type |-> "w"]], [op |-> [key |-> "w", type |-> "w"]], [op |-> [key |-> "z", type |-> "r"]]}*)


(***************************************************************************)
(* Quorum conditions:                                                      *)
(*  (simplified)                                                           *)
(***************************************************************************)

(*ASSUME \A r \in Replicas:
  /\ SlowQuorums(r) \subseteq SUBSET Replicas
  /\ \A SQ \in SlowQuorums(r): 
    /\ r \in SQ
    /\ Cardinality(SQ) = (Cardinality(Replicas) \div 2) + 1

ASSUME \A r \in Replicas:
  /\ FastQuorums(r) \subseteq SUBSET Replicas
  /\ \A FQ \in FastQuorums(r):
    /\ r \in FQ
    /\ Cardinality(FQ) = (Cardinality(Replicas) \div 2) + 
                         ((Cardinality(Replicas) \div 2) + 1) \div 2*)
                         

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
(* Special empty instance                                                  *)
(***************************************************************************)

emptyInstance == [inst   |-> <<0,0>>,
                  status |-> "none",
                  state |-> "none",
                  bal |-> <<0,0>>,
                  cmd    |-> {},
                  deps   |-> {},
                  seq    |-> 0,
                  consistency |-> "none",
                  ctxid |-> 0,
                  execution_order |-> 0,
                  execution_order_list |-> {},
                  commit_order |-> 0 ]


(***************************************************************************)
(* The instance space                                                      *)
(***************************************************************************)

Instances == Replicas \X (1..Cardinality(Commands))

(***************************************************************************)
(* The possible status of a command in the log of a replica.               *)
(***************************************************************************)

Status == {"not-seen", "pre-accepted", "accepted", "causally-committed", "strongly-committed", "executed" , "discarded", "none"}
State == {"not-seen", "ready", "waiting", "done", "none"}


(***************************************************************************)
(* All possible protocol messages:                                         *)
(***************************************************************************)

Message ==
        [type: {"pre-accept"}, inst: Instances, ballot: Nat \X Replicas, 
        cmd: Commands \cup {[op |-> [key |-> "", type |-> ""]]}, deps: SUBSET Instances,
         seq: Nat, consistency: Consistency_level, ctxid: Ctx_id \cup {0}, 
         commit_order: Nat, src: Replicas, dst: Replicas]
                                       
                                            
        
  \cup  [type: {"commit"}, inst: Instances, ballot: Nat \X Replicas, 
        cmd: Commands \cup {[op |-> [key |-> "", type |-> ""]]}, 
        deps: SUBSET Instances, seq: Nat, consistency: Consistency_level,
        ctxid: Ctx_id \cup {0}, commit_order: Nat]
   
  \cup  [type: {"accept"}, inst: Instances,  ballot: Nat \X Replicas, cmd: Commands \cup {[op |-> [key |-> "", type |-> ""]]},
        deps: SUBSET Instances, seq: Nat, consistency: Consistency_level, ctxid:  Ctx_id \cup {0}, commit_order: Nat, 
        src: Replicas, dst: Replicas]
        
        
  \cup  [type: {"prepare"}, inst: Instances ,  ballot: Nat \X Replicas, src: Replicas, dst: Replicas]
  
  \cup  [type: {"pre-accept-reply"}, inst: Instances, ballot: Nat \X Replicas,
        deps: SUBSET Instances,  seq: Nat, consistency: Consistency_level \cup {"not-seen"}, 
        ctxid:  Ctx_id \cup {0}, commit_order: Nat, src: Replicas, dst: Replicas, 
        committed: SUBSET Instances]
        
        
  \cup  [type: {"accept-reply"}, inst: Instances,  ballot: Nat \X Replicas, consistency: Consistency_level \cup {"not-seen"},
        commit_order: Nat, src: Replicas, dst: Replicas]
    
         
  \cup  [type: {"prepare-reply"}, inst: Instances, status: Status, ballot: Nat \X Replicas,cmd: Commands \cup {[op |-> [key |-> "", type |-> ""]]}, 
          deps: SUBSET Instances, seq: Nat, consistency: Consistency_level \cup {"not-seen"},   ctxid:  Ctx_id \cup {0}, 
          commit_order: Nat, src: Replicas, dst: Replicas,   prev_ballot: Nat \X Replicas]
  
  
  \cup  [type: {"try-pre-accept"}, src: Replicas, dst: Replicas, inst: Instances, ballot: Nat \X Replicas,  status: Status,
        cmd: Commands \cup {[op |-> [key |-> "", type |-> ""]]}, deps: SUBSET Instances, seq: Nat]
  \cup  [type: {"try-pre-accept-reply"}, src: Replicas, dst: Replicas, inst: Instances, ballot: Nat \X Replicas, status: Status \cup {"OK"}, consistency: Consistency_level, ctxid: Ctx_id \cup {0}]
        
        
        

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
(*          committed = maps commands to set of commit attributes              *)
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
                                       state: State,
                                       bal: Nat \X Replicas,
                                       vbal: Nat \X Replicas,
                                       cmd: Commands \cup {[op |-> [key |-> "", type |-> ""]]}, (* {[op |-> [key |-> "", type |-> ""]]} represents the empty command *)
                                       deps: SUBSET Instances,
                                       seq: Nat,
                                       consistency: Consistency_level \cup {"not-seen","none"},
                                       ctxid: Ctx_id \cup {0}, (* 0 means unknown context id *)
                                       execution_order: Nat, (* This is the global order of execution in a specific replica. Ordering will start from 1. O means not executed yet. *)
                                       execution_order_list: SUBSET {Nat \X Instances},
                                       commit_order : Nat (* 0 means not committed *)
                                       ]]
    /\ proposed \in SUBSET Commands
    /\ executed \in [Replicas -> SUBSET (Nat \X Commands)]
    /\ sentMsg \in SUBSET Message
    /\ crtInst \in [Replicas -> Nat]
    /\ leaderOfInst \in [Replicas -> SUBSET Instances]
    /\ committed \in [Instances -> SUBSET ((Commands \cup {[op |-> [key |-> "", type |-> ""]]}) \X
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
(* Helper Functions                                                        *)
(***************************************************************************)

waitedDeps(deps, replica) == LET waitingRecs== {rec \in cmdLog[replica]: (rec.state = "waiting" \/ rec.state="not-seen") /\ rec.inst \in deps} 
                         waitingInst=={rec.inst: rec \in waitingRecs}
                         allRecs == {rec \in cmdLog[replica]: TRUE}
                         allInst == {rec.inst: rec \in allRecs} 
                         notReachedDeps == {dep \in deps: dep \notin allInst} (* instance of deps that are not reached yet in this replica*)
                         finalWaitingInst == waitingInst \cup notReachedDeps IN
                         finalWaitingInst

RECURSIVE checkWaiting(_,_)
checkWaiting(deps, replica) == LET wDeps == waitedDeps(deps, replica) IN
                         IF Cardinality(wDeps) = 0 THEN
                            "ready"
                         ELSE checkWaiting(wDeps, replica)
                         
FindingWaitingInst(finalDeps) ==
    LET waitingRecs(deps) == {rec \in cmdLog[deps]: rec.state = "waiting"}
        waitingInst(deps) == {rec.inst: rec \in waitingRecs(deps)}
        allWaitingInst == UNION({waitingInst(deps): deps \in finalDeps})
    IN
        allWaitingInst

IsAllCommitted == \A replica \in Replicas : (* check whether all the commands are committed across all the replicas *)
                           LET recs ==  {rec.inst: rec \in cmdLog[replica]} IN
                                /\ \A rec \in recs: rec.status = "causally-committed" \/ rec.status = "strongly-committed" \/ rec.status = "executed" \/ rec.status = "discarded"


IsAllExecutedOrDiscarded == \A replica \in Replicas : (* check whether all the commands are executed or discarded across all the replicas *)
                           LET recs ==  {rec: rec \in cmdLog[replica]} IN
                                /\ \A rec \in recs:   rec.status \in {"executed"} \/ rec.status \in {"discarded"}

MaxSeq(Replica) == LET recs == {rec: rec \in cmdLog[Replica] } IN
                        CHOOSE rec \in recs : \A otherrecs \in recs:
                            rec.seq >= otherrecs.seq 
                            

SameCtxScc(ordered_scc, ctx_id, replica) == (* return instances from the same context *)
    LET 
        RECURSIVE ctxScc(_, _, _, _)
        ctxScc(scc, ctx, r, passed_scc) ==
            IF scc = {} THEN
                passed_scc
            ELSE
                LET  
                    
                    node == CHOOSE n \in scc: TRUE
                    recs == {rec \in cmdLog[r]: rec.ctxid = ctx_id /\ rec.inst = node[2] /\ rec.inst[1] = r} 
                    inst == {rec.inst: rec \in recs} IN
                    IF inst = {} THEN
                    ctxScc(scc \ {node}, ctx, r, passed_scc)  
                    ELSE
                    ctxScc(scc \ {node}, ctx, r, passed_scc \cup {node})                  
    IN
        ctxScc(ordered_scc, ctx_id, replica,  {})             
        

MinInst(allInstances) ==
    CHOOSE inst \in allInstances : \A otherInst \in allInstances : 
    
        inst[2][2] <= otherInst[2][2]

OrderingBasedOnInstanceNumber(scc) ==  (*ordering based on instance number (ascending)*) 
     LET
        RECURSIVE minCover(_, _, _)
        minCover(SeqSet, Cover, i) ==
            IF SeqSet = {}
            THEN Cover
            ELSE
                LET inst == MinInst(SeqSet)
                inst1 == <<>>
                j == (i+1)
                inst2 == Append(inst1,j)
                inst3 == Append(inst2,inst[2]) IN
                        minCover(SeqSet \ {inst}, Cover \cup {inst3}, i)
     IN
       minCover(scc, {},0) 
       
MaxWriteInstance(Replica,deps) == LET recs == {rec \in cmdLog[Replica]: rec.inst \in deps } IN (*returns the maximum sequence number among all the write operations *)
                            IF recs = {} THEN emptyInstance
                            ELSE
                                CHOOSE rec \in recs : \A otherrecs \in recs:
                                    /\ rec.execution_order >= otherrecs.execution_order
                                    
                            
                            
MinExecutionOrderRecs(recs) == 
                            CHOOSE rec \in recs : \A otherrecs \in recs: (* returns the rec with the minimum sequence number *)
                            /\ rec.execution_order <= otherrecs.execution_order
                            
                            
FindMaxExecutionOrder(replica) == LET allrecs == {rec: rec \in cmdLog[replica]} IN (* return the instance with the highest execution_order *)
                                        CHOOSE rec \in allrecs : \A otherrec \in allrecs : 
                                             rec.execution_order >= otherrec.execution_order
                                             
                                             
                                             
LatestWrite(key, replica) ==  LET recs1 == {rec: rec \in cmdLog[replica]}
                                  recs2 == {rec \in recs1: rec.cmd.op.key = key /\ rec.cmd.op.type = "w"} IN
                                  IF recs2 = {} THEN emptyInstance
                                  ELSE
                                   CHOOSE rec \in recs2 : \A otherrecs \in recs2:
                                    /\ rec.execution_order >= otherrecs.execution_order
                                    
                                                                        
LatestWriteofSpecificKey(key)  ==  (* finidng the latest write of a specific key across all the replicas *)
    LET 
        RECURSIVE latestWrite(_, _, _)
        latestWrite(k, r, lw) == 
            IF r = {}
            THEN lw
            ELSE
                LET replica == CHOOSE x \in r: TRUE
                lw2 == LatestWrite(k, replica)
                IN
                IF lw2.status = "none" THEN
                latestWrite(k, r \ {replica}, lw)
                ELSE
                latestWrite(k, r \ {replica}, lw \cup {lw2})
                
    IN
        latestWrite(key, Replicas, {})
        
     
        
MaxCommitNumber(replica) ==  LET recs == {rec \in cmdLog[replica]: rec.status \in {"strongly-committed", "executed", "discarded"} } IN
                                IF recs = {} THEN emptyInstance
                                ELSE 
                                    CHOOSE rec \in recs : \A otherrecs \in recs:
                                    /\ rec.commit_order >= otherrecs.commit_order
                          
        
        
MaxCommitOrder ==  (* finding the max commit order among all the replicas *)
     LET 
        RECURSIVE maxCommit(_, _)
        maxCommit(r, max) ==
            IF r = {}
            THEN max
            ELSE
                LET
                    replica == CHOOSE x \in r: TRUE
                    max2 == MaxCommitNumber(replica) IN
                    IF max2.status # "none" /\ max2.commit_order >= max THEN
                    maxCommit(r \ {replica}, max2.commit_order)
                    ELSE
                    maxCommit(r \ {replica}, max)
    IN 
        maxCommit(Replicas, 1) (* initial commit order number is 1 *)
        
        
       
 DependentWriteInstances(deps_list, replica) == (* finding only dependent causal write commands *) (* return {<<"a", 1>>, <<"b", 0>>} *)
    LET 
        RECURSIVE depWriteInstance(_,_,_)
            depWriteInstance(dlist, r, fdlist) ==
                IF dlist = {}
                THEN fdlist
                ELSE
                    LET
                        dep == CHOOSE x \in dlist: TRUE
                        rec1 == {rec: rec \in cmdLog[r]}
                        rec2 == {rec \in rec1:  rec.inst = dep /\ rec.cmd.op.type = "w" /\ rec.consistency \in {"causal"}} 
                        inst == {rec.inst: rec \in rec2} IN
                        depWriteInstance(dlist \ {dep}, r, fdlist \cup inst)           
    IN
        depWriteInstance(deps_list, replica, {})
        
        
(*IsMajorityCommitted(inst) == (* finding in how many replicas the instance is committed *)
    LET 
        RECURSIVE majorityCommitted(_, _, _)
        majorityCommitted(i, r, flist) ==
            IF r = {}
            THEN flist
            ELSE
                LET
                    replica == CHOOSE x \in r: TRUE
                    rec1 == {rec: rec \in cmdLog[replica]}
                    rec2 == {rec \in rec1: rec.inst = i /\ rec.status \in {"causally-committed", "executed", "discarded"}}
                    inst2 == {rec.inst: rec \in rec2} IN
                    majorityCommitted(i, r \ {replica}, flist \cup inst2)
    IN 
        majorityCommitted(inst, Replicas, {})*)
        
        
IsMajorityCommitted(inst) ==
    LET 
        RECURSIVE majorityCommitted(_, _, _,_)
        majorityCommitted(i, r, flist, count) ==
            IF r = {}
            THEN count
            ELSE
                LET
                    replica == CHOOSE x \in r: TRUE
                    rec1 == {rec: rec \in cmdLog[replica]}
                    rec2 == {rec \in rec1: rec.inst = i /\ rec.status \in {"causally-committed", "strongly-committed", "executed", "discarded"}}
                    inst2 == {rec.inst: rec \in rec2} IN
                    IF Cardinality(inst2) # 0 THEN 
                    majorityCommitted(i, r \ {replica}, flist \cup inst2, count+1)
                    ELSE 
                    majorityCommitted(i, r \ {replica}, flist \cup inst2, count)
    IN 
        majorityCommitted(inst, Replicas, {}, 0)
        
        
CheckStrongRead(rec) ==  IF rec.cmd.op.type = "r" /\ rec.consistency \in {"strong"} /\ rec.status \in {"strongly-committed", "executed"} THEN
                            TRUE
                         ELSE
                            FALSE      
                            
CheckDependentWrite(rec, replica) == LET deps_list == rec.deps 
                                        dep_write_instances ==  DependentWriteInstances(deps_list, replica) IN  (* retrieving the dependent causal write commnads for a specific strong read command *)
                                        IF Cardinality(dep_write_instances) = 0 THEN TRUE
                                        ELSE 
                                            /\ \A inst \in dep_write_instances : LET noOfCommit == IsMajorityCommitted(inst) IN (* retrieving the replicas where the instance is committed or executed or discarded *)
                                                /\ noOfCommit >= ((Cardinality(Replicas) \div 2) + 1)
                                                
                            
(***************************************************************************)
(* Actions                                                                 *)
(***************************************************************************)

StartPhase1Causal(C, cleader, Q, inst, ballot, oldMsg, cl,ctx) ==
         LET recs1 == {rec \in cmdLog[cleader]: rec.ctxid = ctx}
           deps1 == {rec2.inst: rec2 \in recs1} (* same session dependency *)
           
           maxRec == MaxSeq(cleader) (* selecting the rec with the highest sequence number in this specific replica *)
           recs2 == {rec \in cmdLog[cleader]: rec.state = "executed" /\ rec.cmd.op.type = "w" /\ rec.cmd.op.key = C.op.key  /\ rec.seq = maxRec.seq /\ C.op.type = "r"} 
           (* rec.state = "executed" /\ rec.cmd.op.type = "w" => latest executed write 
           rec.cmd.op.key = C.op.key  => key of the command to commit and the dependecy is the same.
           rec.seq = maxRec.seq => select the rec with the maximum seq that means the latest write operation
           C.op.type = "r" => will add this dependency only if the command to commit is a read command *)
           deps2 == {rec.inst: rec \in recs2} (* get from and transitive dependency *)
           (*deps == deps1 \cup deps2*) (* taking union of same session dependency and get from dependency to calculate the transitive dependency *)
           (* no need to calculate the transitive dependency. It will be calculated during the graph formation in the execution phase *) 
            newDeps == deps1 \cup deps2
            
            recs3 == {rec \in cmdLog[cleader]: rec.cmd.op.key = C.op.key} (* maximum oberved seq of a command that have the same key as C*)
            recsUnion == recs1 \cup recs2 \cup recs3
            newSeq == 1 + Max({t.seq: t \in recsUnion}) 
            oldRecs == {rec \in cmdLog[cleader] : rec.inst = inst}
            
            waitingInst== waitedDeps(newDeps, cleader) IN
            IF Cardinality(waitingInst) = 0 THEN
                /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ oldRecs) \cup 
                                        {[inst   |-> inst,
                                          status |-> "causally-committed",
                                          state |-> "done",
                                          bal |-> ballot,
                                          vbal |-> ballot,
                                          cmd    |-> C,
                                          deps   |-> newDeps,
                                          seq    |-> newSeq,
                                          consistency |-> cl,
                                          ctxid |-> ctx,
                                          execution_order |-> 0,
                                          execution_order_list |-> {},
                                          commit_order |-> 0 ]}]   (* incase of causal command, the commit order will be always 0 and will not be tracked *)
                /\ leaderOfInst' = [leaderOfInst EXCEPT ![cleader] = @ \cup {inst}]
                /\ sentMsg' = (sentMsg \ oldMsg) \cup 
                                        [type  : {"commit"},
                                          inst  : {inst},
                                          ballot: {ballot},
                                          cmd   : {C},
                                          deps  : {newDeps},
                                          seq   : {newSeq},
                                          consistency : {cl},
                                          ctxid : {ctx},
                                          commit_order : {0}]
                  

           ELSE
                 /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ oldRecs) \cup 
                                        {[inst   |-> inst,
                                          status |-> "causally-committed",
                                          state |-> "waiting",
                                          bal    |-> ballot,
                                          vbal |-> ballot,
                                          cmd    |-> C,
                                          deps   |-> newDeps,
                                          seq    |-> newSeq,
                                          consistency |-> cl,
                                          ctxid |-> ctx,
                                          execution_order |-> 0,
                                          execution_order_list |-> {},
                                          commit_order |-> 0 ]}]
                /\ LET newcmdstate == checkWaiting(waitingInst, cleader) IN
                     /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ oldRecs) \cup 
                        {[inst   |-> inst,
                          status |-> "causally-committed",
                          state |-> "done",
                          bal   |-> ballot,
                          vbal |-> ballot,
                          cmd    |-> C,
                          deps   |-> newDeps,
                          seq    |-> newSeq,
                          consistency |-> cl,
                          ctxid |-> ctx,
                          execution_order |-> 0,
                          execution_order_list |-> {},
                          commit_order |-> 0 ]}]
                    /\ leaderOfInst' = [leaderOfInst EXCEPT ![cleader] = @ \cup {inst}]
                    /\ sentMsg' = (sentMsg \ oldMsg) \cup 
                                            [type  : {"commit"},
                                              inst  : {inst},
                                              ballot: {ballot},
                                              cmd   : {C},
                                              deps  : {newDeps},
                                              seq   : {newSeq},
                                              consistency : {cl},
                                              ctxid : {ctx},
                                              commit_order : {0}]
                                              
                                              
                                              
StartPhase1Strong(C, cleader, Q, inst, ballot, oldMsg, cl,ctx) ==

    LET recs1 == {rec \in cmdLog[cleader]: rec.ctxid = ctx}
           deps1 == {rec.inst: rec \in recs1} (* same session dependency *)
           maxRec == MaxSeq(cleader) (* selecting the rec with the highest sequence number in this specific replica *)
           recs2 == {rec \in cmdLog[cleader]: rec.state = "executed" /\ rec.cmd.op.type = "w" /\ rec.cmd.op.key = C.op.key  /\ rec.seq = maxRec.seq /\ C.op.type = "r"} 
           (* rec.state = "executed" /\ rec.cmd.op.type = "w" => latest executed write 
           rec.cmd.op.key = C.op.key  => key of the command to commit and the dependecy is the same.
           rec.seq = maxRec.seq => select the rec with the maximum seq that means the latest write operation
           C.op.type = "r" => will add this dependency only if the command to commit is a read command *)
           deps2 == {rec.inst: rec \in recs2} (* get from and transitive dependency *)
           
           (*deps == deps1 \cup deps2*) (* taking union of same session dependency and get from dependency to calculate the transitive dependency *)
           (* no need to calculate the transitive dependency. It will be calculated during the graph formation in the execution phase *) 
           recs3 == {rec \in cmdLog[cleader]: rec.cmd.op.key = C.op.key}
           deps3 == {rec.inst: rec \in recs3} (* command interference *)
           newDeps == deps1 \cup deps2 \cup deps3 
           maxCommit == MaxCommitOrder (* finding the max commit order among all the strong committed instances across the replicas *)
           recsUnion == recs1 \cup recs2 \cup recs3
            newSeq == 1 + Max({t.seq: t \in recsUnion}) 
            oldRecs == {rec \in cmdLog[cleader] : rec.inst = inst} 
            
            waitingInst== waitedDeps(newDeps, cleader) IN
            IF Cardinality(waitingInst) = 0 THEN
                /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ oldRecs) \cup 
                                        {[inst   |-> inst,
                                          status |-> "pre-accepted",
                                          state |-> "done",
                                          bal |-> ballot,
                                          vbal |-> ballot,
                                          cmd    |-> C,
                                          deps   |-> newDeps,
                                          seq    |-> newSeq,
                                          consistency |-> cl,
                                          ctxid |-> ctx,
                                          execution_order |-> 0,
                                          execution_order_list |-> {},
                                          commit_order |-> maxCommit+1 ]}]
                /\ leaderOfInst' = [leaderOfInst EXCEPT ![cleader] = @ \cup {inst}]
                /\ sentMsg' = (sentMsg \ oldMsg) \cup 
                                        [type  : {"pre-accept"},
                                          src   : {cleader},
                                          dst   : Q \ {cleader},
                                          inst  : {inst},
                                          ballot: {ballot},
                                          cmd   : {C},
                                          deps  : {newDeps},
                                          seq   : {newSeq},
                                          consistency : {cl},
                                          ctxid : {ctx},
                                          commit_order : {maxCommit+1}]
                                          
           ELSE
                 /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ oldRecs) \cup 
                                        {[inst   |-> inst,
                                          status |-> "pre-accepted",
                                          state |-> "waiting",
                                          bal |-> ballot,
                                          vbal |-> ballot,
                                          cmd    |-> C,
                                          deps   |-> newDeps,
                                          seq    |-> newSeq,
                                          consistency |-> cl,
                                          ctxid |-> ctx,
                                          execution_order |-> 0,
                                          execution_order_list |-> {},
                                          commit_order |-> maxCommit+1]}]
                /\ LET newcmdstate == checkWaiting(waitingInst, cleader) IN
                     /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ oldRecs) \cup 
                        {[inst   |-> inst,
                          status |-> "pre-accepted",
                          state  |-> "done",
                          bal    |-> ballot,
                          vbal |-> ballot,
                          cmd    |-> C,
                          deps   |-> newDeps,
                          seq    |-> newSeq,
                          consistency |-> cl,
                          ctxid |-> ctx,
                           execution_order |-> 0,
                          execution_order_list |-> {},
                          commit_order |-> maxCommit+1]}]
                    /\ leaderOfInst' = [leaderOfInst EXCEPT ![cleader] = @ \cup {inst}]
                    /\ sentMsg' = (sentMsg \ oldMsg) \cup 
                                            [type  : {"pre-accept"},
                                              src   : {cleader},
                                              dst   : Q \ {cleader},
                                              inst  : {inst},
                                              ballot: {ballot},
                                              cmd   : {C},
                                              deps  : {newDeps},
                                              seq   : {newSeq},
                                              consistency : {cl},
                                              ctxid : {ctx},
                                              commit_order : {maxCommit+1}]


StartPhase1(C, cleader, Q, inst, ballot, oldMsg, cl,ctx) ==
    IF cl = "causal" THEN
        StartPhase1Causal(C, cleader, Q, inst, ballot, oldMsg,cl,ctx)
           
     ELSE
         StartPhase1Strong(C, cleader, Q, inst, ballot, oldMsg, cl,ctx)
         
           
Propose(C, cleader,cl,ctx) ==
    LET newInst == <<cleader, crtInst[cleader]>> 
        newBallot == <<0, cleader>> 
    IN  /\ proposed' = proposed \cup {C}
        /\ (\E Q \in FastQuorums(cleader):
                 StartPhase1(C, cleader, Q, newInst, newBallot, {},cl,ctx))
        /\ crtInst' = [crtInst EXCEPT ![cleader] = @ + 1]
        /\ UNCHANGED << executed, committed, ballots, preparing>>
        
        
        
Phase1Reply(replica) ==
    \E msg \in sentMsg:
        /\ msg.type = "pre-accept"
        /\ msg.dst = replica
        /\ LET oldRec == {rec \in cmdLog[replica]: rec.inst = msg.inst} IN
            /\ (\A rec \in oldRec : 
                (rec.bal = msg.ballot \/ rec.bal[1] < msg.ballot[1]))
            /\ LET (*recs1 == {rec \in cmdLog[replica]: rec.ctxid = msg.ctx}
                   deps1 == {rec.inst: rec \in recs1}*) (* same session dependency *)
                   maxRec == MaxSeq(replica) (* selecting the rec with the highest sequence number in this specific replica *)
                   recs2 == {rec \in cmdLog[replica]: rec.state = "executed" /\ rec.cmd.op.type = "w" /\ rec.cmd.op.key = msg.cmd.op.key  /\ rec.seq = maxRec.seq /\ msg.cmd.op.type = "r"} 
                   (* rec.state = "executed" /\ rec.cmd.op.type = "w" => latest executed write 
                   rec.cmd.op.key = C.op.key  => key of the command to commit and the dependecy is the same.
                   rec.seq = maxRec.seq => select the rec with the maximum seq that means the latest write operation
                   C.op.type = "r" => will add this dependency only if the command to commit is a read command *)
                   deps2 == {rec.inst: rec \in recs2} (* get from and transitive dependency *)
                   
                   (*deps == deps1 \cup deps2*) (* taking union of same session dependency and get from dependency to calculate the transitive dependency *)
                   (* no need to calculate the transitive dependency. It will be calculated during the graph formation in the execution phase *) 
                   recs3 == {rec \in cmdLog[replica]: rec.cmd.op.key = msg.cmd.op.key}
                   deps3 == {rec.inst: rec \in recs3} (* command interference *)
            
                   newDeps == msg.deps \cup deps2 \cup deps3
                   recsUnion == recs2 \cup recs3
                   newSeq == 1+ Max({t.seq: t \in recsUnion} \cup {msg.seq})
                   instCom == {t.inst: t \in {tt \in cmdLog[replica] :
                              tt.status \in {"causally-committed", "strongly-committed", "executed", "discarded"}}}
  
                    finalWaitingInst == waitedDeps(newDeps, replica)
                    
                    IN
                    IF Cardinality(finalWaitingInst) = 0 THEN
                        /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                                            {[inst   |-> msg.inst,
                                              status |-> "pre-accepted",
                                              state  |-> "done",
                                              bal    |-> msg.ballot,
                                              vbal    |-> msg.ballot,
                                              cmd    |-> msg.cmd,
                                              deps   |-> newDeps,
                                              seq    |-> newSeq,
                                              consistency |-> msg.consistency,
                                              ctxid |-> msg.ctxid,
                                              execution_order |-> 0,
                                              execution_order_list |-> {},
                                              commit_order |-> msg.commit_order]}]
                        /\ sentMsg' = (sentMsg \ {msg}) \cup
                                            {[type  |-> "pre-accept-reply",
                                              src   |-> replica,
                                              dst   |-> msg.src,
                                              inst  |-> msg.inst,
                                              ballot|-> msg.ballot,
                                              deps  |-> newDeps,
                                              seq   |-> newSeq,
                                              committed|-> instCom,
                                              consistency |-> msg.consistency,
                                              ctxid |-> msg.ctxid,
                                              commit_order |-> msg.commit_order]}
                        /\ UNCHANGED << proposed, crtInst, executed, leaderOfInst,
                                        committed, ballots, preparing >>
                    ELSE
                        /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                                            {[inst   |-> msg.inst,
                                              status |-> "pre-accepted",
                                              state  |-> "waiting",
                                              bal |-> msg.ballot,
                                              vbal |-> msg.ballot,
                                              cmd    |-> msg.cmd,
                                              deps   |-> newDeps,
                                              seq    |-> newSeq,
                                              consistency |-> msg.consistency,
                                              ctxid |-> msg.ctxid,
                                              execution_order |-> 0,
                                              execution_order_list |-> {},
                                              commit_order |-> msg.commit_order  ]}]
                        /\ LET newcmdstate == checkWaiting(finalWaitingInst, replica) IN
                            /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                                            {[inst   |-> msg.inst,
                                              status |-> "pre-accepted",
                                              state  |-> "done",
                                              bal |-> msg.ballot,
                                              vbal |-> msg.ballot,
                                              cmd    |-> msg.cmd,
                                              deps   |-> newDeps,
                                              seq    |-> newSeq,
                                              consistency |-> msg.consistency,
                                              ctxid |-> msg.ctxid,
                                              execution_order |-> 0,
                                              execution_order_list |-> {},
                                              commit_order |-> msg.commit_order]}]
                             /\ sentMsg' = (sentMsg \ {msg}) \cup
                                            {[type  |-> "pre-accept-reply",
                                              src   |-> replica,
                                              dst   |-> msg.src,
                                              inst  |-> msg.inst,
                                              ballot|-> msg.ballot,
                                              deps  |-> newDeps,
                                              seq   |-> newSeq,
                                              committed|-> instCom,
                                              consistency |-> msg.consistency,
                                              ctxid |-> msg.ctxid,
                                              commit_order |-> msg.commit_order]}
                             /\ UNCHANGED << proposed, crtInst, executed, leaderOfInst,
                                        committed, ballots, preparing >>
                
                
                            
Phase1Fast(cleader, i, Q) ==
    /\ i \in leaderOfInst[cleader]
    /\ Q \in FastQuorums(cleader)
    /\ \E record \in cmdLog[cleader]:
        /\ record.inst = i
        /\ record.status = "pre-accepted"
        /\ record.bal[1] = 0
        /\ LET replies == {msg \in sentMsg: 
                                /\ msg.inst = i
                                /\ msg.type = "pre-accept-reply"
                                /\ msg.dst = cleader
                                /\ msg.src \in Q
                                /\ msg.ballot = record.bal} IN
            /\ (\A replica \in (Q \ {cleader}): 
                    \E msg \in replies: msg.src = replica)
            /\ (\A r1, r2 \in replies:
                /\ r1.deps = r2.deps
                /\ r1.seq = r2.seq)
            /\ LET r == CHOOSE r \in replies : TRUE
                   
                   waitingInst == waitedDeps(r.deps, cleader) IN               
                        IF Cardinality(waitingInst) = 0 THEN
                        /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {record}) \cup 
                                                {[inst   |-> i,
                                                  status |-> "strongly-committed",
                                                  state |-> "done",
                                                  bal |-> record.bal,
                                                  vbal |-> record.bal,
                                                  cmd    |-> record.cmd,
                                                  deps   |-> r.deps,
                                                  seq    |-> r.seq,
                                                  consistency |-> record.consistency,
                                                  ctxid |-> record.ctxid,
                                                  execution_order |-> 0,
                                                  execution_order_list |-> {},
                                                  commit_order |-> r.commit_order ]}]
                                            
                          /\
                            sentMsg' = (sentMsg \ replies) \cup
                            {[type  |-> "commit",
                            inst    |-> i,
                            ballot  |-> record.bal,
                            cmd     |-> record.cmd,
                            deps    |-> r.deps,
                            seq     |-> r.seq,
                            consistency |-> record.consistency,
                            ctxid |-> record.ctxid,
                            commit_order |-> r.commit_order]}
                        /\ leaderOfInst' = [leaderOfInst EXCEPT ![cleader] = @ \ {i}]
                        /\ committed' = [committed EXCEPT ![i] = 
                                                    @ \cup {<<record.cmd, r.deps, r.seq>>}]
                        /\ UNCHANGED << proposed, executed, crtInst, ballots, preparing >> 
                        
                     ELSE  
                        /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {record}) \cup 
                                                {[inst   |-> i,
                                                  status |-> "strongly-committed",
                                                  state |-> "waiting",
                                                  bal |-> record.bal,
                                                  vbal |-> record.bal,
                                                  cmd    |-> record.cmd,
                                                  deps   |-> r.deps,
                                                  seq    |-> r.seq,
                                                  consistency |-> record.consistency,
                                                  ctxid |-> record.ctxid,
                                                  execution_order |-> 0,
                                                  execution_order_list |-> {},
                                                  commit_order |-> r.commit_order ]}]
   
                        /\ LET newcmdstate == checkWaiting(waitingInst, cleader) IN   
                        
                            /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {record}) \cup 
                                                    {[inst   |-> i,
                                                      status |-> "strongly-committed",
                                                      state |-> "done",
                                                      bal |-> record.bal,
                                                      vbal |-> record.bal,
                                                      cmd    |-> record.cmd,
                                                      deps   |-> r.deps,
                                                      seq    |-> r.seq,
                                                      consistency |-> record.consistency,
                                                      ctxid |-> record.ctxid,
                                                      execution_order |-> 0,
                                                      execution_order_list |-> {},
                                                      commit_order |-> r.commit_order ]}]
                             /\ sentMsg' = (sentMsg \ replies) \cup
                                {[type  |-> "commit",
                                inst    |-> i,
                                ballot  |-> record.bal,
                                cmd     |-> record.cmd,
                                deps    |-> r.deps,
                                seq     |-> r.seq,
                                consistency |-> record.consistency,
                                ctxid |-> record.ctxid,
                                commit_order |-> r.commit_order]}
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
                                /\ msg.ballot = record.bal} IN
            /\ (\A replica \in (Q \ {cleader}): \E msg \in replies: msg.src = replica)
            /\ LET finalDeps == UNION {msg.deps : msg \in replies}
                   finalSeq == Max({msg.seq : msg \in replies})
                    waitingInst == waitedDeps(finalDeps, cleader)  IN   
                        IF Cardinality(waitingInst) = 0 THEN
                            /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {record}) \cup 
                                                    {[inst   |-> i,
                                                      status |-> "accepted",
                                                      state  |-> "done", 
                                                      bal |-> record.bal,
                                                      vbal |-> record.bal,
                                                      cmd    |-> record.cmd,
                                                      deps   |-> finalDeps,
                                                      seq    |-> finalSeq,
                                                      consistency |-> record.consistency, 
                                                      ctxid |-> record.ctxid,
                                                      execution_order |-> 0,
                                                      execution_order_list |-> {},
                                                      commit_order |-> record.commit_order]}]
                            /\ \E SQ \in SlowQuorums(cleader):
                               (sentMsg' = (sentMsg \ replies) \cup
                                        [type : {"accept"},
                                        src : {cleader},
                                        dst : SQ \ {cleader},
                                        inst : {i},
                                        ballot: {record.bal},
                                        cmd : {record.cmd},
                                        deps : {finalDeps},
                                        seq : {finalSeq},
                                        consistency : {record.consistency},
                                        ctxid : {record.ctxid},
                                        commit_order : {record.commit_order}])
                            /\ UNCHANGED << proposed, executed, crtInst, leaderOfInst,
                                            committed, ballots, preparing >>
                         ELSE
                            /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {record}) \cup 
                                                    {[inst   |-> i,
                                                      status |-> "accepted",
                                                      state  |-> "waiting", 
                                                      bal |-> record.bal,
                                                      vbal |-> record.bal,
                                                      cmd    |-> record.cmd,
                                                      deps   |-> finalDeps,
                                                      seq    |-> finalSeq,
                                                      consistency |-> record.consistency, 
                                                      ctxid |-> record.ctxid,
                                                      execution_order |-> 0,
                                                      execution_order_list |-> {},
                                                      commit_order |-> record.commit_order  ]}]
                            /\ LET newcmdstate == checkWaiting(waitingInst, cleader) IN
                                /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {record}) \cup 
                                                    {[inst   |-> i,
                                                      status |-> "accepted",
                                                      state  |-> "done", 
                                                      bal |-> record.bal,
                                                      vbal |-> record.bal,
                                                      cmd    |-> record.cmd,
                                                      deps   |-> finalDeps,
                                                      seq    |-> finalSeq,
                                                      consistency |-> record.consistency, 
                                                      ctxid |-> record.ctxid,
                                                      execution_order |-> 0,
                                                      execution_order_list |-> {},
                                                      commit_order |-> record.commit_order  ]}]
                                /\ \E SQ \in SlowQuorums(cleader):
                                   (sentMsg' = (sentMsg \ replies) \cup
                                            [type : {"accept"},
                                            src : {cleader},
                                            dst : SQ \ {cleader},
                                            inst : {i},
                                            ballot: {record.bal},
                                            cmd : {record.cmd},
                                            deps : {finalDeps},
                                            seq : {finalSeq},
                                            consistency : {record.consistency},
                                            ctxid : {record.ctxid},
                                            commit_order : {record.commit_order}])
                                /\ UNCHANGED << proposed, executed, crtInst, leaderOfInst,
                                                committed, ballots, preparing >>
       
                                                
                                                
  Phase2Reply(replica) ==
    \E msg \in sentMsg: 
        /\ msg.type = "accept"
    /\ msg.dst = replica
    /\ LET oldRec == {rec \in cmdLog[replica]: rec.inst = msg.inst}
         
           waitingInst == waitedDeps(msg.deps, replica)  IN
          IF Cardinality(waitingInst) = 0 THEN
        /\ (\A rec \in oldRec: (rec.bal = msg.ballot \/ 
                                rec.bal[1] < msg.ballot[1]))
        /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                            {[inst   |-> msg.inst,
                              status |-> "accepted",
                              state  |-> "done", 
                              bal    |-> msg.ballot,
                              vbal    |-> msg.ballot,
                              cmd    |-> msg.cmd,
                              deps   |-> msg.deps,
                              seq    |-> msg.seq,
                              consistency |-> msg.consistency, 
                              ctxid |-> msg.ctxid,
                              execution_order |-> 0,
                              execution_order_list |-> {},
                              commit_order |-> msg.commit_order ]}]
        /\ sentMsg' = (sentMsg \ {msg}) \cup
                                {[type  |-> "accept-reply",
                                  src   |-> replica,
                                  dst   |-> msg.src,
                                  inst  |-> msg.inst,
                                  ballot|-> msg.ballot,
                                  consistency |-> msg.consistency,
                                  commit_order |-> msg.commit_order]}
        /\ UNCHANGED << proposed, crtInst, executed, leaderOfInst,
                        committed, ballots, preparing >>   
         ELSE
           /\ (\A rec \in oldRec: (rec.bal = msg.ballot \/ 
                                rec.bal[1] < msg.ballot[1]))
           /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                            {[inst   |-> msg.inst,
                              status |-> "accepted",
                              state  |-> "waiting", 
                              bal    |-> msg.ballot,
                              vbal    |-> msg.ballot,
                              cmd    |-> msg.cmd,
                              deps   |-> msg.deps,
                              seq    |-> msg.seq,
                              consistency |-> msg.consistency, 
                              ctxid |-> msg.ctxid,
                              execution_order |-> 0,
                              execution_order_list |-> {},
                              commit_order |-> msg.commit_order ]}]
           /\ LET newcmdstate == checkWaiting(waitingInst, replica) IN   
             /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                            {[inst   |-> msg.inst,
                              status |-> "accepted",
                              state  |-> "done", 
                              bal    |-> msg.ballot,
                              vbal    |-> msg.ballot,
                              cmd    |-> msg.cmd,
                              deps   |-> msg.deps,
                              seq    |-> msg.seq,
                              consistency |-> msg.consistency, 
                              ctxid |-> msg.ctxid,
                              execution_order |-> 0,
                              execution_order_list |-> {} ]}]               
               /\ sentMsg' = (sentMsg \ {msg}) \cup
                                    {[type  |-> "accept-reply",
                                      src   |-> replica,
                                      dst   |-> msg.src,
                                      inst  |-> msg.inst,
                                      ballot|-> msg.ballot,
                                      consistency |-> msg.consistency,
                                      commit_order |-> msg.commit_order]}
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
                                /\ msg.ballot = record.bal}
            receivedClk == Max({msg.clk : msg \in replies}) IN
            /\ (\A replica \in (Q \ {cleader}): \E msg \in replies: 
                                                        msg.src = replica)
            /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {record}) \cup 
                                    {[inst   |-> i,
                                      status |-> "strongly-committed",
                                      state  |-> "done",
                                      bal    |-> record.bal,
                                      vbal    |-> record.bal,
                                      cmd    |-> record.cmd,
                                      deps   |-> record.deps,
                                      seq    |-> record.seq,
                                      consistency |-> record.consistency,
                                      ctxid |-> record.ctxid,
                                      execution_order |-> 0,
                                      execution_order_list |-> {},
                                      commit_order |-> record.commit_order ]}]
            /\ sentMsg' = (sentMsg \ replies) \cup
                        {[type  |-> "commit",
                        inst    |-> i,
                        ballot  |-> record.bal,
                        cmd     |-> record.cmd,
                        deps    |-> record.deps,
                        seq     |-> record.seq,
                        consistency |-> record.consistency,
                        ctxid |-> record.ctxid,
                        commit_order |-> record.commit_order]}
            /\ committed' = [committed EXCEPT ![i] = @ \cup 
                               {<<record.cmd, record.deps, record.seq>>}]
            /\ leaderOfInst' = [leaderOfInst EXCEPT ![cleader] = @ \ {i}]
            /\ UNCHANGED << proposed, executed, crtInst, ballots, preparing >>                                            
                         
                         
                         
                               

CommitCausal(replica, cmsg) ==
    LET oldRec == {rec \in cmdLog[replica] : rec.inst = cmsg.inst}
            waitingInst == waitedDeps(cmsg.deps, replica) IN
            IF Cardinality(waitingInst) = 0 THEN
                /\ \A rec \in oldRec : (rec.status \notin {"causally-committed", "executed", "discarded"} /\ 
                                        rec.bal[1] <= cmsg.ballot[1])
                /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup 
                                            {[inst     |-> cmsg.inst,
                                              status   |-> "causally-committed",
                                              state    |-> "done",
                                              bal      |-> cmsg.ballot,
                                              vbal      |-> cmsg.ballot,
                                              cmd      |-> cmsg.cmd,
                                              deps     |-> cmsg.deps,
                                              seq      |-> cmsg.seq,
                                              consistency |-> cmsg.consistency,
                                              ctxid |-> cmsg.ctxid,
                                              execution_order |-> 0,
                                              execution_order_list |-> {},
                                              commit_order |-> cmsg.commit_order  ]}]
                /\ committed' = [committed EXCEPT ![cmsg.inst] = @ \cup 
                                       {<<cmsg.cmd, cmsg.deps, cmsg.seq>>}]
                /\ UNCHANGED << proposed, executed, crtInst, leaderOfInst,
                                sentMsg, ballots, preparing>>      
            ELSE
                /\ \A rec \in oldRec : (rec.status \notin {"causally-committed", "executed", "discarded"} /\ 
                                        rec.bal[1] <= cmsg.ballot[1])
                /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup 
                                            {[inst     |-> cmsg.inst,
                                              status   |-> "causally-committed",
                                              state    |-> "waiting",
                                              bal      |-> cmsg.ballot,
                                              vbal      |-> cmsg.ballot,
                                              cmd      |-> cmsg.cmd,
                                              deps     |-> cmsg.deps,
                                              seq      |-> cmsg.seq,
                                              consistency |-> cmsg.consistency,
                                              ctxid |-> cmsg.ctxid,
                                              execution_order |-> 0,
                                              execution_order_list |-> {},
                                              commit_order |-> cmsg.commit_order  ]}]
                /\ LET newcmdstate == checkWaiting(waitingInst, replica) IN
                    /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup 
                                                {[inst     |-> cmsg.inst,
                                                  status   |-> "causally-committed",
                                                  state    |-> "done",
                                                  bal      |-> cmsg.ballot,
                                                  vbal      |-> cmsg.ballot,
                                                  cmd      |-> cmsg.cmd,
                                                  deps     |-> cmsg.deps,
                                                  seq      |-> cmsg.seq,
                                                  consistency |-> cmsg.consistency,
                                                  ctxid |-> cmsg.ctxid,
                                                  execution_order |-> 0,
                                                  execution_order_list |-> {},
                                                  commit_order |-> cmsg.commit_order  ]}]
                    /\ committed' = [committed EXCEPT ![cmsg.inst] = @ \cup 
                                           {<<cmsg.cmd, cmsg.deps, cmsg.seq>>}]
                    /\ UNCHANGED << proposed, executed, crtInst, leaderOfInst,
                                    sentMsg, ballots, preparing>> 
                            
                            
                            
                                    
CommitStrong(replica, cmsg) ==
    LET oldRec == {rec \in cmdLog[replica] : rec.inst = cmsg.inst}
            waitingInst == waitedDeps(cmsg.deps, replica) IN
            IF Cardinality(waitingInst) = 0 THEN
                /\ \A rec \in oldRec : (rec.status \notin {"strongly-committed", "executed", "discarded"} /\ 
                                        rec.bal[1] <= cmsg.ballot[1])
                /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup 
                                            {[inst     |-> cmsg.inst,
                                              status   |-> "strongly-committed",
                                              state    |-> "done",
                                              bal      |-> cmsg.ballot,
                                              vbal      |-> cmsg.ballot,
                                              cmd      |-> cmsg.cmd,
                                              deps     |-> cmsg.deps,
                                              seq      |-> cmsg.seq,
                                              consistency |-> cmsg.consistency,
                                              ctxid |-> cmsg.ctxid,
                                              execution_order |-> 0,
                                              execution_order_list |-> {},
                                              commit_order |-> cmsg.commit_order  ]}]
                /\ committed' = [committed EXCEPT ![cmsg.inst] = @ \cup 
                                       {<<cmsg.cmd, cmsg.deps, cmsg.seq>>}]
                /\ UNCHANGED << proposed, executed, crtInst, leaderOfInst,
                                sentMsg, ballots, preparing>>      
            ELSE
                /\ \A rec \in oldRec : (rec.status \notin {"strongly-committed", "executed", "discarded"} /\ 
                                        rec.bal[1] <= cmsg.ballot[1])
                /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup 
                                            {[inst     |-> cmsg.inst,
                                              status   |-> "strongly-committed",
                                              state    |-> "waiting",
                                              bal      |-> cmsg.ballot,
                                              vbal      |-> cmsg.ballot,
                                              cmd      |-> cmsg.cmd,
                                              deps     |-> cmsg.deps,
                                              seq      |-> cmsg.seq,
                                              consistency |-> cmsg.consistency,
                                              ctxid |-> cmsg.ctxid,
                                              execution_order |-> 0 ,
                                              execution_order_list |-> {},
                                              commit_order |-> cmsg.commit_order ]}]
                /\ LET newcmdstate == checkWaiting(waitingInst, replica) IN
                    /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup 
                                                {[inst     |-> cmsg.inst,
                                                  status   |-> "strongly-committed",
                                                  state    |-> "done",
                                                  bal      |-> cmsg.ballot,
                                                  vbal      |-> cmsg.ballot,
                                                  cmd      |-> cmsg.cmd,
                                                  deps     |-> cmsg.deps,
                                                  seq      |-> cmsg.seq,
                                                  consistency |-> cmsg.consistency,
                                                  ctxid |-> cmsg.ctxid,
                                                  execution_order |-> 0,
                                                  execution_order_list |-> {},
                                                  commit_order |-> cmsg.commit_order]}]
                    /\ committed' = [committed EXCEPT ![cmsg.inst] = @ \cup 
                                           {<<cmsg.cmd, cmsg.deps, cmsg.seq>>}]
                    /\ UNCHANGED << proposed, executed, crtInst, leaderOfInst,
                                    sentMsg, ballots, preparing>>                   
 
 
 
                            
Commit(replica, cmsg) ==
    IF cmsg.consistency = "causal" THEN
        CommitCausal(replica, cmsg)
    ELSE
        CommitStrong(replica, cmsg)
        

(***************************************************************************)
(* Recovery actions                                                        *)
(***************************************************************************)

SendPrepare(replica, i, Q) ==
    /\ i \notin leaderOfInst[replica]
    /\ ballots <= MaxBallot
    /\ ~(\E rec \in cmdLog[replica] :
                        /\ rec.inst = i
                        /\ rec.status \in {"causally-committed", "strongly-committed", "executed", "discarded"})
    /\ ballots' = ballots + 1
    /\ sentMsg' = sentMsg \cup
                    [type   : {"prepare"},
                     src    : {replica},
                     dst    : Q,
                     inst   : {i},
                     ballot : {<< ballots, replica >>}]
    /\ preparing' = [preparing EXCEPT ![replica] = @ \cup {i}]
    /\ UNCHANGED << cmdLog, proposed, executed, crtInst,
                    leaderOfInst, committed>>
                    
 ReplyPrepare(replica) ==
    \E msg \in sentMsg : 
        /\ msg.type = "prepare"
        /\ msg.dst = replica
        /\ \/ \E rec \in cmdLog[replica] : 
                /\ rec.inst = msg.inst
                /\ msg.ballot[1] > rec.bal[1]
                /\ LET waitingInst == waitedDeps(rec.deps, replica) IN
                   IF Cardinality(waitingInst) = 0 THEN
                   
                      /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                                    {[inst  |-> rec.inst,
                                      status|-> rec.status,
                                      state |-> rec.state, 
                                      bal   |-> msg.ballot,
                                      vbal   |-> rec.bal,
                                      cmd   |-> rec.cmd,
                                      deps  |-> rec.deps,
                                      seq   |-> rec.seq,
                                      consistency |-> rec.consistency,
                                      ctxid |-> rec.ctxid,
                                      execution_order |-> 0,
                                      execution_order_list |-> {},
                                      commit_order |-> rec.commit_order ]}]
                                      
                        /\ sentMsg' = (sentMsg \ {msg}) \cup
                                    {[type  |-> "prepare-reply",
                                      src   |-> replica,
                                      dst   |-> msg.src,
                                      inst  |-> rec.inst,
                                      ballot|-> msg.ballot,
                                      prev_ballot|-> rec.bal,
                                      status|-> rec.status,
                                      cmd   |-> rec.cmd,
                                      deps  |-> rec.deps,
                                      seq   |-> rec.seq,
                                      consistency |-> rec.consistency,
                                      ctxid |-> rec.ctxid,
                                      commit_order |-> rec.commit_order
                                     ]}
                      
                         /\ IF rec.inst \in leaderOfInst[replica] THEN
                                /\ leaderOfInst' = [leaderOfInst EXCEPT ![replica] = 
                                                                        @ \ {rec.inst}]
                                /\ UNCHANGED << proposed, executed, committed,
                                                crtInst, ballots, preparing>>
                            ELSE UNCHANGED << proposed, executed, committed, crtInst,
                                              ballots, preparing, leaderOfInst>>
                    ELSE
                       /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                                    {[inst  |-> rec.inst,
                                      status|-> rec.status,
                                      state |-> "waiting", 
                                      bal   |-> msg.ballot,
                                      vbal   |-> rec.bal,
                                      cmd   |-> rec.cmd,
                                      deps  |-> rec.deps,
                                      seq   |-> rec.seq,
                                      consistency |-> rec.consistency,
                                      ctxid |-> rec.ctxid,
                                      execution_order |-> 0,
                                      execution_order_list |-> {},
                                      commit_order |-> rec.commit_order ]}]
                      /\ LET newcmdstate == checkWaiting(waitingInst, replica) IN
                      
                         /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                                        {[inst  |-> rec.inst,
                                          status|-> rec.status,
                                          state |-> rec.state, 
                                          bal   |-> msg.ballot,
                                          vbal   |-> rec.bal,
                                          cmd   |-> rec.cmd,
                                          deps  |-> rec.deps,
                                          seq   |-> rec.seq,
                                          consistency |-> rec.consistency,
                                          ctxid |-> rec.ctxid,
                                          execution_order |-> 0,
                                          execution_order_list |-> {},
                                          commit_order |-> rec.commit_order ]}]
                                          
                                          
                          /\ sentMsg' = (sentMsg \ {msg}) \cup
                                        {[type  |-> "prepare-reply",
                                          src   |-> replica,
                                          dst   |-> msg.src,
                                          inst  |-> rec.inst,
                                          ballot|-> msg.ballot,
                                          prev_ballot|-> rec.bal,
                                          status|-> rec.status,
                                          cmd   |-> rec.cmd,
                                          deps  |-> rec.deps,
                                          seq   |-> rec.seq,
                                          consistency |-> rec.consistency,
                                          ctxid |-> rec.ctxid,
                                          commit_order |-> rec.commit_order]}
                          
                             /\ IF rec.inst \in leaderOfInst[replica] THEN
                                    /\ leaderOfInst' = [leaderOfInst EXCEPT ![replica] = 
                                                                            @ \ {rec.inst}]
                                    /\ UNCHANGED << proposed, executed, committed,
                                                    crtInst, ballots, preparing>>
                                ELSE UNCHANGED << proposed, executed, committed, crtInst,
                                                  ballots, preparing, leaderOfInst>>
                  
                        
           \/ /\ ~(\E rec \in cmdLog[replica] : rec.inst = msg.inst)
           
           
           /\ cmdLog' = [cmdLog EXCEPT ![replica] = @ \cup
                            {[inst  |-> msg.inst,
                              status|-> "not-seen",
                              state |-> "not-seen",
                              bal   |-> msg.ballot,
                              vbal   |-> << 0, replica >>,
                              cmd   |-> [op |-> [key |-> "", type |-> ""]],
                              deps  |-> {},
                              seq   |-> 0,
                              consistency|-> "not-seen",
                              ctxid |-> 0,
                              execution_order |-> 0,
                              execution_order_list |-> {},
                              commit_order |-> 0 ]}]
                              
                              
              /\ sentMsg' = (sentMsg \ {msg}) \cup
                            {[type  |-> "prepare-reply",
                              src   |-> replica,
                              dst   |-> msg.src,
                              inst  |-> msg.inst,
                              ballot|-> msg.ballot,
                              prev_ballot|-> << 0, replica >>,
                              status|-> "not-seen",
                              cmd   |-> [op |-> [key |-> "", type |-> ""]],
                              deps  |-> {},
                              seq   |-> 0,
                              consistency |-> "not-seen",
                              ctxid |-> 0, (* ctxid 0 means unknown context id *)
                              commit_order |-> 0]} (* commit_order 0 means not committed *)
              
              /\ UNCHANGED << proposed, executed, committed, crtInst, ballots,
                              leaderOfInst, preparing>>     
  
                              
PrepareFinalize(replica, i, Q) ==
    /\ i \in preparing[replica]
    /\ \E rec \in cmdLog[replica] :
       /\ rec.inst = i
       /\ rec.status \notin {"causally-committed", "strongly-committed", "executed", "discarded"}
       /\ LET replies == {msg \in sentMsg : 
                        /\ msg.inst = i
                        /\ msg.type = "prepare-reply"
                        /\ msg.dst = replica
                        /\ msg.ballot = rec.bal} IN
            /\ (\A rep \in Q : \E msg \in replies : msg.src = rep)
            /\  \/ \E com \in replies :
                        /\ (com.status \in {"causally-committed", "strongly-committed", "executed", "discarded"})
                        /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                        (*/\ sentMsg' = sentMsg \ replies*)
                        /\ sentMsg' = (sentMsg \ replies) \cup
                                [type  : {"commit"},
                                inst    : {i},
                                ballot  : {rec.bal},
                                cmd     : {com.cmd},
                                deps    : {com.deps},
                                seq     : {com.seq},
                                consistency : {com.consistency},
                                ctxid : {com.ctxid},
                                commit_order : {com.commit_order}]
                        /\ UNCHANGED << cmdLog, proposed, executed, crtInst, leaderOfInst,
                                        committed, ballots>>
                        
                \/ /\ ~(\E msg \in replies : msg.status \in {"causally-committed", "strongly-committed", "executed", "discarded"})
                   /\ \E acc \in replies :
                        /\ acc.status = "accepted"
                        /\ (\A msg \in (replies \ {acc}) : 
                            (msg.prev_ballot[1] <= acc.prev_ballot[1] \/ 
                             msg.status # "accepted"))
                        /\ LET waitingInst == waitedDeps(acc.deps, replica) IN
                         IF Cardinality(waitingInst) = 0 THEN
                            /\ sentMsg' = (sentMsg \ replies) \cup
                                     [type  : {"accept"},
                                      src   : {replica},
                                      dst   : Q \ {replica},
                                      inst  : {i},
                                      ballot: {rec.bal},
                                      cmd   : {acc.cmd},
                                      deps  : {acc.deps},
                                      seq   : {acc.seq},
                                      consistency : {acc.consistency},
                                      ctxid : {acc.ctxid},
                                      commit_order: {0}]
                            /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                                    {[inst  |-> i,
                                      status|-> "accepted",
                                      state |-> "done",
                                      bal   |-> rec.bal,
                                      vbal   |-> rec.bal,
                                      cmd   |-> acc.cmd,
                                      deps  |-> acc.deps,
                                      seq   |-> acc.seq,
                                      consistency |-> acc.consistency,
                                      ctxid |-> acc.ctxid,
                                      execution_order |-> 0,
                                      execution_order_list |-> {},
                                      commit_order |-> 0]}]
                             /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                             /\ leaderOfInst' = [leaderOfInst EXCEPT ![replica] = @ \cup {i}]
                             /\ UNCHANGED << proposed, executed, crtInst, committed, ballots>>
                         ELSE
                             /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                                    {[inst  |-> i,
                                      status|-> "accepted",
                                      state |-> "waiting",
                                      bal   |-> rec.bal,
                                      vbal   |-> rec.bal,
                                      cmd   |-> acc.cmd,
                                      deps  |-> acc.deps,
                                      seq   |-> acc.seq,
                                      consistency |-> acc.consistency,
                                      ctxid |-> acc.ctxid,
                                      execution_order |-> 0,
                                      execution_order_list |-> {},
                                      commit_order |-> 0]}]
                            /\ LET newcmdstate == checkWaiting(waitingInst, replica) IN
                                /\ sentMsg' = (sentMsg \ replies) \cup
                                         [type  : {"accept"},
                                          src   : {replica},
                                          dst   : Q \ {replica},
                                          inst  : {i},
                                          ballot: {rec.bal},
                                          cmd   : {acc.cmd},
                                          deps  : {acc.deps},
                                          seq   : {acc.seq},
                                          consistency : {acc.consistency},
                                          ctxid : {acc.ctxid},
                                          commit_order: {0}]
                                /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                                        {[inst  |-> i,
                                          status|-> "accepted",
                                          state |-> "done",
                                          bal   |-> rec.bal,
                                          vbal   |-> rec.bal,
                                          cmd   |-> acc.cmd,
                                          deps  |-> acc.deps,
                                          seq   |-> acc.seq,
                                          consistency |-> acc.consistency,
                                          ctxid |-> acc.ctxid,
                                          execution_order |-> 0,
                                          execution_order_list |-> {},
                                          commit_order |-> 0 ]}]
                                 /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                                 /\ leaderOfInst' = [leaderOfInst EXCEPT ![replica] = @ \cup {i}]
                                 /\ UNCHANGED << proposed, executed, crtInst, committed, ballots>>
                            
                \/ /\ ~(\E msg \in replies : 
                        msg.status \in {"accepted", "causally-committed", "strongly-committed", "executed", "discarded"})
                   /\ LET preaccepts == {msg \in replies : msg.status = "pre-accepted"} IN
                       (\/  /\ \A p1, p2 \in preaccepts :
                                    p1.cmd = p2.cmd /\ p1.deps = p2.deps /\ p1.seq = p2.seq
                            /\ ~(\E pl \in preaccepts : pl.src = i[1])
                            /\ Cardinality(preaccepts) >= Cardinality(Q) - 1
                            /\ LET pac == CHOOSE pac \in preaccepts : TRUE IN
                                /\ LET waitingInst == waitedDeps(pac.deps, replica) IN
                                   IF Cardinality(waitingInst) = 0 THEN
                                        /\ sentMsg' = (sentMsg \ replies) \cup
                                                 [type  : {"accept"},
                                                  src   : {replica},
                                                  dst   : Q \ {replica},
                                                  inst  : {i},
                                                  ballot: {rec.bal},
                                                  cmd   : {pac.cmd},
                                                  deps  : {pac.deps},
                                                  seq   : {pac.seq},
                                                  consistency : {pac.consistency},
                                                  ctxid : {pac.ctxid},
                                                  commit_order: {0}]
                                        /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                                                {[inst  |-> i,
                                                  status|-> "accepted",
                                                  state |-> "done",
                                                  bal   |-> rec.bal,
                                                  vbal   |-> rec.bal,
                                                  cmd   |-> pac.cmd,
                                                  deps  |-> pac.deps,
                                                  seq   |-> pac.seq,
                                                  consistency |-> pac.consistency,
                                                  ctxid |-> pac.ctxid,
                                                  execution_order |-> 0,
                                                  execution_order_list |-> {},
                                                  commit_order |-> 0 ]}]
                                         /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                                         /\ leaderOfInst' = [leaderOfInst EXCEPT ![replica] = @ \cup {i}]
                                         /\ UNCHANGED << proposed, executed, crtInst, committed, ballots >>
                                    ELSE
                                        /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                                                {[inst  |-> i,
                                                  status|-> "accepted",
                                                  state |-> "waiting",
                                                  bal   |-> rec.bal,
                                                  vbal   |-> rec.bal,
                                                  cmd   |-> pac.cmd,
                                                  deps  |-> pac.deps,
                                                  seq   |-> pac.seq,
                                                  consistency |-> pac.consistency,
                                                  ctxid |-> pac.ctxid,
                                                  execution_order |-> 0,
                                                  execution_order_list |-> {},
                                                  commit_order |-> 0 ]}]
                                       /\ LET newcmdstate == checkWaiting(waitingInst, replica) IN
                                           /\ sentMsg' = (sentMsg \ replies) \cup
                                                     [type  : {"accept"},
                                                      src   : {replica},
                                                      dst   : Q \ {replica},
                                                      inst  : {i},
                                                      ballot: {rec.bal},
                                                      cmd   : {pac.cmd},
                                                      deps  : {pac.deps},
                                                      seq   : {pac.seq},
                                                      consistency : {pac.consistency},
                                                      ctxid : {pac.ctxid},
                                                      commit_order: {0}]
                                            /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ {rec}) \cup
                                                    {[inst  |-> i,
                                                      status|-> "accepted",
                                                      state |-> "done",
                                                      bal   |-> rec.bal,
                                                      vbal   |-> rec.bal,
                                                      cmd   |-> pac.cmd,
                                                      deps  |-> pac.deps,
                                                      seq   |-> pac.seq,
                                                      consistency |-> pac.consistency,
                                                      ctxid |-> pac.ctxid,
                                                      execution_order |-> 0,
                                                      execution_order_list |-> {},
                                                      commit_order |-> 0 ]}]
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
                                          ballot   : {rec.bal},
                                          status: {pac.status},
                                          cmd   : {pac.cmd},
                                          deps  : {pac.deps},
                                          seq   : {pac.seq},
                                          consistency : {pac.consistency},
                                          ctxid : {pac.ctxid}]
                                /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                                /\ leaderOfInst' = [leaderOfInst EXCEPT ![replica] = @ \cup {i}]
                                /\ UNCHANGED << cmdLog, proposed, executed,
                                                crtInst, committed, ballots>>
                        \/  /\ \/ \E p1, p2 \in preaccepts : p1.cmd # p2.cmd \/ 
                                                             p1.deps # p2.deps \/
                                                             p1.seq # p2.seq
                               \/ \E pl \in preaccepts : pl.src = i[1]
                               \/ Cardinality(preaccepts) < Cardinality(Q) \div 2
                            /\ preaccepts # {}
                            /\ LET pac == CHOOSE pac \in preaccepts : TRUE IN
                                /\ StartPhase1(pac.cmd, replica, Q, i, rec.bal, replies,pac.consistency,pac.ctxid)
                                /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                                /\ UNCHANGED << proposed, executed, crtInst, committed, ballots>>)
                \/  /\ \A msg \in replies : msg.status = "not-seen"
                    /\ StartPhase1([op |-> [key |-> "", type |-> ""]], replica, Q, i, rec.bal, replies, "strong", 0) (* no dependency will be build on it *)
                    /\ preparing' = [preparing EXCEPT ![replica] = @ \ {i}]
                    /\ UNCHANGED << proposed, executed, crtInst, committed, ballots >>   
                    
      
ReplyTryPreaccept(replica) ==
    \E tpa \in sentMsg :
        /\ tpa.type = "try-pre-accept" 
        /\ tpa.dst = replica
        /\ LET oldRec == {rec \in cmdLog[replica] : rec.inst = tpa.inst}  IN
            /\ \A rec \in oldRec : rec.bal[1] <= tpa.ballot[1] /\ 
                                   rec.status \notin {"accepted",  "causally-committed", "strongly-committed", "executed", "discarded"}
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
                                      status|-> rec.status,
                                      consistency |-> tpa.consistency,
                                      ctxid |-> tpa.ctxid]})
                        /\ UNCHANGED << cmdLog, proposed, executed, committed, crtInst,
                                        ballots, leaderOfInst, preparing >>
               \/ /\ (\A rec \in cmdLog[replica] \ oldRec: 
                            tpa.inst \in rec.deps \/ (rec.inst \in tpa.deps /\
                                                      rec.seq < tpa.seq))
                 /\ LET waitingInst == waitedDeps(tpa.deps, replica) IN
                   IF Cardinality(waitingInst) = 0 THEN
                      /\ sentMsg' = (sentMsg \ {tpa}) \cup
                                        {[type  |-> "try-pre-accept-reply",
                                          src   |-> replica,
                                          dst   |-> tpa.src,
                                          inst  |-> tpa.inst,
                                          ballot|-> tpa.ballot,
                                          status|-> "OK",
                                          consistency |-> tpa.consistency,
                                          ctxid |-> tpa.ctxid]}
                      /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                                        {[inst  |-> tpa.inst,
                                          status|-> "pre-accepted",
                                          state |-> "done",
                                          bal   |-> tpa.ballot,
                                          vbal   |-> tpa.ballot,
                                          cmd   |-> tpa.cmd,
                                          deps  |-> tpa.deps,
                                          seq   |-> tpa.seq,
                                          consistency |-> tpa.consistency,
                                          ctxid |-> tpa.ctxid,
                                          execution_order |-> 0,
                                          execution_order_list |-> {},
                                          commit_order |-> 0 ]}]
                      /\ UNCHANGED << proposed, executed, committed, crtInst, ballots,
                                      leaderOfInst, preparing >>
                  ELSE 
                      /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                                            {[inst  |-> tpa.inst,
                                              status|-> "pre-accepted",
                                              state |-> "waiting",
                                              bal   |-> tpa.ballot,
                                              vbal   |-> tpa.ballot,
                                              cmd   |-> tpa.cmd,
                                              deps  |-> tpa.deps,
                                              seq   |-> tpa.seq,
                                              consistency |-> tpa.consistency,
                                              ctxid |-> tpa.ctxid,
                                              execution_order |-> 0,
                                              execution_order_list |-> {},
                                              commit_order |-> 0 ]}]
                      /\ LET newcmdstate == checkWaiting(waitingInst, replica) IN
                          /\ sentMsg' = (sentMsg \ {tpa}) \cup
                                            {[type  |-> "try-pre-accept-reply",
                                              src   |-> replica,
                                              dst   |-> tpa.src,
                                              inst  |-> tpa.inst,
                                              ballot|-> tpa.ballot,
                                              status|-> "OK",
                                              consistency |-> tpa.consistency,
                                              ctxid |-> tpa.ctxid]}
                          /\ cmdLog' = [cmdLog EXCEPT ![replica] = (@ \ oldRec) \cup
                                            {[inst  |-> tpa.inst,
                                              status|-> "pre-accepted",
                                              state |-> "done",
                                              bal   |-> tpa.ballot,
                                              vbal   |-> tpa.ballot,
                                              cmd   |-> tpa.cmd,
                                              deps  |-> tpa.deps,
                                              seq   |-> tpa.seq,
                                              consistency |-> tpa.consistency,
                                              ctxid |-> tpa.ctxid,
                                              execution_order |-> 0,
                                              execution_order_list |-> {},
                                              commit_order |-> 0 ]}]
                          /\ UNCHANGED << proposed, executed, committed, crtInst, ballots,
                                          leaderOfInst, preparing >>
                          
             
FinalizeTryPreAccept(cleader, i, Q) ==
    \E rec \in cmdLog[cleader]:
        /\ rec.inst = i
        /\ LET tprs == {msg \in sentMsg : msg.type = "try-pre-accept-reply" /\
                            msg.dst = cleader /\ msg.inst = i /\
                            msg.ballot = rec.bal}  IN
            /\ \A r \in Q: \E tpr \in tprs : tpr.src = r
            /\ \/ /\ \A tpr \in tprs: tpr.status = "OK"
                  /\ LET waitingInst == waitedDeps(rec.deps, cleader) IN
                     IF Cardinality(waitingInst) = 0 THEN
                          /\ sentMsg' = (sentMsg \ tprs) \cup
                                     [type  : {"accept"},
                                      src   : {cleader},
                                      dst   : Q \ {cleader},
                                      inst  : {i},
                                      ballot: {rec.bal},
                                      cmd   : {rec.cmd},
                                      deps  : {rec.deps},
                                      seq   : {rec.seq},
                                      consistency : {rec.consistency},
                                      ctxid : {rec.ctxid},
                                      commit_order: {0}]           
                          /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {rec}) \cup
                                    {[inst  |-> i,
                                      status|-> "accepted",
                                      bal   |-> rec.bal,
                                      vbal   |-> rec.bal,
                                      cmd   |-> rec.cmd,
                                      deps  |-> rec.deps,
                                      seq   |-> rec.seq,
                                      state |-> rec.state,
                                      consistency |-> rec.consistency,
                                      ctxid |-> rec.ctxid,
                                      execution_order |-> 0,
                                      execution_order_list |-> {},
                                      commit_order |-> 0 ]}]
                          /\ UNCHANGED << proposed, executed, committed, crtInst, ballots,
                                          leaderOfInst, preparing >>
                      ELSE 
                         /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {rec}) \cup
                                    {[inst  |-> i,
                                      status|-> "accepted",
                                      bal   |-> rec.bal,
                                      vbal   |-> rec.bal,
                                      cmd   |-> rec.cmd,
                                      deps  |-> rec.deps,
                                      seq   |-> rec.seq,
                                      state |-> "waiting",
                                      consistency |-> rec.consistency,
                                      ctxid |-> rec.ctxid,
                                      execution_order |-> 0,
                                      execution_order_list |-> {},
                                      commit_order |-> 0 ]}]
                        /\ LET newcmdstate == checkWaiting(waitingInst, cleader) IN
                            /\ sentMsg' = (sentMsg \ tprs) \cup
                                         [type  : {"accept"},
                                          src   : {cleader},
                                          dst   : Q \ {cleader},
                                          inst  : {i},
                                          ballot: {rec.ballot},
                                          cmd   : {rec.cmd},
                                          deps  : {rec.deps},
                                          seq   : {rec.seq},
                                          consistency : {rec.consistency},
                                          ctxid : {rec.ctxid},
                                          commit_order: {0} ]           
                              /\ cmdLog' = [cmdLog EXCEPT ![cleader] = (@ \ {rec}) \cup
                                        {[inst  |-> i,
                                          status|-> "accepted",
                                          bal   |-> rec.bal,
                                          vbal   |-> rec.bal,
                                          cmd   |-> rec.cmd,
                                          deps  |-> rec.deps,
                                          seq   |-> rec.seq,
                                          state |-> rec.state,
                                          consistency |-> rec.consistency,
                                          ctxid |-> rec.ctxid,
                                          execution_order |-> 0,
                                          execution_order_list |-> {},
                                          commit_order |-> 0 ]}]
                              /\ UNCHANGED << proposed, executed, committed, crtInst, ballots,
                                              leaderOfInst, preparing >>
               \/ /\ \E tpr \in tprs: tpr.status \in {"accepted", "causally-committed", "strongly-committed", "executed", "discarded"}
                  /\ StartPhase1(rec.cmd, cleader, Q, i, rec.bal, tprs, rec.consistency, rec.ctxid)
                  /\ UNCHANGED << proposed, executed, committed, crtInst, ballots,
                                  leaderOfInst, preparing >>
               \/ /\ \E tpr \in tprs: tpr.status = "pre-accepted"
                  /\ \A tpr \in tprs: tpr.status \in {"OK", "pre-accepted"}
                  /\ sentMsg' = sentMsg \ tprs
                  /\ leaderOfInst' = [leaderOfInst EXCEPT ![cleader] = @ \ {i}]
                  /\ UNCHANGED << cmdLog, proposed, executed, committed, crtInst,
                                  ballots, preparing >> 
                                  
(***************************************************************************)
(* Command Execution Handler Functions                                     *)
(***************************************************************************)

BoundedSeq(S, n) == UNION {[1..i -> S] : i \in 0..n}  (* this is generating all possible paths among 
all the instances of the system*)
BSeq(S) == BoundedSeq(S, Cardinality(S)+1)


NewDepPathSet(replica,G) ==
    {
    p \in BSeq(G) : /\ p /= <<>>
                          /\ \forall i \in 1 .. (Len(p)-1) : (*this is checking wehther each pair of vertex (of an edge)
                          in the path is also a part of the dependency graph.  Checking this by finding whether the 
                          first vertex of the edge is the instance itself and the second vertex of the edge is in the dependency
                          graph of the instance*)
                            \E rec \in cmdLog[replica]: 
                                /\ rec.inst = p[i]
                                /\ p[i+1] \in rec.deps
                             
                             }

AreConnectedIn(replica, m,n,G) == 
    \E p \in NewDepPathSet(replica,G) : (p[1]=m) /\ (p[Len(p)] = n)



IsStronglyConnectedSCC(replica,i,scc,G) == 
    \A m,n \in scc: m#n => AreConnectedIn(replica,m,n,G)
        


FindAllInstances(replica, i) ==   (* finding all the instances of the command log *)
    {rec.inst: rec \in cmdLog[replica] 
        }
        
AreStronglyConnectedIn(replica, m, n, G) ==
    /\ AreConnectedIn(replica, m, n, G)
    /\ AreConnectedIn(replica, n, m, G)
        
        
SccTidSet(replica, i, dep, tid) == 
{
    tid_set \in SUBSET UNION {dep,{<<replica,i>>}}:
    /\ tid \in tid_set
    /\ IsStronglyConnectedSCC(replica, i , tid_set, UNION {dep,{<<replica,i>>}})
    /\ \forall m \in dep:
         AreStronglyConnectedIn(replica, m, tid, dep) => m \in tid_set
         }
         
FindSpecificInstance(replica, inst) == (*find a specific instance*)
    {rec \in cmdLog[replica] : rec.inst = inst} 
         
FindDeps(replica, i) == (*find the dependency of a specific instance*)
    { rec.deps: rec \in FindSpecificInstance("a",<<replica,i>>)}   (*--replace replica value--*)
    
MaxSequence(allSequences) == 
    CHOOSE seq \in allSequences : \A otherSeq \in allSequences : Cardinality(seq) >= Cardinality(otherSeq)

MinSetCover(allSequences) == 
    LET
        RECURSIVE minCover(_, _)
        minCover(SeqSet, Cover) ==
            IF SeqSet = {}
            THEN Cover
            ELSE
                LET seq == MaxSequence(SeqSet) IN
                    IF (\E inst \in Cover: \E i \in inst : i \in seq) /\ (Cover # {}) THEN
                        minCover(SeqSet \ {seq}, Cover)
                    ELSE
                        minCover(SeqSet \ {seq}, Cover \cup {seq})
    IN
        minCover(allSequences, {})


   
FindSCC(replica, i) == 
{
   scc \in SUBSET FindAllInstances(replica, i): 
    /\ IsStronglyConnectedSCC(replica, i, scc, Instances)
    /\ LET dep == FindDeps(replica, i) 
        dep2 == CHOOSE s \in dep  : TRUE IN
        /\ \E tid \in scc: scc \in SccTidSet(replica, i, dep2, tid)
}

FinalSCC(replica,i) ==
    MinSetCover(FindSCC(replica, i))



(*************Ordering Instances in SCC**************)

FindSeq(replica, inst) == (*find the sequence number of a specific instance*)
    {rec.seq: rec \in FindSpecificInstance(replica,inst)}
    
ChoosingSetElement(replica,i) == 
    CHOOSE inst \in FindSeq(replica,i): TRUE

MinSeq(allInstances) ==
    CHOOSE inst \in allInstances : \A otherInst \in allInstances : 
        ChoosingSetElement("a", <<inst[1],inst[2]>>) <= ChoosingSetElement("a",<<otherInst[1],otherInst[2]>>)(*--replace replica value--*)

OrderingInstancesFirstLevel(scc) ==  (*ordering based on sequence number (ascending)*)(* returns a set of sequences with ordered instance.*)
     LET
        RECURSIVE minCover(_, _, _)
        minCover(SeqSet, Cover, i) ==
            IF SeqSet = {}
            THEN Cover
            ELSE
                LET seq == MinSeq(SeqSet)
                seq1 == <<>>
                j == (i+1)
                seq2 == Append(seq1,j)
                seq3 == Append(seq2,seq)
               
                       IN
                        minCover(SeqSet \ {seq}, Cover \cup {seq3},j)
     IN
       minCover(scc, {}, 0)
    
  
SetToSeq(s) == 
    LET
        RECURSIVE settoseq(_, _)
        settoseq(set, newseq) ==
            IF set = {}
            THEN newseq
            ELSE
                LET seq == CHOOSE x \in set: TRUE
              
                IN  
                
                settoseq(set \ {seq}, Append(newseq,seq[2]))
    IN
        settoseq(s,<<>>)
        
        

        
MinInstancePosition(item, seq) == 
    LET 
        RECURSIVE mininstanceposition(_, _, _, _)
        mininstanceposition(min, s, pos, ptr) ==
            IF ptr = Len(s)+1 
            THEN pos
            ELSE
                IF min[1] = s[ptr][1] /\ min[2] >= s[ptr][2]
                THEN  
                    LET min1 == s[ptr] 
                    IN mininstanceposition(min1, s, ptr, ptr+1)
                ELSE 
                    mininstanceposition(min, s, pos, ptr+1)
                    
    IN 
    mininstanceposition(item, seq, 1, 1)
    
swapSeq(seq, pos, newValue) == [i \in DOMAIN seq |-> IF i = pos THEN newValue ELSE seq[i]]

 
Delete(seq, pos) == [i \in 1..Len(seq)-1 |-> IF i < pos THEN seq[i] ELSE seq[i + 1]]
   
SortSeq2(seq) ==
    LET
        RECURSIVE sortseq2(_, _)
        sortseq2(s, newS) == 
            IF s = <<>>
            THEN newS
            ELSE
                LET minitemPos == MinInstancePosition(s[1],s)
                item == s[minitemPos]
                swapedSeq == swapSeq(s, minitemPos, s[1])
                IN
                sortseq2(Delete(swapedSeq,1), Append(newS,item ))
    IN     
        sortseq2(seq, <<>>)         
           
OrderingInstancesSecondLevel(scc) ==
        LET 
            SortedInstances == SortSeq2(SetToSeq(scc))
        IN  { <<Index, SortedInstances[Index]>> : Index \in 1..Len(SortedInstances) }
                        


(***************************************************************************)
(* Command Execution Actions                                               *)
(***************************************************************************)
 
ExecuteCommand(replica, i) == 
        LET  rec == {r \in cmdLog[replica] : r.inst = i /\ r.status \in {"causally-committed","strongly-committed"}} IN 
            IF Cardinality(rec) = 0 THEN 
                /\UNCHANGED <<cmdLog, proposed, executed, sentMsg, crtInst, leaderOfInst,
                        committed, ballots, preparing>>
            ELSE  
            /\ LET scc_set == FinalSCC(replica,i) (*finding all scc *)IN
                /\ \A scc \in scc_set: LET firstlevelordering ==  OrderingInstancesFirstLevel(scc) IN (*ordering based on seq number *)
                 \A instant \in OrderingInstancesSecondLevel(firstlevelordering): (*ordering based on instance number to gurantee the session causality *)
                    \E rec2 \in cmdLog[instant[2][1]]:
                        /\rec2.inst=instant[2][2]
                        /\ LET max_execution_order_inst == FindMaxExecutionOrder(replica)
                              max_execution_order == max_execution_order_inst.execution_order (* Finding max execution order from the cmdLog *) IN
                                /\ IF rec2.cmd.op.type = "r" THEN  (*checking whether the operation is read or write*)
                                    /\cmdLog' = [cmdLog EXCEPT ![instant[2][1]] = (@ \ instant[2][2]) \cup
                                                    {[inst   |-> rec2.inst,
                                                      status |-> "executed",
                                                      state  |-> rec2.state,
                                                      ballot |-> rec2.ballot,
                                                      cmd    |-> rec2.cmd,
                                                      deps   |-> rec2.deps,
                                                      seq    |-> rec2.seq,
                                                      consistency |-> rec2.consistency,
                                                      ctxid |-> rec2.ctxid,
                                                      execution_order |-> (max_execution_order+1),
                                                      execution_order_list |-> instant,
                                                      commit_order |-> rec2.commit_order]}]
                                    /\UNCHANGED <<proposed, executed, sentMsg, crtInst, leaderOfInst,
                                            committed, ballots, preparing>>
                                   
                                   ELSE 
                                    LET 
                                        recs == {rec3 \in cmdLog[replica]: rec3.state = "executed" /\ rec3.cmd.op.key = rec2.cmd.op.key /\ rec3.cmd.op.type = rec2.cmd.op.key} (* finding the instance that has the same key as the instance that we are going to execute *)
                                        seq == {rec4.seq: rec4 \in recs} (* finding the seq number of the last write *) IN
                                            IF rec2.seq > seq THEN
                                                /\cmdLog' = [cmdLog EXCEPT ![instant[1]] = (@ \ instant[2]) \cup
                                                    {[inst   |-> rec2.inst,
                                                      status |-> "executed", (* latest write win *)
                                                      state  |-> rec2.state,
                                                      ballot |-> rec2.ballot,
                                                      cmd    |-> rec2.cmd,
                                                      deps   |-> rec2.deps,
                                                      seq    |-> rec2.seq,
                                                      consistency |-> rec2.consistency,
                                                      ctxid |-> rec2.ctxid,
                                                      execution_order |-> (max_execution_order+1),
                                                      execution_order_list |-> instant,
                                                      commit_order |-> rec2.commit_order  ]}]
                                               /\UNCHANGED <<proposed, executed, sentMsg, crtInst, leaderOfInst,
                                                 committed, ballots, preparing>>
                                            ELSE
                                                /\cmdLog' = [cmdLog EXCEPT ![instant[1]] = (@ \ instant[2]) \cup
                                                    {[inst   |-> rec2.inst,
                                                      status |-> "discarded",
                                                      state  |-> rec2.state,
                                                      ballot |-> rec2.ballot,
                                                      cmd    |-> rec2.cmd,
                                                      deps   |-> rec2.deps,
                                                      seq    |-> rec2.seq,
                                                      consistency |-> rec2.consistency,
                                                      ctxid |-> rec2.ctxid,
                                                      execution_order |-> (max_execution_order+1),
                                                      execution_order_list |-> instant,
                                                      commit_order |-> rec2.commit_order  ]}]
                                               /\UNCHANGED <<proposed, executed, sentMsg, crtInst, leaderOfInst,
                                                 committed, ballots, preparing>>    

(***************************************************************************)
(* Action groups                                                           *)
(***************************************************************************)        

CommandLeaderAction ==
    \/ (\E C \in (Commands \ proposed) :
            \E cl \in Consistency_level: 
                \E ctx \in Ctx_id:
                    \E cleader \in Replicas : Propose(C, cleader,cl,ctx))
    \/ (\E cleader \in Replicas : \E inst \in leaderOfInst[cleader] :
            \/ (\E Q \in FastQuorums(cleader) : Phase1Fast(cleader, inst, Q))
            \/ (\E Q \in SlowQuorums(cleader) : Phase1Slow(cleader, inst, Q))
            \/ (\E Q \in SlowQuorums(cleader) : Phase2Finalize(cleader, inst, Q))
            \/ (\E Q \in SlowQuorums(cleader) : FinalizeTryPreAccept(cleader, inst, Q)))
    \/ (\E replica \in Replicas: 
            \E inst \in cmdLog[replica]: ExecuteCommand(replica, inst))
    
    
  
            
   
            
ReplicaAction ==
    \E replica \in Replicas :
        (\/ Phase1Reply(replica)
         \/ \E cmsg \in sentMsg : (cmsg.type = "commit" /\ Commit(replica, cmsg))
         \/ Phase2Reply(replica)
         \/ \E i \in Instances : 
            /\ crtInst[i[1]] > i[2] 
            /\ \E Q \in SlowQuorums(replica) : SendPrepare(replica, i, Q)
         \/ ReplyPrepare(replica)
         \/ \E i \in preparing[replica] :
            \E Q \in SlowQuorums(replica) : PrepareFinalize(replica, i, Q)
         \/ ReplyTryPreaccept(replica)
         \/ \E inst \in cmdLog[replica]: ExecuteCommand(replica, inst)
         )


(***************************************************************************)
(* Next action                                                             *)
(***************************************************************************)

Next == 
    \/ CommandLeaderAction
    \/ ReplicaAction
    \/ (* Disjunct to prevent deadlock on termination *)
     ((\A r \in Replicas:
            \A inst \in cmdLog[r]: inst.status = "executed" \/ inst.status = "discarded") /\ UNCHANGED vars)
      (*\A r \in Replicas:
            \A inst \in cmdLog[r]: inst.status = "executed" \/ inst.status = "discarded") /\ UNCHANGED vars)*)


(***************************************************************************)
(* The complete definition of the algorithm                                *)
(***************************************************************************)

Spec == Init /\ [][Next]_vars

(***************************************************************************)
(* Safety Property                                                         *)
(***************************************************************************)
Nontriviality ==  (* Checking whether any command committed by any replica has been proposed by a client. *)
    \A i \in Instances :
        (\A C \in committed[i] : C[1] \in proposed \/ C[1] \in {[op |-> [key |-> "", type |-> ""]]})
        
        
Consistency == (* Two replicas can never have different commands committed for the same instance. *)
    \A i \in Instances :
        (Cardinality(committed[i]) <= 1)
        
        
Stability == (* For any replica, the set of committed commands at any time is a subset of the committed commands at any later time. *)
    \A replica \in Replicas :
        \A i \in Instances :
            \A C \in Commands :
                ((\E rec1 \in cmdLog[replica] :
                    /\ rec1.inst = i
                    /\ rec1.cmd = C
                    /\ rec1.status \in {"causally-committed", "strongly-committed", "executed", "discarded"}) =>
                    (\E rec2 \in cmdLog[replica] :
                        /\ rec2.inst = i
                        /\ rec2.cmd = C
                        /\ rec2.status \in {"causally-committed", "strongly-committed", "executed", "discarded"}))
       

SameSessionCausality ==  (* whether the same session causal order is maintaining or not *)
                \A replica1 \in Replicas: 
                            \A rec1 \in cmdLog[replica1]:
                                /\ rec1.status \in {"causally-committed", "strongly-committed", "executed", "discarded"} =>
                                    (/\ LET execution_order == rec1.execution_order_list IN (* pick execution order list for a specific instance *)
                                            /\ \A ctx \in Ctx_id: 
                                                /\ LET same_ctx_execution_order == SameCtxScc(execution_order, ctx, replica1)  (* Finding the instances from the same context id *)
                                                       ordered_same_ctx_execution_order == OrderingBasedOnInstanceNumber(same_ctx_execution_order) IN (* ordering (ascending) the instances based on the instance number. 
                                                        My assumption is that the command came earlier from a context will assign lower instance number than the command came
                                                        from the same context at a later time *)
                                                       /\ same_ctx_execution_order = ordered_same_ctx_execution_order)
                                                       
                                                       
                                                       
                                                   (* done *)    (* assign global order directly during the execution and then check *)
                                                       
                                            
GetFromCausality == (* whether the get from and transitive cauality is maintaining or not *)        
                \A replica1 \in Replicas: 
                            \A rec1 \in cmdLog[replica1]:
                                /\ rec1.status \in {"executed", "discarded"} => 
                                    (/\ rec1.cmd.op.type = "r"
                                    /\ LET max_write_instance == MaxWriteInstance(replica1,rec1.deps) (* finding the instance with max sequence of all dependent write of a specific read command*)
                                        max_execution_order == max_write_instance.execution_order (* max execution order *)
                                        recs2 == {rec \in cmdLog[replica1]: rec.inst[2] > rec1.inst[2]} (* finding all instances that came to the same replica after the read command *)
                                        IN
                                            IF recs2 = {} THEN TRUE
                                            ELSE 
                                                LET
                                                min_execution_order_recs == MinExecutionOrderRecs(recs2)
                                                min_execution_order == min_execution_order_recs.execution_order  (* finding the instance with the min execution order that came to the same replica after the read command *)
                                                IN                                      
                                                  /\ min_execution_order  > max_execution_order) 
                (* (GetFromCausality): I am doing the following for every read command:
                   1) finding all write operantions that are on the dependency list of the read command
                   2) finding the max execution order among all the write command of step1
                   3) finding the commands that came to the same replica (as read command) after the read command
                   4) finding the min execution order among all the commands of setp3
                *)(* problem when previous write also from the same session *)


GlobalOrderingOfWrite == (* checking whether the system is converging or not? *)
        LET pc == IsAllExecutedOrDiscarded IN  (* checking whether all instances across all the replicas are executed or discarded *)
                                pc = TRUE => \A key \in Keys: 
                                                LET all_latest_write == LatestWriteofSpecificKey(key) IN (* retrieving latest write of a specific key across all the replicas *)
                                                \A recs \in all_latest_write : \A otherrecs \in all_latest_write :
                                                 /\ recs.inst = otherrecs.inst (* comparing whether all latest write for a specific key , across all the replicas are same or not *)
                                 
                      
                                                                       
GlobalOrderingOfRead == (* once a strong read, read any write (strong/weak), all later strong read must observe that write *) 
                        (* I am checking majority committed or not. My assumption is that, is a commit is majority committed then it will be observed by all later strong commands *)
                        \A replica \in Replicas:
                            \A rec \in cmdLog[replica]:
                                  IF CheckStrongRead(rec) THEN (* as the command is read command, hence checking only for "strongly-committed" or executed "*)
                                    IF CheckDependentWrite(rec, replica) THEN
                                        TRUE
                                    ELSE
                                        FALSE
                                  ELSE
                                    TRUE
                                        
                (* (GlobalOrderingOfRead): I am doing the following for every strong read command:
                   1) finding the dependent instances of the strong read command
                   2) selecting the write commands from the dependent instance list
                   3) chekcing whether each of the instances of step2 is majority committed or not 
                *)         
                                    
                                        
RealTimeOrderingOfStrong  == (* If two interfering strong commands γ and δ are serialized by clients (i.e., δ is pro-
posed only after γ is committed by any replica), then every replica will execute γ before δ.*) (* this holds only for strong commands *)
    \A replica \in Replicas:
        \A rec1, rec2 \in cmdLog[replica]:
            IF rec1.consistency \in {"strong"} /\ rec2.consistency \in {"strong"} THEN 
                /\ rec1.commit_order >= rec2.commit_order => rec1.execution_order >= rec2.execution_order
            ELSE
                TRUE
    

(***************************************************************************)
(* Liveness Property                                                       *)
(***************************************************************************)


(***************************************************************************)
(* Termination Property                                                    *)
(***************************************************************************)

Termination == <>((\A r \in Replicas:
            \A inst \in cmdLog[r]: inst.status = "executed" \/ inst.status = "discarded"))
(*Termination == <>((\A r \in Replicas:
            \A inst \in cmdLog[r]: inst.status = "executed" \/ inst.status = "discarded"))*)
                                       
    

=============================================================================
\* Modification History
\* Last modified Fri Aug 22 17:23:35 EDT 2025 by santamariashithil
\* Created Thu Nov 30 14:15:52 EST 2023 by santamariashithil