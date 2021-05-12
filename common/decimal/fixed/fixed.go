package fixed

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common/decimal"
)

var precision = decimal.Precision(24)

// Fixed is a decimal number with 24 decimal digits
type Fixed big.Int

// MustNew creates an instance of Fixed from a string
func MustNew(str string) *Fixed { return (*Fixed)(decimal.MustNew(str, precision)) }

// String implements fmt.Stringer
func (v *Fixed) String() string { return decimal.String(v.BigInt(), precision) }

// BigInt returns big.Int representation
func (v *Fixed) BigInt() *big.Int { return (*big.Int)(v) }

// MarshalJSON implements json.Marshaller
func (v Fixed) MarshalJSON() ([]byte, error) { return decimal.ToJSON(v.BigInt(), precision) }

// UnmarshalJSON implements json.Unmarshaller
func (v *Fixed) UnmarshalJSON(b []byte) error {
	value, err := decimal.FromJSON(b, precision)
	if err == nil {
		*v = (Fixed)(*value)
	}
	return err
}
