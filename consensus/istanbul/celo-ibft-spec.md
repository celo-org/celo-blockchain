# Specification for Celo IBFT protocol

## Celo IBFT specification

This document is an attempt to specify at a conceptual level, the consensus
algorithm currently used by the Celo blockchain. It attempts to distill
everything that is pertinent to consensus and eschew all that is not. Where
consensus is the problem of agreeing on a sequence of values in a decentralized
manner.

It is not trying to specify everything that lives under `consensus/istanbul`.

By doing this we allow for a clean and clear specification of the consensus,
conversely this specification does not resemble the code and mapping this to
the code and vice versa, requires considerable effort.

### High level overview

The Celo IBFT protocol is a BFT (Byzantine Fault Tolerant) protocol that allows
a group of participants (commonly referred to as validators in the Celo
ecosystem) to agree on the ordering of values by exchanging voting messages
across a network. The voting proceeds in rounds and for each round a specific
participant is able to propose the value to be agreed upon, they are known as
the proposer. As long as less than 1/3rd of participants deviate from the
protocol then the protocol should ensure that there is only one ordering of
values that participants agree on and also that they can continue to agree on
new values, i.e. they don't get stuck.

Ensuring that only one ordering is agreed upon is referred to as 'safety' and
ensuring that the participants can continue to agree on new values is referred
to as 'liveness'.

### System model

We consider a group of `3f+1` participating computers (participants) that are
able to communicate across a network. The group should be able to tolerate `f`
failures without losing liveness or safety.

All participants know the other participants in the system and it is assumed
that all messages are signed in some way such that participants in the protocol
know which participant a message came from and that messages cannot be forged.

Only messages from participants are considered.

For messages containing a value only messages with valid values are considered.

Participants are able to broadcast messages to all other participants. In the
case that a participant is off-line or somehow inaccessible they will not
receive broadcast messages and there is no mechanism for these messages to be
re-sent.

We refer to a consensus instance to mean consensus for a specific height,
consensus instances are independent (they have no shared state), that is
messages or values from the previous height have no bearing on the outcome of
the current height

We assume the existence of an application that feeds values to be agreed upon
to the consensus instance and also receives agreed upon values from the
consensus. The application also provides application specific implementations
for certain functions that the consensus algorithm relies upon, such as a
function that determines the validity of a value. 

### Algorithm

