package nat

import (
	"fmt"
)

func MarshalNat(n Interface) (string, error) {
	if n == nil {
		return "", nil
	}
	// See nat.Parse doc for accepted formats
	switch t := n.(type) {
	case *autodisc:
		return "any", nil
	case *pmp:
		return fmt.Sprintf("pmp:%v", t.gw), nil
	case *upnp:
		return "upnp", nil
	case ExtIP:
		text, err := t.MarshalText()
		return string(text), err

	default:
		panic(fmt.Sprintf("unrecognised nat implementation %T", n))
	}
}

func UnmarshalNat(n string) (Interface, error) {
	return Parse(n)
}
