package decimal

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
)

var precision = Precision(24)

func TestToString(t *testing.T) {
	RegisterTestingT(t)
	Ω(String(MustNew("1.135", precision), precision)).To(Equal("1.135"))
}

func TestJsonMarshalling(t *testing.T) {
	RegisterTestingT(t)

	value := MustNew("1.135", precision)

	bt, err := ToJSON(value, precision)
	Ω(err).ShouldNot(HaveOccurred())

	fmt.Println((string)(bt))
	otherValue, err := FromJSON(bt, precision)
	Ω(err).Should(Succeed())
	Ω(String(otherValue, precision)).To(Equal(String(value, precision)))

}
