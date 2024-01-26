package types

import "github.com/celo-org/celo-blockchain/common"

// IstanbulExtra returns the 'Extra' field of the header deserialized into an
// IstanbulExtra struct, if there is an error deserializing the 'Extra' field
// it will be returned.
func (h *Header) IstanbulExtra() (*IstanbulExtra, error) {
	h.extraLock.Lock()
	defer h.extraLock.Unlock()

	if h.extraValue == nil && h.extraError == nil {
		extra, err := extractIstanbulExtra(h)

		h.extraValue = extra
		h.extraError = err
	}

	return h.extraValue, h.extraError
}

// ParentOrGenesisHash returns the parent hash unless this is the genesis block
// in which case it reurns the hash of the genesis block.
func (h *Header) ParentOrGenesisHash() common.Hash {
	if h.Number.Uint64() == 0 {
		return h.Hash()
	}
	return h.ParentHash
}