See supporting [functions](#Functions) and [notation](#Appendix-1-Notation).

Note all state modifications are contained in the following code block,
supporting functions cannot modify state, they all operate without side
effects.

Variable names                  |Instance state                          
--------------------------------|----------------------------------------
`H - height`                    |`Hc - current height`                                      
`R - round          `           |`Rc - current round`                    
`V - value`                     |`Rd - desired round`                    
`T - Message type`              |`Vc - currently proposed value`         
`RCC - round change certificate`|`PCc - current prepared certificate`    
`PC - prepared certificate`     |`Sc - current participant state`        
`S - participant state`         |
`M or N - message sets`         |

```

// Upon receiving this event the algorithm transitions to the next height the
// final committed event signifies that the application has accepted the agreed
// upon value for the current height.
upon: <FinalCommittedEvent>
  Hc ← Hc+1
  Rc ← 0
  Rd ← 0
  Sc ← AcceptRequest
  Vc ← nil
  schedule onRoundChangeTimeout(Hc, 0) after roundChangeTimeout(0)

// A request event is a request to reach agreement on the provided value, the
// request event is sent by the application, if this parcicipant is the proposer
// it will propose that value by sending a preprepared message.
upon: <RequestEvent, Hc, V> && Sc = AcceptRequest
  if Rc = 0 && isProposer(Hc, Rd) {
    bc(<Preprepare, Hc, 0, V, nil>)
  }

// When receiving a preprepare from a proposer participants will vote for the
// value (if valid) by sending a prepare message.
upon: <Preprepare, Hc, Rd, V, RCC> from proposer(Hc, Rd) && Sc = AcceptRequest
  if (Rd > 0 && validRCC(V, RCC)) || (Rd = 0 && RCC = nil)  {
    Rc ← Rd
    Vc ← V
    Sc ← Preprepared
    bc(<Prepare, Hc, Rd, Vc>)
  }

// When a participant sees at least 2f+1 prepare or commit messages for a value
// it will send a commit message for that value.
upon: M ← { <T, Hc, Rd, Vc> : T ∈ {Prepare, Commit} } && |M| >= 2f+1 && Sc ∈ {AcceptRequest, Preprepared} 
  Sc ← Prepared
  PCc ← <PreparedCertificate, M, Vc>
  bc(<Commit, Hc, Rd, Vc>)

// When a participant sees at least 2f+1 commit messages for a value, they
// consider that value committed (agreed) and pass the value to the application,
// which will in turn issue a final committed event if the value is considered
// valid by the application.
upon: M ← { <Commit, Hc, Rd, Vc> } && |M| >= 2f+1 && Sc ∈ {AcceptRequest, Preprepared, Prepared} 
  Sc ← Committed
  deliverValue(Vc)

// Upon receipt of a round change if that round change is old, send the participant's round
// change back to the sender to help them catch up. Othewise if there are at least 2f+1 round
// change messages sharing the same round then switch to it. Otherwise if there are at least
// f+1 round change messages switch to the higest round that is less than or equal to the top
// f+1 rounds.
upon: m<RoundChange, Hc , R, PC> && (PC = nil || validPC(PC)) 
  if R < Rd {
    send(<RoundChange, Hc, Rd, PCc>, sender(m))
  } else if quorumRound() > Rd {
    Rd ← quorumRound()
    Rc ← quorumRound()
    Sc ← AcceptRequest
    schedule onRoundChangeTimeout(Hc, Rd) after roundChangeTimeout(Rd)
    if Vc != nil && isProposer(Hc, Rc) {
      bc(<Preprepare, Hc, Rc, Vc, PCc>)
    }
  } else if f1Round() > Rd {
    Rd ← f1Round() 
    Sc ← WaitingForNewRound
    schedule onRoundChangeTimeout(Hc, Rd) after roundChangeTimeout(Rd)
    bc(<RoundChange, Hc, Rd, PCc>)
  }

// Functions that modify instance state.

// As long as the round and height have not changed since it was scheduled
// onRoundChangeTimeout sets the desired round to be one greater than the Current
// round, sets the current state to be WaitingForNewRound and broadcasts a round
// change message.
// 
// Note: This function is referred to in the code as
// `handleTimeoutAndMoveToNextRound`, which is misleading because it does not move
// to the next round, it only updates the desired round and sends a round change
// message. Hence why it has been renamed here to avoid confusion.
onRoundChangeTimeout(H, R) {
  if H = Hc && R = Rc {
    Rd ← Rc+1
    Sc ← WaitingForNewRound
    schedule onRoundChangeTimeout(Hc, Rd) after roundChangeTimeout(Rd)
    bc<RoundChange, Hc, Rd, PCc>
  }
}
```

### Supporting Functions

These functions can read global state but cannot modify it.

#### Application provided functions
No pseudocode is provided for these functions since their implementation is
application specific.

`proposer(H,R)`\
Returns the proposer for the given height and round.

`isProposer(H, R)`\
Returns true if the current participant is the proposer for the given height
and round. 

`deliverValue(V)`\
Delivers the given value to the application.

`roundChangeTimeout(R)`\
Returns the timeout for the given round 

`bc(<PP, H, R, V>)`\
Broadcasts the given message to all connected participants. 

`send(<Commit, H, R, V>, sender(m))`\
Sends the given message to to the sender of another message.

#### PCRound
Asserts that all messages in the given prepared certificate share the same round and returns that round.
```
PCRound(<PreparedCertificate, M, *>) {
  ∃ R : ∀ m<*, *, Rm, *> ∈ M : R = Rm
  return R
}
```

#### PCValue
Return the value associated with a prepared certificate.
```
PCValue(<PreparedCertificate, *, V>) {
  return V
}
```

#### validPC

Returns true if the message set contains prepare or commit messages from at
least 2f+1 and no more than 3f+1 participants for current height, matching the
prepared certificate value and all sharing the same height and round.
```
validPC(<PreparedCertificate, M, V>) {
  N ← { m<T, Hc, *, Vm> ∈ M : (T = Prepare || T = Commit) && Vm = V } 
  return 2f+1 <= |N| <= 3f+1 &&
  ∀ m<*, Hm, Rm, *>, n<*, Hn, Rn, *> ∈ N : Hm = Hn && Rm = Rn 
}
```

#### validRCC

Returns true if the round change contains at least 2f+1 and no more than 3f+1
round changes for the current height, with a round greater or equal to the
desired round and either have a valid prepared certificate or no prepared
certificate. If any round change certificates have a prepared certificate, then
there must exist one with round greater than or equal to all the others and
with a value of V.

```
validRCC(V, RCC) {
  M ← { m<RoundChange, Hm , Rm, PC> ∈ RCC : Hm = Hc && Rm >= Rd && (PC = nil || validPC(PC)) }
  N ← { m<RoundChange, * , *, PC> ∈ M : PC != nil }
  if |N| > 0 {
    return 2f+1 <= |M| <= 3f+1 &&
    ∃ m<RoundChange, *, *, Pcm> ∈ N : validPC(PCm) && PCValue(PCm) = V && ∀ n<RoundChange, * , *, PCn> ∈ N != m : PCRound(PCm) >= PCRound(PCn)
  }
  return 2f+1 <= |M| <= 3f+1
}
```

#### quorumRound

Asserts that at least 2f+1 round change messages share the same round and
returns that round.
```
quorumRound() {
  M ← { m<RoundChange, Hc, Rm, *>, n<RoundChange, Hc, Rn, *> : Rm = Rn } &&
  |M| >= 2f+1 &&
  ∃ R : ∀ m<*, *, Rm, *> ∈ M : R = Rm
  return R
}
```

#### f1Round

Asserts that there are at least f+1 round change messages and returns the
lowest round from the top f+1 rounds.
```
f1Round() {
  // This is saying that for any Rm there cannot be >= f+1 elements set with a
  // larger R, since if there were that would mean that Rm is not in the top
  // f+1 rounds. 
  M ← { m<RoundChange, Hc, Rm, *> : |{ n<RoundChange, Hc, Rn, *> : Rm < Rn }| < f+1 } &&
  |M| >= f+1 &&
  ∃ R : ∀ m<*, *, Rm, *> ∈ M : R <= Rm
  return R
}
```

#### onRoundChangeTimeout
As long as the round and height have not changed since it was scheduled
onRoundChangeTimeout sets the desired round to be one greater than the Current
round, sets the current state to be WaitingForNewRound and broadcasts a round
change message.

Note: This function is referred to in the code as
`handleTimeoutAndMoveToNextRound`, which is misleading because it does not move
to the next round, it only updates the desired round and sends a round change
message. Hence why it has been renamed here to avoid confusion.

```
onRoundChangeTimeout(H, R) {
  if H = Hc && R = Rc {
    Rd ← Rc+1
    Sc ← WaitingForNewRound
    schedule onRoundChangeTimeout(Hc, Rd) after roundChangeTimeout(Rd)
    bc<RoundChange, Hc, Rd, PCc>
  }
}
```


## Appendix 1: Notation
Elements of sets are represented with lower case letters (e.g. `m ∈ M`).
Because all sets are messages we use `m` to represent an element and if we
need to denote 2 messages from the same set we use `n` to denote the second
message. Other variables are represented with Upper case letters (e.g. `H` for
height).

Composite objects are defined by a set of comma separated variables enclosed in
angled brackets.\
E.g. `<A, B, C>`

If we need to refer to the composite object we prefix the angled brackets with
an identifier that is a lower case letter.\
E.g. `m<A, B, C>`

An identifier can also be provided in order to distinguish variables belonging
to a composite object from other similarly named variables in the same scope.
In this case the variables are annotated with the identifier by appending the
identifier to the variable name.\
E.g. `m<Am, Bm, Cm>`

If all instances of a composite object element share the same value for one
of its variables then that value can be used in the definition. E.g. `<A, B, Hc>`
represents a composite element with variables `A` `B` and current height.

If a composite object element has variables for which the value is not
important then `*` is used in the place of that variable.

### Numbers of participants
`3f+1 - the total number of participants`\
`2f+1 - a quorum of participants`
`f - the number of failed or malicious parcicipants that the system can tolerate`

### Participant states
`AcceptRequest`\
`Preprepared`\
`Prepared`\
`Committed`\
`WaitingForNewRound`

### Variable names
`S - participant state`\
`H - height`\
`R - round`\
`V - value`\
`T - Message type`\
`RCC - round change certificate`\
`PC - prepared certificate`\
`M or N - message sets`\
`nil - indicates that the relevant variable is not set`

### Message composite object structures
`<Preprepare, H, R, V, RCC>`\
`<Prepare, H, R, V>`\
`<Commit, H, R, V>`\
`<RoundChange, H, R, PC>`\
`<PreparedCertificate, M, V>`

### Event composite object structures
Events are a means for the application to communicate with the consensus
instance, they are never sent across the network.

`<RequestEvent, H, V> - request event, provides the value for the proposer to propose`\
`<FinalCommittedEvent> - final committed event, sent by the application to initiate a new consensus instance`

### Pseudocode notation
```
Function definitions are represented as follows where the name of the function is foo, its
parameters are X and Y, functions can optionally return a value.

foo(X, Y) {
  ...  
  return X
}
```
```
Conditional statements are represented as follows where statements inside only
one set of the curly braces will be executed if the corresponding condition C
evaluates to true or in the case of the final else those statements will be
executed if none of the previous conditions evaluated to true. 

if C {
  ...  
} else if C {
  ...  
} else {
  ...  
}
```
```
upon: UponCondition - Pseudocode directly following upon statements is executed
when the associated UponCondition evaluates to true

Upon statements are triggered upon receipt of messages or events when the
associated upon condition evaluates to true.
Upon conditions are structured thus:

<composite object, set of objects or eventto match against> <additional qualifications>

E.G:
// 2f+1 commit messages for the current round and height with a non nil value.
M ← <Commit, Hc, Rc, V> && |M| = 2f+1 && V != nil
```
```
schedule <function call> after <duration> - This notation schedules the given
function call to occur after the given duration.

```

### Math notation
`← - assignment`\
`= - is equal`\
`!= - is not equal`\
`&& - logical and`\
`|| - logical or`\
`{X, Y} - the set containing X and Y`\
`|M| - the cardinality of M`\
`m ∈ M - m is an element of the set M`\
`{ m : C(m) } - set builder notiation, the set of messages m such that they satisfy condition C`\
`∃ m : C(m) - there exists m that satisfies condition C`\
`∀ m : C(m) - all m satisfy condition C`

### Math Notation examples
```
// There exists a commit message m in M such that m's height (Hm) is
// less than m's round (Rm) and m's value (V) is not important.
∃ m<Commit, H, R, *> ∈ M : Hm < Rm

// The cardinality of prepare messages in M with height and round equal
// to Hc and value equal to V is greater than 1 and less than 10.
1 < |{ m<Prepare, Hc, Rm, Vm> ∈ M : Rm = Hc && Vm = V }| < 10
```

## Strange things

This is from check message but it doesn't check the message
it just compares current round to desired

consensus/istanbul/core/backlog.go:64
  if c.current.Round().Cmp(c.current.DesiredRound()) > 0 {

## Thoughts

The future preprepare timer seems unnecessary, shouldn't the future preprepare
message simply be handled when moving to the future sequence? Actually I think
the future prepare timer is there to ensure that the network doesn't race ahead
of the proposed block times, but it would be better if nodes simply waited some
amount of time from the last block rather than making a calculation based on
the time value set by the proposer.

It seems the resetResendRoundChangeTimer functions are there to ensure round
changes get resent, we need to represent that here because we don't assume
reliable broadcast.

When a round change timeout occurs it starts the timer for the next round
change, which will result in another round change message being broadcast for
the newer round, so why do we need the resendRoundChangeMessage functionality?

I think nodes can get stuck:

Say all nodes are at round 0 they all time out and send round changes for round 1 so they all move to round one.

Now f+1 nodes timeout and send round changes for round 2 they have desired round 2 current round 1. The remaining 2f
nodes receive the f+1 round changes and set desired round to 2 and schedule
roundChangeTimeout with a target round of 2.

Now if those 2f nodes have their scheduled roundChangeTimeout execute before
they have updated their current round to 2 roundChangeTimeout will be a no-op.
if the round changes sent by these 2f nodes are lost then they will never
schedule any further round changes without outside intervention.

The other nodes will have their round changes for round expire and their
current round will be 1 meaning that their roundChangeTimeouts will also be
no-ops. 

And then the network is stuck

I think nodes can get stuck2:

Everyone times out and Sends a round change for round 1. All the round change
messages are lost.  So all nodes have Rc 0 and Rd 1 state WaitingForNewRound.
And their next round change timeout is looking for Rc to be equal to 1 because
Rd was set to 1. But Rc is only ever set when receiving a preprepare or
receiving a quorum of round changes.  So the next roundChangeTimeout will be a
no op and then no more will be scheduled.

This is why we need the resend round change timer.
