package bls12377

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

func bigFromHex(hex string) *big.Int {
	return new(big.Int).SetBytes(common.FromHex(hex))
}

// decodeFieldElement expects 64 byte input with zero top 16 bytes,
// returns lower 48 bytes.
func decodeFieldElement(in []byte) ([]byte, error) {
	if len(in) != ENCODED_FIELD_ELEMENT_SIZE {
		return nil, errors.New("invalid field element length")
	}
	// check top bytes
	for i := 0; i < ENCODED_FIELD_ELEMENT_SIZE-FE_BYTE_SIZE; i++ {
		if in[i] != byte(0x00) {
			return nil, errors.New("invalid field element top bytes")
		}
	}
	out := make([]byte, FE_BYTE_SIZE)
	copy(out[:], in[ENCODED_FIELD_ELEMENT_SIZE-FE_BYTE_SIZE:])
	return out, nil
}
