package main

import (
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	cli "gopkg.in/urfave/cli.v1"
)

var (
	signerCommand = cli.Command{
		Action:    startSigner,
		Name:      "signer",
		Usage:     "Start node in signer-only mode",
		ArgsUsage: "",
		Flags:     []cli.Flag{},
		Category:  "BLOCKCHAIN COMMANDS",
		Description: `
The signer command starts the node in a signer only mode which
only exposes RPCs related to transaction signing and does
not connect to the network.
`,
	}
)

func startSigner(ctx *cli.Context) error {
	stack := makeSignerNode(ctx)
	defer stack.Close()
	startSignerNode(ctx, stack)
	stack.Wait()
	return nil
}

// startSignerNode boots up the system node in signer mode.
// It only unlocks any requested accounts, and starts the RPC/IPC interfaces
// All other protocols/subsystems are NOT initialized
func startSignerNode(ctx *cli.Context, stack *node.Node) {
	debug.Memsize.Add("node", stack)
	// Start up the node itself
	utils.StartNode(stack)
	// Unlock any account specifically requested
	unlockAccounts(ctx, stack)

	// Register wallet event handlers to open and auto-derive wallets
	events := make(chan accounts.WalletEvent, 16)
	stack.AccountManager().Subscribe(events)

	// Create a client to interact with local geth node.
	rpcClient, err := stack.Attach()
	if err != nil {
		utils.Fatalf("Failed to attach to self: %v", err)
	}
	ethClient := ethclient.NewClient(rpcClient)

	go func() {
		// Open any wallets already attached
		for _, wallet := range stack.AccountManager().Wallets() {
			if err := wallet.Open(""); err != nil {
				log.Warn("Failed to open wallet", "url", wallet.URL(), "err", err)
			}
		}
		// Listen for wallet event till termination
		for event := range events {
			switch event.Kind {
			case accounts.WalletArrived:
				if err := event.Wallet.Open(""); err != nil {
					log.Warn("New wallet appeared, failed to open", "url", event.Wallet.URL(), "err", err)
				}
			case accounts.WalletOpened:
				status, _ := event.Wallet.Status()
				log.Info("New wallet appeared", "url", event.Wallet.URL(), "status", status)

				var derivationPaths []accounts.DerivationPath
				if event.Wallet.URL().Scheme == "ledger" {
					derivationPaths = append(derivationPaths, accounts.LegacyLedgerBaseDerivationPath)
				}
				derivationPaths = append(derivationPaths, accounts.DefaultBaseDerivationPath)

				event.Wallet.SelfDerive(derivationPaths, ethClient)

			case accounts.WalletDropped:
				log.Info("Old wallet dropped", "url", event.Wallet.URL())
				event.Wallet.Close()
			}
		}
	}()
}
