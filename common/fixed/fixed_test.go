package fixed

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
)

func TestToString(t *testing.T) {
	RegisterTestingT(t)
	Ω(MustNew("1.135").String()).To(Equal("1.135"))
}

func TestJsonMarshalling(t *testing.T) {
	RegisterTestingT(t)

	value := MustNew("1.135")

	bt, err := json.Marshal(value)
	Ω(err).ShouldNot(HaveOccurred())

	var otherValue Fixed
	Ω(json.Unmarshal(bt, &otherValue)).Should(Succeed())
	Ω(otherValue.String()).To(Equal(value.String()))

}
