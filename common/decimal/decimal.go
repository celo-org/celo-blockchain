package decimal

import (
	"encoding/json"
	"math/big"

	dec "github.com/shopspring/decimal"
)

// Precision creates precision constant required for decimal type
func Precision(digits int64) dec.Decimal {
	return dec.NewFromInt(10).Pow(dec.NewFromInt(digits))
}

// String returns big.Int string representation as a number with fixed decimals
func String(value *big.Int, precision dec.Decimal) string {
	amount, _ := dec.NewFromString(value.String())
	return amount.Div(precision).String()
}

// FromJSON unnmarshal JSON string a big.Int with fixed precision
func FromJSON(b []byte, precision dec.Decimal) (*big.Int, error) {
	var data string
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return New(data, precision)
}

// ToJSON returns json marshalling as decimal number
func ToJSON(value *big.Int, precision dec.Decimal) ([]byte, error) {
	return json.Marshal(String(value, precision))
}

// New creates a new Decimal from a string|float|int|decimal
func New(iamount interface{}, precision dec.Decimal) (*big.Int, error) {
	var err error
	amount := dec.NewFromFloat(0)
	switch v := iamount.(type) {
	case string:
		amount, err = dec.NewFromString(v)
		if err != nil {
			return nil, err
		}
	case float64:
		amount = dec.NewFromFloat(v)
	case int64:
		amount = dec.NewFromFloat(float64(v))
	case dec.Decimal:
		amount = v
	case *dec.Decimal:
		amount = *v
	}

	result := amount.Mul(precision)

	value := new(big.Int)
	value.SetString(result.String(), 10)

	return value, nil
}

// MustNew New variant that panics on error
func MustNew(iamount interface{}, precision dec.Decimal) *big.Int {
	value, err := New(iamount, precision)
	if err != nil {
		panic(err)
	}
	return value
}
