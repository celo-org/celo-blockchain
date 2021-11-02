# Celo Announce Protocol

While here referred as a protocol by itself, Announce is technically a subset of the Istanbul protocol, on the [RLPx] transport, that facilitates Validators to privately discover the `eNodeURL` of other Validators in the network, using the underlying p2p eth network. This is used by Istanbul to have direct connections between Validators, speeding up the consensus phase of block generation.

It is important to note that a validator's `eNodeURL` is never shared among third parties, but directly published to specific peers. Therefore it is allowed for a validator to publish different external facing `eNodeURL` values to different validator peers.

To clarify, the `eNodeURL` value itself is public, since it's used in the p2p network. What is NOT public, and the announce protocol helps in sharing among validators, is the _mapping_ between a validator and its `eNodeURL`. That is, p2p full nodes don't know (and should not know) if a peer is a validator or not.

## Terminology

For the purpose of this specification, certain terms are used that have a specific meaning, those are:

* Full node
* Validator
* Nearly Elected Validator
* eNodeURL

Note that node labels (Full node, Validator, Nearly Elected Validator) are not set in stone. A Full node can be restarted into a Validator, and a Validator can become a Nearly Elected Validator with enough votes from the election contract.

### Full Node

A Full Node is a Celo node fully operating in the Celo p2p network.

### Validator

A Validator is a [FullNode] that has been started with the ability to produce blocks, even if it has not yet been elected. In the geth program,
this is equivalent as being started with the `--mine` param.

### Nearly Elected Validator (NEV)

A node is a Nearly Elected Validator (NEV) if and only if:

1) It is a [Validator]
2) It is in the result set from calling `ElectNValidatorSigners` with `additionalAboveMaxElectable = 10`

In loose terms, it means it's a validator that has a good chance of becoming an elected validator in the following epoch.

### eNodeURL

