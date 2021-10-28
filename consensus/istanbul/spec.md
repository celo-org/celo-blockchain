# Specification for Celo IBFT protocol

## Celo IBFT specification

### High level overview

The Celo IBFT protocol is a BFT (Byzantine Fault Tolerant) protocol that allows
a group of participants to agree on the ordering of values by exchanging voting
messages across a network. As long as less than 1/3rd of participants follow
the protocol correctly then the protocol should ensure that there is only one
ordering of values that participants agree on and also that and that they can
continue to agree on new values, i.e. they don't get stuck.

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
consensus instances are independent (they have no shared state).

### Algorithm
See supporting [functions](#Functions) and [notation](Appendix-1:-Notation).
```
upon: FC_E
  Hc ← Hc+1
  Rc ← 0
  Rd ← 0
  Sc ← AcceptRequest
  Vc ← nil
  schedule onRoundChangeTimeout(Hc, 0) after roundChangeTimeout(0)

upon: <R_E, Hc, V> && Sc = AcceptRequest
  if Rc = 0 && isProposer(Hc, Rd) {
    bc(<PP_T, Hc, 0, V, nil>)
  }

upon: <PP_T, Hc, Rd, V, RCC> from proposer(Hc, Rd) && Sc = AcceptRequest
  if (Rd > 0 && validRCC(Hc, Rd, V, RCC)) || (Rd = 0 && RCC = nil)  {
    Rc ← Rd
    Vc ← V
    Sc ← Preprepared
    bc(<P_T, Hc, Rd, Vc>)
  }

upon: M ← { <T, Hc, Rd, Vc> : T ∈ {P_T, C_T} } && |M| > 2f+1 && Sc ∈ {AcceptRequest, Preprepared} 
  Sc ← Prepared
  PCc ← <PC_T, M, Vc>
  bc(<C_T, Hc, Rd, Vc>)

upon: M ← { <C_T, Hc, Rd, Vc> } && |M| > 2f+1 && Sc ∈ {AcceptRequest, Preprepared, Prepared} 
  Sc ← Committed
  deliverValue(Vc)

upon: m<RC_T, Hc , R, PC> && (PC = nil || validPC(PC)) 
  if R < Rd {
    send(<RC_T, Hc, Rd, PCc>, sender(m))
  } else if quorumRound() > Rd {
	Rd ← quorumRound()
	Rc ← quorumRound()
    schedule onRoundChangeTimeout(Hc, Rd) after roundChangeTimeout(Rd)
	if Vc != nil && isProposer(Hc, Rc) {
      bc(<RC_T, Hc, Rc, Vc, PCc>)
	}
  } else if f1Round() > Rd {
    Rd ← f1Round() 
    Sc ← WaitingForNewRound
    schedule onRoundChangeTimeout(Hc, Rd) after roundChangeTimeout(Rd)
    bc(<RC_T, Hc, Rd, PCc>)
  }
```

### Functions

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

`send(<C_T, H, R, V>, sender(m))`\
Sends the given message to to the sender of another message.

#### PCRound
Asserts that all messages in the given prepared certificate share the same round and returns that round.
```
PCRound(<PC_T, M, *>) {
  ∃ R : ∀ m<*, *, Rm, *> ∈ M : R = Rm
  return R
}
```

#### PCValue
Return the value associated with a prepared certificate.
```
PCValue(<PC_T, *, V>) {
  return V
}
```

#### validPC

Returns true if the message set contains prepare or commit messages from at
least 2f+1 and no more than 3f+1 participants for current height, matching the
prepared certificate value and all sharing the same height and round.
```
validPC(<PC_T, M, V>) {
  N ← { m<T, Hc, *, Vm> ∈ M : (T = P_T || T = C_T) && Vm = V } 
  return 2f+1 <= |N| <= 3f+1 &&
  ∀ m<*, Hm, Rm, *>, n<*, Hn, Rn, *> ∈ N : Hm = Hn && Rm = Rn 
}
```

#### validRCC

Returns true if the round change contains at least 2f+1 and no more than 3f+1
round changes that match the given height and have a round greater or equal
than the given round and either have a valid preparedCert or no preparedCert.
If any round change certificates have a prepared cert, then there must exist
one with greater than or equal round to all the others and with a value of V.

```
validRCC(H, R, V, RCC) {
  M ← { m<RC_T, Hm , Rm, PC> ∈ RCC : Hm = H && Rm >= R && (PC = nil || validPC(PC)) }
  N ← { m<RC_T, Hm , Rm, PC> ∈ M : PC != nil }
  if |N| > 0 {
    return 2f+1 <= |M| <= 3f+1 &&
    ∃ m<RC_T, *, *, Pcm> ∈ N : validPC(PCm) && PCValue(PCm) = V && ∀ n<RC_T, * , *, PCn> ∈ N != m : PCRound(PCm) >= PCRound(PCn)
  }
  return 2f+1 <= |M| <= 3f+1 &&
}
```

#### quorumRound

Asserts that at least 2f+1 round change messages share the same round and
returns that round.
```
quorumRound() {
  M ← { m<RC_T, Hc, Rm, *>, n<RC_T, Hc, Rn, *> : Rm = Rn } &&
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
  M ← { m<RC_T, Hc, Rm, *> : |{ n<RC_T, Hc, Rn, *> : Rm < Rn }| < f+1 } &&
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
    bc<RC_T, Hc, Rd, PCc>
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

### Global Variables
`Sc - current participant state`\
`Hc - current height`\
`Rc - current round`\
`Rd - desired round`\
`Vc - currently proposed value`\
`PCc - current prepared cert`\
`f - the number of failed or malicious parcicipants that the system can tolerate`\
`3f+1 - the total number of participants`\
`2f+1 - a quorum of participants`

### Participant states
`AcceptRequest`\
`Preprepared`\
`Prepared`\
`Committed`\
`WaitingForNewRound`

### Message Types
`PP_T - preprepare`\
`P_T - prepare`\
`C_T - commit`\
`RC_T - round change`\
`PC_T - prepared cert`

### Message composite object structures
`<PP_T, H, R, V, RCC> - preprepare`\
`<P_T, H, R, V> - prepare`\
`<C_T, H, R, V> - commit`\
`<RC_T, H, R, PC> - round change`\
`<PC_T, M, V> - prepared certificate`

### Event types
Events are a means for the application to communicate with the consensus
instance, they are never sent across the network.

`R_E - request`\
`FC_E - final committed`

### Event composite object structures
`<R_E, H, V> - request event, provides the value for the proposer to propose`\
`<FC_E> - final comitted event, sent by the application to initiate a new consensus instance`

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
M ← <C_T, Hc, Rc, V> && |M| = 2f+1 && V != nil
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
∃ m<C_T, H, R, *> ∈ M : Hm < Rm

// The cardinality of prepare messages in M with height and round equal
// to Hc and value equal to V is greater than 1 and less than 10.
1 < | m<P_T, Hc, Rm, Vm> ∈ M : Rm = Hc && Vm = V| < 10
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
changes get resent, but I don't think we need to represent that here because we
assumed reliable broadcast.

When a round change timeout occurs it starts the timer for the next round
change, which will result in another round change message being broadcast for
the newer round, so why do we need the resendRoundChangeMessage functionality.
