package eth

import (
	"bytes"

	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/rlp"
)

// EncodeAndSend writes an RLP-encoded message with the given code.
// data should encode as an RLP list.
func (p *Peer) EncodeAndSend(msgcode uint64, data []byte) error {
	raw, err := rlp.EncodeToBytes(data)
	if err != nil {
		return err
	}
	return p.Send(msgcode, raw)
}

// Send sends the message to this peer. Since current istanbul protocol
// has messages encoded twice, data should be twice encoded
// with rlp encoding.
func (p *Peer) Send(msgcode uint64, data []byte) error {
	// Istanbul was encoding messages before sending it to the peer,
	// then the peer itself would re-encode them before writing it into the
	// output stream. This made it so that sending a message to 100 peers (validators),
	// would encode the message a first time, then one hundred times more. With this
	// change (making the double encode explicit here) we ensure the peer already
	// receives the message in double encoded form, reducing the amount of rlp.encode
	// calls from 101 to 2.
	return p.rw.WriteMsg(p2p.Msg{Code: msgcode, Size: uint32(len(data)), Payload: bytes.NewReader(data)})

}
