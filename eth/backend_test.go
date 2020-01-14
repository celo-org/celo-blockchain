package eth

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/node"
)

const (
	testInstance         = "validator-tester"
	testEtherbaseAddress = "0x8605cdbbdb6d264aa742e77020dcbc58fcdce182"
	testValidatorAddress = "0x8605cdbbdb6d264aa742e77020dcbc58fcdce183"
)

type tester struct {
	workspace string
	stack     *node.Node
	ethereum  *Ethereum
}

// newTester creates a test environment based on which the console can operate.
// Please ensure you call Close() on the returned tester to avoid leaks.
func newTester(t *testing.T, confOverride func(*Config)) *tester {
	// Create a temporary storage for the node keys and initialize it
	workspace, err := ioutil.TempDir("", "validator-tester-")
	if err != nil {
		t.Fatalf("failed to create temporary keystore: %v", err)
	}

	// Create a networkless protocol stack and start an Ethereum service within
	stack, err := node.New(&node.Config{DataDir: workspace, UseLightweightKDF: true, Name: testInstance})
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	ethConf := &Config{
		Genesis:   core.DeveloperGenesisBlock(15, common.Address{}),
		Etherbase: common.HexToAddress(testEtherbaseAddress),
		Ethash: ethash.Config{
			PowMode: ethash.ModeTest,
		},
	}
	if confOverride != nil {
		confOverride(ethConf)
	}
	if err = stack.Register(func(ctx *node.ServiceContext) (node.Service, error) { return New(ctx, ethConf) }); err != nil {
		t.Fatalf("failed to register Ethereum protocol: %v", err)
	}
	// Start the node and assemble the JavaScript console around it
	if err = stack.Start(); err != nil {
		t.Fatalf("failed to start test stack: %v", err)
	}

	// Create the final tester and return
	var ethereum *Ethereum
	stack.Service(&ethereum)

	return &tester{
		workspace: workspace,
		stack:     stack,
		ethereum:  ethereum,
	}
}

// Close cleans up any temporary data folders and held resources.
func (env *tester) Close(t *testing.T) {
	os.RemoveAll(env.workspace)
}

func TestValidatorSign(t *testing.T) {
	tester := newTester(t, func(c *Config) {
		c.Validator = common.HexToAddress(testValidatorAddress)
	})
	defer tester.Close(t)
	tester.ethereum.StartMining(1)
	block1 := tester.ethereum.blockchain.CurrentBlock()
  t.Logf("HEre 1 %v", block1.NumberU64())
  tester.ethereum.engine.Seal()
	time.Sleep(10 * time.Second)
	block2 := tester.ethereum.miner.PendingBlock()
	// block2 := tester.ethereum.blockchain.CurrentBlock()
	t.Logf("HEre 2 %v", block2.)
}

func TestDefaultSign(t *testing.T) {
	tester := newTester(t, nil)
	defer tester.Close(t)
	tester.ethereum.StartMining(1)

}
