package env

// CoinType represents the coin_type based on BIP-44
// Refer to https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki#path-levels
type CoinType int

// https://github.com/satoshilabs/slips/blob/master/slip-0044.md
// This number is not duplicated in SLIP-0044 and copied from `ChainID` in `cmd/mycelo/templates.go`
var MyceloCT CoinType = 9099000

func (c CoinType) Int() int {
	return int(c)
}
