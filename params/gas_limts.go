package params

import "math/big"

type GasLimits struct {
	// changes holds all gas limit changes, it is assumed that the first change ocurrs at block 0.
	changes []LimitChange
}

type LimitChange struct {
	block    *big.Int
	gasLimit uint64
}

func (g *GasLimits) Limit(block *big.Int) uint64 {
	// Grab the gas limit at block 0
	curr := g.changes[0].gasLimit
	for _, c := range g.changes[1:] {
		if block.Cmp(c.block) < 0 {
			return curr
		}
		curr = c.gasLimit
	}
	return curr
}
