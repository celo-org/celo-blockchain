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
	return p.rw.WriteMsg(p2p.Msg{Code: msgcode, Size: uint32(len(data)), Payload: bytes.NewReader(data)})

}
