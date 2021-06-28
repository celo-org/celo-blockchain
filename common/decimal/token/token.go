package token

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/decimal"
)

var precision = decimal.Precision(18)

// Token is a decimal number with 18 decimal digits
type Token big.Int

// MustNew creates an instance of Token from a string
func MustNew(str string) *Token { return (*Token)(decimal.MustNew(str, precision)) }

// String implements fmt.Stringer
func (t *Token) String() string { return decimal.String(t.BigInt(), precision) }

// BigInt returns big.Int representation
func (t *Token) BigInt() *big.Int { return (*big.Int)(t) }

// MarshalJSON implements json.Marshaller
func (t Token) MarshalJSON() ([]byte, error) { return decimal.ToJSON(t.BigInt(), precision) }

// UnmarshalJSON implements json.Unmarshaller
func (t *Token) UnmarshalJSON(b []byte) error {
	value, err := decimal.FromJSON(b, precision)
	if err == nil {
		*t = (Token)(*value)
	}
	return err
}
