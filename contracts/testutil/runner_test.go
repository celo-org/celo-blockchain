package testutil

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/contracts/blockchain_parameters"
	. "github.com/onsi/gomega"
)

func TestRunnerWorks(t *testing.T) {
	g := NewGomegaWithT(t)

	celo := NewCeloMock()

	lw, err := blockchain_parameters.GetLookbackWindow(celo.Runner)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(lw).To((Equal(uint64(3))))

	celo.BlockchainParameters.LookbackWindow = big.NewInt(10)

	lw, err = blockchain_parameters.GetLookbackWindow(celo.Runner)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(lw).To((Equal(uint64(10))))
}
