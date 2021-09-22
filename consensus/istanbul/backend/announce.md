# Celo Announce Protocol

While here referred as a protocol by itself, Announce is technically a subset of the Istanbul protocol, on the [RLPx] transport, that facilitates Validators to privately discover the eNodeURL of other Validators in the network, using the underlying p2p eth network. This is used by Istanbul to have direct connections between Validators, speeding up the consensus phase of block generation.

## Basic Operation

When a validator is close to being elected ([NearlyElectedValidator]), it starts periodically sending [queryEnodeMsg] messages through the p2p network. These message are regossipped by nodes, and validators reply with a direct message to the originator with an [eNodeCertificateMsg], holding their `eNodeURL`. The initial [queryEnodeMsg] contained the origin validator's `eNodeURL` encrypted with the destination public key, therefore after the direct reply, both validators are aware of each others' `eNodeURL`.

### Keeping eNodeURL's up to date

To ensure that validator `eNodeURL` tables don't get stale, each validator's `eNodeURL` is paired with a version number, which is both attached to the origin [queryEnodeMsg], and in the [eNodeCertificateMsg] reply. Validators should ignore enodeurl from versions older than the highest known one. The current convention used is that versions are unix timestamps from the moment of the update.

### Minimizing network traffic

In order to reduce the amount of message flooding, there's a third type of message: the [versionCertificateMsg]. Periodically (or when it's updated by the validator operators) a validator will update its version, and gossip through the network a VersionCertificateMsg holding only its own new version.

When a node (every full node in the network) receives a [versionCertificateMsg], it will update the highest known versions according to the certificates, and regossip a VersionCertificateMsg holding only the version certificates that were new to it.

This allows every validator to have an idea of what the highest known version is for every other validator's `eNodeURL`, and in the next query only request for those that are stale.

### Proxy agnosticism

The announce protocol itself is agnostic to the concept of proxies, it cares only for the sharing of `<validator address, validator eNodeUrl>` tuples. It is the proxy implementation's responsibility to ensure the correct behavior of this specification.

### p2p connection management

Wether a validator opens a connection or not against another validator is not in the scope of this spec, but only how and when should the protocol messages should be sent, replied and regossipped. For example, it IS part of the spec what messages should be sent to a newly connected peer or validator peer.

## Protocol Messages (RLP)

// add types with the RLP nomenclature

### queryEnodeMsg (0x12)

`[[dest_address: , [encrypted_enode_url: ]], version: , timestamp: ]`

- `dest_address`: the destination validator address this message is querying to.
- `encrypted_enode_url`: the origin's `eNodeURL` encrypted with the destination public key.
- `version`: the current announce version for the origin's `eNodeURL`.
- `timestamp`: a message generation timestamp, used to bypass hash caches for messages.

### enodeCertificateMsg (0x17)

`[enode_url: , version: ]`

- `enode_url`: the origin's plaintext `eNodeURL`.
- `version`: the current announce version for the origin's `eNodeURL`.

### versionCertificatesMsg (0x16)

`[[version: , signature: ]]`

- `version`: the current highest known announce version for the specified validator `address`, according to the peer that gossipped this message.
- `signature`: the signature for the version payload (`'versionCertificate'|version`) by the emmitter of the certificate. Note that the address and public key can be deduced from this `signature`.


## Nearly Elected Validator (NEV)

// Explain the concept of a NEV and probably redefine it to something simpler

## Spec by node type

Note that validators are full nodes too.

### Full node (non validator) Spec

#### Peer handshake

During an inbound peer connection, the remote peer can send an [enodeCertificateMsg] to identify itself as a validator. This allows for preferential treatment for the p2p connection.

#### Handling [queryEnodeMsg]

Messages received should be only processed once, so a local cache is a must

Max amount of encrypted node urls is 2 * (max validator local set) // <- TODO: maybe change this? it's too variable and a bit weird

Should be regossipped as is, unless another message from the same validator origin has been regossipped in the past 5 minutes

#### Handling [enodeCertificateMsg]

Non validators should never receive [enodeCertificateMsg].

#### Handling [versionCertificatesMsg]

Messages received should be only processed once, so a local cache is a must.
when received, regossips new entries that hasn't been gossiped (by address) in 5 minutes

#### On new peer connection

// when a peer is registered, all version certificates should be sent to the registered peer in a [versionCertificatesMsg].

### Validator Spec

#### Handling [queryEnodeMsg]

Should be replied with an [enodeCertificateMsg] if the origin is a validator and this validator is the destination, and if an already existing connection to the origin validator exists.

#### Handling [enodeCertificateMsg]


#### Query spawning
// currently sending queries if this validator is a [NearlyElectedValidator]
// on start, wait 1 minute before querying
// first 10 queries can be spaced by 1 minute after starting. after that, every 5 minutes
// cant send more than (retrybackoff for highest known version) for each destination
// only sends if a higher version than the one this validator has is known


#### Version certificates spawning
// currently updating own announce version every 5 minutes, if node is validating
// also every 5 minutes, gossips the whole version certificate table to peers
// sent when updating announce version (from validator to everyone)




## Change Log

