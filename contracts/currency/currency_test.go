package currency

import (
	"errors"
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/vm"
	. "github.com/onsi/gomega"
)

type getExchangeRateMock struct {
	calls   []*common.Address
	returns []struct {
		rate *ExchangeRate
		err  error
	}
	returnIdx int
}

func (m *getExchangeRateMock) totalCalls() int {
	return len(m.calls)
}

func (m *getExchangeRateMock) getExchangeRate(vmRunner vm.EVMRunner, currency *common.Address) (*ExchangeRate, error) {
	m.calls = append(m.calls, currency)

	if len(m.returns) <= m.returnIdx {
		return nil, errors.New("mock: missing return info")
	}

	ret := m.returns[m.returnIdx]
	m.returnIdx++
	return ret.rate, ret.err
}

func (m *getExchangeRateMock) nextReturn(rate *ExchangeRate, err error) {
	m.returns = append(m.returns, struct {
		rate *ExchangeRate
		err  error
	}{
		rate, err,
	})
}

func TestCurrencyManager(t *testing.T) {
	twoToOne := MustNewExchangeRate(common.Big1, common.Big2)
	oneToTwo := MustNewExchangeRate(common.Big2, common.Big1)

	t.Run("should not call getExchange rate if both currencies are gold", func(t *testing.T) {
		g := NewGomegaWithT(t)
		mock := getExchangeRateMock{}
		manager := newManager(mock.getExchangeRate, nil)

		g.Expect(manager.CmpValues(common.Big1, nil, common.Big2, nil)).To(Equal(-1))
		// no call to getExchange Rate
		g.Expect(mock.totalCalls()).To(BeZero())
	})

	t.Run("should not call getExchange rate if both currencies are the same", func(t *testing.T) {
		g := NewGomegaWithT(t)
		mock := getExchangeRateMock{}
		manager := newManager(mock.getExchangeRate, nil)

		g.Expect(manager.CmpValues(common.Big1, &common.Address{12}, common.Big2, &common.Address{12})).To(Equal(-1))
		// no call to getExchange Rate
		g.Expect(mock.totalCalls()).To(BeZero())
	})

	t.Run("should not call getExchange rate on goldToken currency", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}

		mock.nextReturn(twoToOne, nil)

		manager := newManager(mock.getExchangeRate, nil)

		g.Expect(manager.CmpValues(common.Big1, nil, common.Big1, &common.Address{12})).To(Equal(-1))
		// call to the exchange rate only for non goldToken currency
		g.Expect(mock.totalCalls()).To(Equal(1))
		g.Expect(*mock.calls[0]).To(Equal(common.Address{12}))
	})

	t.Run("should use returned exchange rate", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}
		manager := newManager(mock.getExchangeRate, nil)

		// case 1: 2 gold = 1 usd
		// then 1 gold < 1 usd
		mock.nextReturn(twoToOne, nil)
		g.Expect(manager.CmpValues(common.Big1, nil, common.Big1, &common.Address{10})).To(Equal(-1))

		// case 2: 1 gold = 2 usd
		// then 1 gold > 1 usd
		mock.nextReturn(oneToTwo, nil)
		g.Expect(manager.CmpValues(common.Big1, nil, common.Big1, &common.Address{20})).To(Equal(1))

		// case 3: 1 gold = 2 usd && 1 gold = 2 eur
		// then 2 eur > 1 usd
		mock.nextReturn(oneToTwo, nil)
		mock.nextReturn(oneToTwo, nil)
		g.Expect(manager.CmpValues(common.Big2, &common.Address{30}, common.Big1, &common.Address{40})).To(Equal(1))
	})

	t.Run("should work with zero values", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}
		manager := newManager(mock.getExchangeRate, nil)

		// case 1: both values == 0
		g.Expect(manager.CmpValues(common.Big0, nil, common.Big0, nil)).To(Equal(0))

		// case 2: first value == 0
		g.Expect(manager.CmpValues(common.Big0, nil, common.Big1, nil)).To(Equal(-1))

		// case 3: second value == 0
		g.Expect(manager.CmpValues(common.Big1, nil, common.Big0, nil)).To(Equal(1))
	})

	t.Run("should compare value if first get exchange rate fails", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}
		mock.nextReturn(twoToOne, nil)
		mock.nextReturn(nil, errors.New("boom!"))

		manager := newManager(mock.getExchangeRate, nil)
		g.Expect(manager.CmpValues(common.Big2, &common.Address{30}, common.Big1, &common.Address{12})).To(Equal(1))
	})

	t.Run("should compare value if second get exchange rate fails", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}
		mock.nextReturn(nil, errors.New("boom!"))
		mock.nextReturn(twoToOne, nil)

		manager := newManager(mock.getExchangeRate, nil)
		g.Expect(manager.CmpValues(common.Big2, &common.Address{30}, common.Big1, &common.Address{12})).To(Equal(1))
	})

	t.Run("should cache exchange rate on subsequent calls", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}
		mock.nextReturn(twoToOne, nil)
		mock.nextReturn(oneToTwo, nil)

		manager := newManager(mock.getExchangeRate, nil)

		for i := 0; i < 10; i++ {
			g.Expect(manager.CmpValues(common.Big1, &common.Address{30}, common.Big1, &common.Address{12})).To(Equal(1))
		}

		// call to the exchange rate only for non goldToken currency
		g.Expect(mock.totalCalls()).To(Equal(2))
		g.Expect(*mock.calls[0]).To(Equal(common.Address{30}))
		g.Expect(*mock.calls[1]).To(Equal(common.Address{12}))
	})

	t.Run("should NOT cache exchange rate on errors", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}
		// default return is an error

		manager := newManager(mock.getExchangeRate, nil)

		for i := 0; i < 10; i++ {
			g.Expect(manager.CmpValues(common.Big1, &common.Address{30}, common.Big1, &common.Address{12})).To(Equal(0))
		}

		// expect 10 call for address{30} and 10 for address{12}
		g.Expect(mock.totalCalls()).To(Equal(20))
	})

}

