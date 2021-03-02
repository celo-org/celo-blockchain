package token

import (
	"encoding/json"
	"math/big"

	"github.com/shopspring/decimal"
)

const decimals = 18

var precision = decimal.NewFromFloat(float64(10)).Pow(decimal.NewFromFloat(decimals))

// Token is a decimal number with 24 decimal digits
type Token big.Int

// String implements fmt.Stringer
func (f *Token) String() string {
	amount, _ := decimal.NewFromString((*big.Int)(f).String())
	return amount.Div(precision).String()
}

// BigInt returns big.Int representation of Token
func (f *Token) BigInt() *big.Int {
	return (*big.Int)(f)
}

// UnmarshalJSON implements json.Unmarshaller
func (f *Token) UnmarshalJSON(b []byte) error {
	var data string
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	parsed, err := ToToken(data)
	if err != nil {
		return err
	}

	*f = *parsed
	return nil
}

// MarshalJSON implements json.Marshaller
func (f Token) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.String())
}

func MustNew(str string) *Token {
	f, err := ToToken(str)
	if err != nil {
		panic(err)
	}
	return f
}

// ToToken converts a string|float|int|decimal to Token
func ToToken(iamount interface{}) (*Token, error) {
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

	return (*Token)(fixed), nil
}
