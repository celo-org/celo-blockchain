package bigintstr

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/decimal"
)

// Since this is just a wrapper for big.Int, it's precision is 1 (10^0)
// We could created a wrapper without using `decimal`, but this lets us reuse some code.
var precision = decimal.Precision(0)

// BigIntStr is a wrapper for big.Int which is marshaled to a string
type BigIntStr big.Int

// MustNew creates an instance of Fixed from a string
func MustNew(str string) *BigIntStr { return (*BigIntStr)(decimal.MustNew(str, precision)) }

// String implements fmt.Stringer
func (v *BigIntStr) String() string { return decimal.String(v.BigInt(), precision) }

// BigInt returns big.Int representation
func (v *BigIntStr) BigInt() *big.Int { return (*big.Int)(v) }

// MarshalJSON implements json.Marshaller
func (v BigIntStr) MarshalJSON() ([]byte, error) { return decimal.ToJSON(v.BigInt(), precision) }

// UnmarshalJSON implements json.Unmarshaller
func (v *BigIntStr) UnmarshalJSON(b []byte) error {
	value, err := decimal.FromJSON(b, precision)
	if err == nil {
		*v = (BigIntStr)(*value)
	}
	return err
}