// MustNewExchangeRate creates an exchange rate, panic on error
func MustNewExchangeRate(numerator *big.Int, denominator *big.Int) *ExchangeRate {
	rate, err := NewExchangeRate(numerator, denominator)
	if err != nil {
		panic(err)
	}
	return rate
}

func EqualBigInt(n int64) OmegaMatcher {
	return WithTransform(func(b *big.Int) int64 { return b.Int64() }, Equal(n))
}

func TestExchangeRate(t *testing.T) {

	t.Run("can't create with numerator <= 0", func(t *testing.T) {
		g := NewGomegaWithT(t)

		_, err := NewExchangeRate(common.Big0, common.Big1)
		g.Expect(err).Should((HaveOccurred()))

		_, err = NewExchangeRate(big.NewInt(-1), common.Big1)
		g.Expect(err).Should((HaveOccurred()))
	})

	t.Run("can't create with denominator <= 0", func(t *testing.T) {
		g := NewGomegaWithT(t)

		_, err := NewExchangeRate(common.Big1, common.Big0)
		g.Expect(err).Should((HaveOccurred()))

		_, err = NewExchangeRate(common.Big1, big.NewInt(-1))
		g.Expect(err).Should((HaveOccurred()))
	})

	t.Run("should convert to base and back", func(t *testing.T) {
		g := NewGomegaWithT(t)
		twoToOne := MustNewExchangeRate(common.Big2, common.Big1)

		g.Expect(twoToOne.FromBase(common.Big1)).Should(EqualBigInt(2))
		g.Expect(twoToOne.ToBase(common.Big2)).Should(EqualBigInt(1))
	})

}

func TestCurrency(t *testing.T) {

	t.Run("should compare with another currency values", func(t *testing.T) {
		g := NewGomegaWithT(t)

		// 1 gold => 2 expensiveToken
		expensiveToken := MustNewExchangeRate(common.Big2, common.Big1)
		// 1 gold => 5 cheapToken
		cheapToken := MustNewExchangeRate(big.NewInt(5), common.Big1)

		expensiveCurrency := Currency{
			Address:    common.HexToAddress("0x1"),
			toCELORate: *expensiveToken,
		}
		cheapCurrency := Currency{
			Address:    common.HexToAddress("0x2"),
			toCELORate: *cheapToken,
		}

		g.Expect(expensiveCurrency.CmpToCurrency(big.NewInt(10), big.NewInt(10), &cheapCurrency)).Should(Equal(1))
	})
}