An `eNodeURL` is a string specifyng the location of an eNode in the p2p network (//TODO add link to what an enode is?), and it has the following format:

`enode://<hex node id>@<IP/hostname>:<tcp port>?discport=<udp discovery port>`

Where `<hex node id>` is the node's serialized and hex encoded ecdsa public key. The URL parameter `discport` should only be specified if the udp discovery port differs from the tcp port.

Some example `eNodeURLs` (with partially elided hex encoded public keys):

```
enode://517318.......607ff3@127.0.0.1:34055

enode://e5e2fdf.......348ce8@127.0.0.1:33503?discport=22042
```

## Objective

The Announce protocol objective is to allow all [NearlyElectedValidator] to maintain an updated table of `validatorAddress -> eNodeURL` for all other [NearlyElectedValidator], while somewhat concealing these values from the rest of the nodes in the network, and also avoiding an unnecessary high message traffic to achieve it. [NearlyElectedValidator] nodes can use the information from this table to open direct connections between themselves, speeding up the consensus phase.

To ensure that validator [eNodeURL] tables don't get stale, each validator's [eNodeURL] is paired with a version number. [NearlyElectedValidator] nodes should ignore [eNodeURL] entries with values from versions older than one currently in their table. The current convention used is that versions are unix timestamps from the moment of the update.

As part of the protocol's design, a [NearlyElectedValidator] can advertise different [eNodeURL] values for different destinations. This is important since validators can live behind multiple proxies and thus have more than one [eNodeURL]. That being said, the announce protocol itself is agnostic to the concept of proxies, it cares only for the sharing of `<validator address, validator eNodeUrl>` mapping tuples. It is the proxy implementation's responsibility to ensure the correct behavior of this specification.

### Concealing eNodeURL values

It is important to ensure that validator [eNodeURL] values can't be discovered by nodes that are not in the [NearlyElectedValidator] set. To achieve this, they are shared in one of two ways:

* Sent through a direct p2p connection from one [NearlyElectedValidator] to another ( [enodeCertificateMsg] )
* Gossipped through the network, but encrypted with the public key of the recipient [NearlyElectedValidator] ( [queryEnodeMsg] )


### Minimizing network traffic

In order to reduce the amount of message flooding, every [FullNode] participating in the p2p network must maintain a version table for the [NearlyElectedValidator] nodes in the network. This table is built and updated with one specific message from the protocol ( [versionCertificatesMsg] ).

[NearlyElectedValidator] can also use this table to understand which [eNodeURL] entries are stale, if the [eNodeURL] table entry has a version older than the version table entry.

## Basic Operation

When a validator is close to being elected ([NearlyElectedValidator]), it starts periodically sending [queryEnodeMsg] messages through the p2p network. These message are regossipped by nodes, and validators reply with a direct message to the originator with an [eNodeCertificateMsg], holding their `eNodeURL`. The initial [queryEnodeMsg] contained the origin validator's `eNodeURL` encrypted with the destination public key, therefore after the direct reply, both validators are aware of each others' `eNodeURL`.

### p2p connection management

Wether a validator opens a connection or not against another validator is not in the scope of this spec, but only how and when should the protocol messages should be sent, replied and regossipped. For example, it IS part of the spec what messages should be sent to a newly connected peer or validator peer.

## Protocol Messages (RLP)

All messages presented here are Istanbul messages, following the same format as the consensus messages (see `istanbul.Message`), which are serialized in the `payload` field for standard p2p messages.

`[code: P, msg: B, sender_address: B_20, signature: B]`

- `code`: the message code (0x12, 0x16, or 0x17).
- `msg`: rlp encoded data, depending on the message type (`code`), and detailed in the following subsections.
- `sender_address`: the originator of the message.
- `signature`: the message signature.

// TODO: double check RLP types

### queryEnodeMsg (0x12)

This message is sent from a [NearlyElectedValidator] and regossipped through the network, with the intention of reach out for
other [NearlyElectedValidator] nodes and discover their [eNodeURL] values, while at the same time sending its own. Note that
the sender sends many queries in one message. A query is a pair `<destination address, encrypted eNodeURL>` so that only the
destination address can decrypt the encrypted [eNodeURL] in it.

`[[dest_address: B_20, encrypted_enode_url: B], version: P, timestamp: P]`

- `dest_address`: the destination validator address this message is querying to.
- `encrypted_enode_url`: the origin's `eNodeURL` encrypted with the destination public key.
- `version`: the current announce version for the origin's `eNodeURL`.
- `timestamp`: a message generation timestamp, used to bypass hash caches for messages.

Max amount of encrypted node urls is 2 * size(set of [NearlyElectedValidator]).

### enodeCertificateMsg (0x17)

This message holds only ONE [eNodeURL] and it's meant to be send directly from [NearlyElectedValidator] to [NearlyElectedValidator] in a direct
connection as a response to a [queryEnodeMsg].

`[enode_url: B, version: P]`

- `enode_url`: the origin's plaintext `eNodeURL`.
- `version`: the current announce version for the origin's `eNodeURL`.

### versionCertificatesMsg (0x16)

This messages holds MANY version certificates. It is used mostly in two ways:

- To share the WHOLE version table a [FullNode] has.
- To share updated parts of the table of a [FullNode].

`[[version: P, signature: B]]`

- `version`: the current highest known announce version for a validator (deduced from the signature), according to the peer that gossipped this message.
- `signature`: the signature for the version payload string (`'versionCertificate'|version`) by the emmitter of the certificate. Note that the address and public key can be deduced from this `signature`.

## Spec by node type

Note that validators are full nodes too, therefore they should follow the same rules as full nodes. This is specially important to not give away the status of the node as a validator.

### Full node (non validator) Spec

#### Peer registered

When a peer is registered, the whole version table should be sent to the peer in a [versionCertificatesMsg].

#### Handling [queryEnodeMsg]

Messages received of this type should be only processed once.
Should be regossipped as is, unless another message from the same validator origin has been regossipped in the past 5 minutes.

#### Handling [enodeCertificateMsg]

[enodeCertificateMsg] should be ignored by [FullNode] instances, since they should never receive one if all other participants are behaving properly. We make
this explicit in the spec since the regossipping of this particular message would make the [eNodeURL] of the sender public, which defeats the purpose of the
protocol. While this should never happen, there are some border cases where a node can be a [Validator] with queries yet unanswered, and then restarted into a [FullNode], thus receiving query replies that should no longer apply to it.

#### Handling [versionCertificatesMsg]

Messages received of this type should be only processed once.
When received, it should upsert its version table, and then multicast to its peers a new filtered [versionCertificatesMsg] without the certificates that were not updated in the table, and also filtering out certificates if another one for the same [NearlyElectedValidator] has been gossiped in the past 5 minutes.

#### Message spawning

Every 5 minutes it should multicast its version certificate table to all peers.

### Validator Spec

#### Peer handshake

When a [NearlyElectedValidator] connects to another [NearlyElectedValidator], the inbound peer can send an [enodeCertificateMsg] to identify itself as a [NearlyElectedValidator]. This allows for preferential treatment for the p2p connection.

This can happen for example if: Say `A` and `B` are both [NearlyElectedValidator] nodes and not directly connected in the p2p network. `A` sends a [queryEnodeMsg] to the network, `B` receives it and therefore decides to directly open a p2p connection to `A`. As soon as the conection is registered, it identifies itself by sending the [enodeCertificateMsg] to `A`.

#### Handling [queryEnodeMsg] as a Validator

If:

* this [Validator] is a [NearlyElectedValidator]
* there is an entry in the [queryEnodeMsg] addressed to this [Validator]

Then this [Validator] should upsert the [eNodeURL] received.

If, in addition, this [Validator] is already connected to the sender of the query, it should reply with an [enodeCertificateMsg] directly.

Regardless of all that, the message should be regossipped through the message like any [FullNode] would do.

#### Receiving [enodeCertificateMsg]

Should upsert the [eNodeURL] received in the local [eNodeURL] table.

#### Query spawning

One minute (60 seconds) after a validator enters the set of [NearlyElectedValidator], it should start sending [queryEnodeMsg] messages to the network, for all other [NearlyElectedValidator] nodes that have a higher version `eNodeURL` than the one currently known.

Messages should be spaced at least 5 minutes (300 seconds) apart. A query for a specific `<validator, version>` tuple has a retry back off period. The `nth` attempt should be spaced from the previous one by:

```
timeoutMinutes = 1.5 ^ (min(n - 1, 5))
```

Optionally, the first ten (10) query messages can be spaced by at least 60 seconds and ignoring retry back off periods for unanswered queries. This is known as the `AggressiveQueryEnodeGossip`.

If the [Validator] is no longer in the list of [NearlyElectedValidator] then it should stop sending [queryEnodeMsg] messages.

#### Version certificates spawning

When a [Validator] enters the [NearlyElectedValidator] set, it should update its [eNodeURL] version, gossip a new [versionCertificatesMsg] with its new certificate. After that, it should be renewed and regossipped every 5 minutes, until its removed from the [NearlyElectedValidator] set.

## Change Log

### Previous relevant PRs

https://github.com/celo-org/celo-blockchain/pull/816

https://github.com/celo-org/celo-blockchain/pull/873

https://github.com/celo-org/celo-blockchain/pull/893

[queryEnodeMsg]: #queryEnodeMsg-0x12
[versionCertificatesMsg]: #versionCertificatesMsg-0x16
[enodeCertificateMsg]: #enodeCertificateMsg-0x17
[NearlyElectedValidator]: #nearly-Elected-Validator-NEV
[Validator]: #validator
[FullNode]: #full-node
[eNodeURL]: #eNodeURL