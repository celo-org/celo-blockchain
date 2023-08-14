package core

import (
	"fmt"
	"math"
)

// BytesBlock tracks the amount of bytes available during execution of the transactions
// in a block. The zero value is a block with zero bytes available.
type BytesBlock uint64

// AddBytes makes bytes available to use.
func (bp *BytesBlock) AddBytes(amount uint64) *BytesBlock {
	if uint64(*bp) > math.MaxUint64-amount {
		panic("block's bytes pushed above uint64")
	}
	*(*uint64)(bp) += amount
	return bp
}

// SubBytes deducts the given amount from the block if enough bytes are
// available and returns an error otherwise.
func (bp *BytesBlock) SubBytes(amount uint64) error {
	if uint64(*bp) < amount {
		return ErrGasLimitReached
	}
	*(*uint64)(bp) -= amount
	return nil
}

// BytesLeft returns the amount of gas remaining in the pool.
func (bp *BytesBlock) BytesLeft() uint64 {
	return uint64(*bp)
}

func (bp *BytesBlock) String() string {
	return fmt.Sprintf("%d", *bp)
}
