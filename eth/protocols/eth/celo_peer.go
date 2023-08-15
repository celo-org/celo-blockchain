package eth

import (
	"bytes"

	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/rlp"
)

// Send writes an RLP-encoded message with the given code.
// data should encode as an RLP list.
func (p *Peer) Send(msgcode uint64, data []byte) error {
	raw, err := rlp.EncodeToBytes(data)
	if err != nil {
		return err
	}
	return p.SendDoubleEncoded(msgcode, raw)
}

// SendDoubleEncoded sends an rlp encoded (twice) message with the given code.
func (p *Peer) SendDoubleEncoded(msgcode uint64, data []byte) error {
	return p.rw.WriteMsg(p2p.Msg{Code: msgcode, Size: uint32(len(data)), Payload: bytes.NewReader(data)})

}
