package fixed

import (
	"encoding/json"
	"math/big"

	"github.com/shopspring/decimal"
)

const decimals = 24

var precision = decimal.NewFromFloat(float64(10)).Pow(decimal.NewFromFloat(decimals))

// Fixed is a decimal number with 24 decimal digits
type Fixed big.Int

// String implements fmt.Stringer
func (f *Fixed) String() string {
	amount, _ := decimal.NewFromString((*big.Int)(f).String())
	return amount.Div(precision).String()
}

// BigInt returns big.Int representation of Fixed
func (f *Fixed) BigInt() *big.Int {
	return (*big.Int)(f)
}

// UnmarshalJSON implements json.Unmarshaller
func (f *Fixed) UnmarshalJSON(b []byte) error {
	var data string
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	parsed, err := ToFixed(data)
	if err != nil {
		return err
	}

	*f = *parsed
	return nil
}

// MarshalJSON implements json.Marshaller
func (f Fixed) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.String())
}

func MustNew(str string) *Fixed {
	f, err := ToFixed(str)
	if err != nil {
		panic(err)
	}
	return f
}

// ToFixed converts a string|float|int|decimal to Fixed
func ToFixed(iamount interface{}) (*Fixed, error) {
	var err error
	amount := decimal.NewFromFloat(0)
	switch v := iamount.(type) {
	case string:
		amount, err = decimal.NewFromString(v)
		if err != nil {
			return nil, err
		}
	case float64:
		amount = decimal.NewFromFloat(v)
	case int64:
		amount = decimal.NewFromFloat(float64(v))
	case decimal.Decimal:
		amount = v
	case *decimal.Decimal:
		amount = *v
	}

	result := amount.Mul(precision)

	fixed := new(big.Int)
	fixed.SetString(result.String(), 10)

	return (*Fixed)(fixed), nil
}
