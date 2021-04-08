package currency

import (
	"errors"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	. "github.com/onsi/gomega"
)

type getExchangeRateMock struct {
	calls   []*common.Address
	returns []struct {
		rate *exchangeRate
		err  error
	}
	returnIdx int
}

func (m *getExchangeRateMock) totalCalls() int {
	return len(m.calls)
}

func (m *getExchangeRateMock) getExchangeRate(currency *common.Address) (*exchangeRate, error) {
	m.calls = append(m.calls, currency)

	if len(m.returns) <= m.returnIdx {
		return nil, errors.New("mock: missing return info")
	}

	ret := m.returns[m.returnIdx]
	m.returnIdx++
	return ret.rate, ret.err
}

func (m *getExchangeRateMock) nextReturn(rate *exchangeRate, err error) {
	m.returns = append(m.returns, struct {
		rate *exchangeRate
		err  error
	}{
		rate, err,
	})
}

func TestCurrencyComparator(t *testing.T) {
	twoToOne := exchangeRate{
		Numerator: common.Big1, Denominator: common.Big2,
	}
	oneToTwo := exchangeRate{
		Numerator: common.Big2, Denominator: common.Big1,
	}

	t.Run("should not call getExchange rate if both currencies are gold", func(t *testing.T) {
		g := NewGomegaWithT(t)
		mock := getExchangeRateMock{}
		comparator := newComparator(mock.getExchangeRate)

		g.Expect(comparator.Cmp(common.Big1, nil, common.Big2, nil)).To(Equal(-1))
		// no call to getExchange Rate
		g.Expect(mock.totalCalls()).To(BeZero())
	})

	t.Run("should not call getExchange rate if both currencies are the same", func(t *testing.T) {
		g := NewGomegaWithT(t)
		mock := getExchangeRateMock{}
		comparator := newComparator(mock.getExchangeRate)

		g.Expect(comparator.Cmp(common.Big1, &common.Address{12}, common.Big2, &common.Address{12})).To(Equal(-1))
		// no call to getExchange Rate
		g.Expect(mock.totalCalls()).To(BeZero())
	})

	t.Run("should not call getExchange rate on goldToken currency", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}

		mock.nextReturn(&twoToOne, nil)

		comparator := newComparator(mock.getExchangeRate)

		g.Expect(comparator.Cmp(common.Big1, nil, common.Big1, &common.Address{12})).To(Equal(-1))
		// call to the exchange rate only for non goldToken currency
		g.Expect(mock.totalCalls()).To(Equal(1))
		g.Expect(*mock.calls[0]).To(Equal(common.Address{12}))
	})

	t.Run("should use returned exchange rate", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}
		comparator := newComparator(mock.getExchangeRate)

		// case 1: 2 gold = 1 usd
		// then 1 gold < 1 usd
		mock.nextReturn(&twoToOne, nil)
		g.Expect(comparator.Cmp(common.Big1, nil, common.Big1, &common.Address{10})).To(Equal(-1))

		// case 2: 1 gold = 2 usd
		// then 1 gold > 1 usd
		mock.nextReturn(&oneToTwo, nil)
		g.Expect(comparator.Cmp(common.Big1, nil, common.Big1, &common.Address{20})).To(Equal(1))

		// case 3: 1 gold = 2 usd && 1 gold = 2 eur
		// then 2 eur > 1 usd
		mock.nextReturn(&oneToTwo, nil)
		mock.nextReturn(&oneToTwo, nil)
		g.Expect(comparator.Cmp(common.Big2, &common.Address{30}, common.Big1, &common.Address{40})).To(Equal(1))
	})

	t.Run("should work with zero values", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}
		comparator := newComparator(mock.getExchangeRate)

		// case 1: both values == 0
		g.Expect(comparator.Cmp(common.Big0, nil, common.Big0, nil)).To(Equal(0))

		// case 2: first value == 0
		g.Expect(comparator.Cmp(common.Big0, nil, common.Big1, nil)).To(Equal(-1))

		// case 3: second value == 0
		g.Expect(comparator.Cmp(common.Big1, nil, common.Big0, nil)).To(Equal(1))
	})

	t.Run("should compare value if first get exchange rate fails", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}
		mock.nextReturn(&twoToOne, nil)
		mock.nextReturn(nil, errors.New("boom!"))

		comparator := newComparator(mock.getExchangeRate)
		g.Expect(comparator.Cmp(common.Big2, &common.Address{30}, common.Big1, &common.Address{12})).To(Equal(1))
	})

	t.Run("should compare value if second get exchange rate fails", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}
		mock.nextReturn(nil, errors.New("boom!"))
		mock.nextReturn(&twoToOne, nil)

		comparator := newComparator(mock.getExchangeRate)
		g.Expect(comparator.Cmp(common.Big2, &common.Address{30}, common.Big1, &common.Address{12})).To(Equal(1))
	})

	t.Run("should cache exchange rate on subsequent calls", func(t *testing.T) {
		g := NewGomegaWithT(t)

		mock := getExchangeRateMock{}
		mock.nextReturn(&twoToOne, nil)
		mock.nextReturn(&oneToTwo, nil)

		comparator := newComparator(mock.getExchangeRate)

		for i := 0; i < 10; i++ {
			g.Expect(comparator.Cmp(common.Big1, &common.Address{30}, common.Big1, &common.Address{12})).To(Equal(1))
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

		comparator := newComparator(mock.getExchangeRate)

		for i := 0; i < 10; i++ {
			g.Expect(comparator.Cmp(common.Big1, &common.Address{30}, common.Big1, &common.Address{12})).To(Equal(0))
		}

		// expect 10 call for address{30} and 10 for address{12}
		g.Expect(mock.totalCalls()).To(Equal(20))
	})

}
