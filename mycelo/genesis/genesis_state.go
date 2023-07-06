package genesis

import (
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/decimal/token"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/core/vm/runtime"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/mycelo/contract"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/shopspring/decimal"
)

var (
	proxyOwnerStorageLocation = common.HexToHash("0xb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d6103")
	proxyByteCode             = common.Hex2Bytes("60806040526004361061004a5760003560e01c806303386ba3146101e757806342404e0714610280578063bb913f41146102d7578063d29d44ee14610328578063f7e6af8014610379575b6000600160405180807f656970313936372e70726f78792e696d706c656d656e746174696f6e00000000815250601c019050604051809103902060001c0360001b9050600081549050600073ffffffffffffffffffffffffffffffffffffffff168173ffffffffffffffffffffffffffffffffffffffff161415610136576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260158152602001807f4e6f20496d706c656d656e746174696f6e20736574000000000000000000000081525060200191505060405180910390fd5b61013f816103d0565b6101b1576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260188152602001807f496e76616c696420636f6e74726163742061646472657373000000000000000081525060200191505060405180910390fd5b60405136810160405236600082376000803683855af43d604051818101604052816000823e82600081146101e3578282f35b8282fd5b61027e600480360360408110156101fd57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291908035906020019064010000000081111561023a57600080fd5b82018360208201111561024c57600080fd5b8035906020019184600183028401116401000000008311171561026e57600080fd5b909192939192939050505061041b565b005b34801561028c57600080fd5b506102956105c1565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b3480156102e357600080fd5b50610326600480360360208110156102fa57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061060d565b005b34801561033457600080fd5b506103776004803603602081101561034b57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506107bd565b005b34801561038557600080fd5b5061038e610871565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b60008060007fc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47060001b9050833f915080821415801561041257506000801b8214155b92505050919050565b610423610871565b73ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff16146104c3576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260148152602001807f73656e64657220776173206e6f74206f776e657200000000000000000000000081525060200191505060405180910390fd5b6104cc8361060d565b600060608473ffffffffffffffffffffffffffffffffffffffff168484604051808383808284378083019250505092505050600060405180830381855af49150503d8060008114610539576040519150601f19603f3d011682016040523d82523d6000602084013e61053e565b606091505b508092508193505050816105ba576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601e8152602001807f696e697469616c697a6174696f6e2063616c6c6261636b206661696c6564000081525060200191505060405180910390fd5b5050505050565b600080600160405180807f656970313936372e70726f78792e696d706c656d656e746174696f6e00000000815250601c019050604051809103902060001c0360001b9050805491505090565b610615610871565b73ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff16146106b5576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260148152602001807f73656e64657220776173206e6f74206f776e657200000000000000000000000081525060200191505060405180910390fd5b6000600160405180807f656970313936372e70726f78792e696d706c656d656e746174696f6e00000000815250601c019050604051809103902060001c0360001b9050610701826103d0565b610773576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260188152602001807f496e76616c696420636f6e74726163742061646472657373000000000000000081525060200191505060405180910390fd5b8181558173ffffffffffffffffffffffffffffffffffffffff167fab64f92ab780ecbf4f3866f57cee465ff36c89450dcce20237ca7a8d81fb7d1360405160405180910390a25050565b6107c5610871565b73ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff1614610865576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260148152602001807f73656e64657220776173206e6f74206f776e657200000000000000000000000081525060200191505060405180910390fd5b61086e816108bd565b50565b600080600160405180807f656970313936372e70726f78792e61646d696e000000000000000000000000008152506013019050604051809103902060001c0360001b9050805491505090565b600073ffffffffffffffffffffffffffffffffffffffff168173ffffffffffffffffffffffffffffffffffffffff161415610960576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260118152602001807f6f776e65722063616e6e6f74206265203000000000000000000000000000000081525060200191505060405180910390fd5b6000600160405180807f656970313936372e70726f78792e61646d696e000000000000000000000000008152506013019050604051809103902060001c0360001b90508181558173ffffffffffffffffffffffffffffffffffffffff167f50146d0e3c60aa1d17a70635b05494f864e86144a2201275021014fbf08bafe260405160405180910390a2505056fea165627a7a72305820f4f741dbef8c566cb1690ae708b8ef1113bdb503225629cc1f9e86bd47efd1a40029")
	adminGoldBalance          = token.MustNew("100000").BigInt() // 100k CELO
)

// deployContext context for deployment
type deployContext struct {
	genesisConfig *Config
	accounts      *env.AccountsConfig
	statedb       *state.StateDB
	runtimeConfig *runtime.Config
	truffleReader contract.TruffleReader
	logger        log.Logger
}

// Helper function to reduce boilerplate, limited to this package on purpose
// Like big.NewInt() except it takes uint64 instead of int64
func newBigInt(x uint64) *big.Int { return new(big.Int).SetUint64(x) }

func generateGenesisState(accounts *env.AccountsConfig, cfg *Config, buildPath string) (core.GenesisAlloc, error) {
	deployment := newDeployment(cfg, accounts, buildPath)
	return deployment.deploy()
}

// NewDeployment generates a new deployment
func newDeployment(genesisConfig *Config, accounts *env.AccountsConfig, buildPath string) *deployContext {
	logger := log.New("obj", "deployment")
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)

	adminAddress := accounts.AdminAccount().Address

	logger.Info("New deployment", "admin_address", adminAddress.Hex())
	return &deployContext{
		genesisConfig: genesisConfig,
		accounts:      accounts,
		logger:        logger,
		statedb:       statedb,
		truffleReader: contract.NewTruffleReader(buildPath),
		runtimeConfig: &runtime.Config{
			ChainConfig: genesisConfig.ChainConfig(),
			Origin:      adminAddress,
			State:       statedb,
			GasLimit:    1000000000000000,
			GasPrice:    big.NewInt(0),
			Value:       big.NewInt(0),
			Time:        newBigInt(genesisConfig.GenesisTimestamp),
			Coinbase:    adminAddress,
			BlockNumber: newBigInt(0),
			EVMConfig: vm.Config{
				Tracer: nil,
				Debug:  false,
			},
		},
	}

}

// Deploy runs the deployment
func (ctx *deployContext) deploy() (core.GenesisAlloc, error) {
	ctx.fundAdminAccount()

	deploySteps := [](func() error){
		// X, Y => X is the number in this list, Y is the migration number in the protocol folder for the core contracts

		// i:00, migr:01 Libraries
		ctx.deployLibraries,

		// i:01, migr:02 Registry
		ctx.deployRegistry,

		// i:02, migr:03 Freezer
		ctx.deployFreezer,

		// i:03, migr:03 FeeCurrencyWhitelist
		ctx.deployFeeCurrencyWhitelist,

		// i:04, migr:04 GoldToken
		ctx.deployGoldToken,

		// i:05, migr:05 SortedOracles
		ctx.deploySortedOracles,

		// i:06, migr:06 GasPriceMinimum
		ctx.deployGasPriceMinimum,

		// i:07, migr:07 Reserve
		ctx.deployReserve,

		// i:08, migr:08 ReserveSpenderMultisig (requires reserve to work)
		ctx.deployReserveSpenderMultisig,

		// i:09, migr:09 StableToken, StableTokenEUR and StableTokenBRL
		ctx.deployStableTokens,

		// i:10, migr:10 Exchange, ExchangeEUR and ExchangeBRL
		ctx.deployExchanges,

		// i:11, migr:11 Accounts
		ctx.deployAccounts,

		// i:12, migr:12 LockedGold
		ctx.deployLockedGold,

		// i:13, migr:13 Validators
		ctx.deployValidators,

		// i:14, migr:14 Election
		ctx.deployElection,

		// i:15, migr:15 EpochRewards
		ctx.deployEpochRewards,

		// i:16, migr:16 Random
		ctx.deployRandom,

		// i:17, migr17 Attestations
		ctx.deployAttestations,

		// 1:18, migr:18 Escrow
		ctx.deployEscrow,

		// i:19, migr:19 BlockchainParameters
		ctx.deployBlockchainParameters,

		// i:20, migr:20 GovernanceSlasher
		ctx.deployGovernanceSlasher,

		// i:21, migr:21 DoubleSigningSlasher
		ctx.deployDoubleSigningSlasher,

		// i:22, migr:22 DowntimeSlasher
		ctx.deployDowntimeSlasher,

		// i:23, migr:23 GovernanceApproverMultiSig
		ctx.deployGovernanceApproverMultiSig,

		// i:24, migr:24 GrandaMento
		ctx.deployGrandaMento,

		// i:25, migr:25 FederatedAttestations
		ctx.deployFederatedAttestations,

		// i:26, migr:26 OdisPayment
		ctx.deployOdisPayments,

		// i:27, migr:27 Governance
		ctx.deployGovernance,

		// i:28, migr:28 Elect Validators
		ctx.electValidators,

		// i:29, migr:29 FeeHandler
		ctx.deployFeeHandler,
	}

	logger := ctx.logger.New()

	for i, step := range deploySteps {
		logger.Info("Running deploy step", "number", i)
		if err := step(); err != nil {
			return nil, fmt.Errorf("Failed deployment step %d: %w", i, err)
		}
	}

	// Flush Changes
	_, err := ctx.statedb.Commit(true)
	if err != nil {
		return nil, err
	}
	ctx.statedb.IntermediateRoot(true)

	if err = ctx.verifyState(); err != nil {
		return nil, err
	}

	dumpConfig := state.DumpConfig{
		SkipCode:          false,
		SkipStorage:       false,
		OnlyWithAddresses: true,
		Start:             nil,
		Max:               0,
	}
	dump := ctx.statedb.RawDump(&dumpConfig).Accounts
	genesisAlloc := make(map[common.Address]core.GenesisAccount)
	for acc, dumpAcc := range dump {
		var account core.GenesisAccount

		if dumpAcc.Balance != "" {
			account.Balance, _ = new(big.Int).SetString(dumpAcc.Balance, 10)
		}

		account.Code = dumpAcc.Code

		if len(dumpAcc.Storage) > 0 {
			account.Storage = make(map[common.Hash]common.Hash)
			for k, v := range dumpAcc.Storage {
				account.Storage[k] = common.HexToHash(v)
			}
		}

		genesisAlloc[acc] = account

	}

	return genesisAlloc, nil
}

// Initialize Admin
func (ctx *deployContext) fundAdminAccount() {
	ctx.statedb.SetBalance(ctx.accounts.AdminAccount().Address, new(big.Int).Set(adminGoldBalance))
}

func (ctx *deployContext) deployLibraries() error {
	for _, name := range env.Libraries() {
		bytecode := ctx.truffleReader.MustReadDeployedBytecodeFor("contracts", name)
		ctx.statedb.SetCode(env.MustLibraryAddressFor(name), bytecode)
	}
	return nil
}

// deployProxiedContract will deploy proxied contract
// It will deploy the proxy contract, the impl contract, and initialize both
func (ctx *deployContext) deployProxiedContract(subpath, name string, initialize func(contract *contract.EVMBackend) error) error {
	proxyAddress := env.MustProxyAddressFor(name)
	implAddress := env.MustImplAddressFor(name)
	bytecode := ctx.truffleReader.MustReadDeployedBytecodeFor(subpath, name)

	logger := ctx.logger.New("contract", name)
	logger.Info("Start Deploy of Proxied Contract", "proxyAddress", proxyAddress.Hex(), "implAddress", implAddress.Hex())

	logger.Info("Deploy Proxy")
	ctx.statedb.SetCode(proxyAddress, proxyByteCode)
	ctx.statedb.SetState(proxyAddress, proxyOwnerStorageLocation, ctx.accounts.AdminAccount().Address.Hash())

	logger.Info("Deploy Implementation")
	ctx.statedb.SetCode(implAddress, bytecode)

	logger.Info("Set proxy implementation")
	proxyContract := ctx.proxyContract(name)

	if err := proxyContract.SimpleCall("_setImplementation", implAddress); err != nil {
		return err
	}

	logger.Info("Initialize Contract")
	if err := initialize(ctx.contract(name)); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	return nil
}

// deployCoreContract will deploy a contract + proxy, and add it to the registry
func (ctx *deployContext) deployCoreContract(subpath, name string, initialize func(contract *contract.EVMBackend) error) error {
	if err := ctx.deployProxiedContract(subpath, name, initialize); err != nil {
		return err
	}

	proxyAddress := env.MustProxyAddressFor(name)
	ctx.logger.Info("Add entry to registry", "name", name, "address", proxyAddress)
	if err := ctx.contract("Registry").SimpleCall("setAddressFor", name, proxyAddress); err != nil {
		return err
	}

	return nil
}

func (ctx *deployContext) deployMultiSig(subpath, name string, params MultiSigParameters) (common.Address, error) {
	err := ctx.deployProxiedContract(subpath, name, func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			params.Signatories,
			newBigInt(params.NumRequiredConfirmations),
			newBigInt(params.NumInternalRequiredConfirmations),
		)
	})
	if err != nil {
		return common.ZeroAddress, err
	}
	return env.MustProxyAddressFor(name), nil
}

func (ctx *deployContext) deployReserveSpenderMultisig() error {
	multiSigAddr, err := ctx.deployMultiSig("contracts-mento", "ReserveSpenderMultiSig", ctx.genesisConfig.ReserveSpenderMultiSig)
	if err != nil {
		return err
	}

	if err := ctx.contract("Reserve").SimpleCall("addSpender", multiSigAddr); err != nil {
		return err
	}
	return nil
}

func (ctx *deployContext) deployGovernanceApproverMultiSig() error {
	_, err := ctx.deployMultiSig("contracts", "GovernanceApproverMultiSig", ctx.genesisConfig.GovernanceApproverMultiSig)
	return err
}

func (ctx *deployContext) deployGovernance() error {
	approver := ctx.accounts.AdminAccount().Address
	if ctx.genesisConfig.Governance.UseMultiSig {
		approver = env.MustProxyAddressFor("GovernanceApproverMultiSig")
	}
	err := ctx.deployCoreContract("contracts", "Governance", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			approver,
			newBigInt(ctx.genesisConfig.Governance.ConcurrentProposals),
			ctx.genesisConfig.Governance.MinDeposit,
			newBigInt(ctx.genesisConfig.Governance.QueueExpiry),
			newBigInt(ctx.genesisConfig.Governance.DequeueFrequency),
			newBigInt(ctx.genesisConfig.Governance.ReferendumStageDuration),
			newBigInt(ctx.genesisConfig.Governance.ExecutionStageDuration),
			ctx.genesisConfig.Governance.ParticipationBaseline.BigInt(),
			ctx.genesisConfig.Governance.ParticipationFloor.BigInt(),
			ctx.genesisConfig.Governance.BaselineUpdateFactor.BigInt(),
			ctx.genesisConfig.Governance.BaselineQuorumFactor.BigInt(),
		)
	})
	if err != nil {
		return err
	}
	// We are skipping two steps for now:
	// 1. Setting the governance thresholds from a constitution
	// 2. Transferring ownership of the core contracts to governance
	// While the monorepo migrations code does support them, in the configurations it's always
	// set to skip them, so we can skip supporting them until it's needed.
	return nil
}

func (ctx *deployContext) deployRegistry() error {
	return ctx.deployCoreContract("contracts", "Registry", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize")
	})
}

func (ctx *deployContext) deployBlockchainParameters() error {
	return ctx.deployCoreContract("contracts", "BlockchainParameters", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			newBigInt(ctx.genesisConfig.Blockchain.GasForNonGoldCurrencies),
			newBigInt(ctx.genesisConfig.Blockchain.BlockGasLimit),
			newBigInt(ctx.genesisConfig.Istanbul.LookbackWindow),
		)
	})
}

func (ctx *deployContext) deployFreezer() error {
	return ctx.deployCoreContract("contracts", "Freezer", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize")
	})
}

func (ctx *deployContext) deployGovernanceSlasher() error {
	err := ctx.deployCoreContract("contracts", "GovernanceSlasher", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
		)
	})
	if err != nil {
		return err
	}

	return ctx.addSlasher("GovernanceSlasher")
}

func (ctx *deployContext) addSlasher(slasherName string) error {
	ctx.logger.Info("Adding new slasher", "slasher", slasherName)
	return ctx.contract("LockedGold").SimpleCall("addSlasher", slasherName)
}

func (ctx *deployContext) deployDoubleSigningSlasher() error {
	err := ctx.deployCoreContract("contracts", "DoubleSigningSlasher", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			ctx.genesisConfig.DoubleSigningSlasher.Penalty,
			ctx.genesisConfig.DoubleSigningSlasher.Reward,
		)
	})
	if err != nil {
		return err
	}

	return ctx.addSlasher("DoubleSigningSlasher")
}

func (ctx *deployContext) deployDowntimeSlasher() error {
	err := ctx.deployCoreContract("contracts", "DowntimeSlasher", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			ctx.genesisConfig.DowntimeSlasher.Penalty,
			ctx.genesisConfig.DowntimeSlasher.Reward,
			newBigInt(ctx.genesisConfig.DowntimeSlasher.SlashableDowntime),
		)
	})
	if err != nil {
		return err
	}

	return ctx.addSlasher("DowntimeSlasher")
}

func (ctx *deployContext) deployAttestations() error {
	return ctx.deployCoreContract("contracts", "Attestations", func(contract *contract.EVMBackend) error {
		dollar := decimal.NewFromBigInt(common.Big1, int32(ctx.genesisConfig.StableToken.Decimals))
		fee := dollar.Mul(ctx.genesisConfig.Attestations.AttestationRequestFeeInDollars)
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			newBigInt(ctx.genesisConfig.Attestations.AttestationExpiryBlocks),
			newBigInt(ctx.genesisConfig.Attestations.SelectIssuersWaitBlocks),
			newBigInt(ctx.genesisConfig.Attestations.MaxAttestations),
			[]common.Address{env.MustProxyAddressFor("StableToken")},
			[]*big.Int{fee.BigInt()},
		)
	})
}

func (ctx *deployContext) deployEscrow() error {
	return ctx.deployCoreContract("contracts", "Escrow", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize")
	})
}

func (ctx *deployContext) deployFeeCurrencyWhitelist() error {
	return ctx.deployCoreContract("contracts", "FeeCurrencyWhitelist", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize")
	})
}

func (ctx *deployContext) deployGrandaMento() error {
	approver := ctx.accounts.AdminAccount().Address

	err := ctx.deployCoreContract("contracts-mento", "GrandaMento", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			approver,
			ctx.genesisConfig.GrandaMento.MaxApprovalExchangeRateChange.BigInt(),
			ctx.genesisConfig.GrandaMento.Spread.BigInt(),
			newBigInt(ctx.genesisConfig.GrandaMento.VetoPeriodSeconds),
		)
	})
	if err != nil {
		return err
	}

	ctx.logger.Info("Adding GrandaMento as a new exchange spender to the reserve", "GrandaMento", env.MustProxyAddressFor("GrandaMento"))
	ctx.contract("Reserve").SimpleCall("addExchangeSpender", env.MustProxyAddressFor("GrandaMento"))

	for _, exchangeLimit := range ctx.genesisConfig.GrandaMento.StableTokenExchangeLimits {
		err = ctx.contract("GrandaMento").SimpleCall("setStableTokenExchangeLimits", exchangeLimit.StableToken, exchangeLimit.MinExchangeAmount, exchangeLimit.MaxExchangeAmount)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ctx *deployContext) deployFeeHandler() error {
	return ctx.deployCoreContract("contracts", "FeeHandler", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			ctx.genesisConfig.FeeHandler.NewFeeBeneficiary,
			ctx.genesisConfig.FeeHandler.NewBurnFraction.BigInt(),
			ctx.genesisConfig.FeeHandler.Tokens,
			ctx.genesisConfig.FeeHandler.Handlers,
			ctx.genesisConfig.FeeHandler.NewLimits,
			ctx.genesisConfig.FeeHandler.NewMaxSlippages,
		)
	})
}

func (ctx *deployContext) deployFederatedAttestations() error {
	return ctx.deployCoreContract("contracts", "FederatedAttestations", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize")
	})
}

func (ctx *deployContext) deployOdisPayments() error {
	return ctx.deployCoreContract("contracts", "OdisPayments", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize")
	})
}

func (ctx *deployContext) deployGoldToken() error {
	err := ctx.deployCoreContract("contracts", "GoldToken", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize", env.MustProxyAddressFor("Registry"))
	})
	if err != nil {
		return err
	}

	if ctx.genesisConfig.GoldToken.Frozen {
		ctx.logger.Info("Freezing GoldToken")
		err = ctx.contract("Freezer").SimpleCall("freeze", env.MustProxyAddressFor("GoldToken"))
		if err != nil {
			return err
		}
	}

	for _, bal := range ctx.genesisConfig.GoldToken.InitialBalances {
		ctx.statedb.SetBalance(bal.Account, bal.Amount)
	}

	return nil
}

func (ctx *deployContext) deployExchanges() error {
	type ExchangeConfig struct {
		contract            string
		stableTokenContract string
		cfg                 ExchangeParameters
	}
	exchanges := []ExchangeConfig{
		{"Exchange", "StableToken", ctx.genesisConfig.Exchange},
		{"ExchangeEUR", "StableTokenEUR", ctx.genesisConfig.ExchangeEUR},
		{"ExchangeBRL", "StableTokenBRL", ctx.genesisConfig.ExchangeBRL},
	}
	for _, exchange := range exchanges {
		err := ctx.deployCoreContract("contracts-mento", exchange.contract, func(contract *contract.EVMBackend) error {
			return contract.SimpleCall("initialize",
				env.MustProxyAddressFor("Registry"),
				exchange.stableTokenContract,
				exchange.cfg.Spread.BigInt(),
				exchange.cfg.ReserveFraction.BigInt(),
				newBigInt(exchange.cfg.UpdateFrequency),
				newBigInt(exchange.cfg.MinimumReports),
			)
		})
		if err != nil {
			return err
		}

		if exchange.cfg.Frozen {
			ctx.logger.Info("Freezing Exchange", "contract", exchange.contract)
			err = ctx.contract("Freezer").SimpleCall("freeze", env.MustProxyAddressFor("Exchange"))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (ctx *deployContext) deployEpochRewards() error {
	err := ctx.deployCoreContract("contracts", "EpochRewards", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			ctx.genesisConfig.EpochRewards.TargetVotingYieldInitial.BigInt(),
			ctx.genesisConfig.EpochRewards.TargetVotingYieldMax.BigInt(),
			ctx.genesisConfig.EpochRewards.TargetVotingYieldAdjustmentFactor.BigInt(),
			ctx.genesisConfig.EpochRewards.RewardsMultiplierMax.BigInt(),
			ctx.genesisConfig.EpochRewards.RewardsMultiplierAdjustmentFactorsUnderspend.BigInt(),
			ctx.genesisConfig.EpochRewards.RewardsMultiplierAdjustmentFactorsOverspend.BigInt(),
			ctx.genesisConfig.EpochRewards.TargetVotingGoldFraction.BigInt(),
			ctx.genesisConfig.EpochRewards.MaxValidatorEpochPayment,
			ctx.genesisConfig.EpochRewards.CommunityRewardFraction.BigInt(),
			ctx.genesisConfig.EpochRewards.CarbonOffsettingPartner,
			ctx.genesisConfig.EpochRewards.CarbonOffsettingFraction.BigInt(),
		)
	})
	if err != nil {
		return err
	}

	if ctx.genesisConfig.EpochRewards.Frozen {
		ctx.logger.Info("Freezing EpochRewards")
		err = ctx.contract("Freezer").SimpleCall("freeze", env.MustProxyAddressFor("EpochRewards"))
		if err != nil {
			return err
		}
	}
	return nil
}

func (ctx *deployContext) deployAccounts() error {
	return ctx.deployCoreContract("contracts", "Accounts", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize", env.MustProxyAddressFor("Registry"))
	})
}

func (ctx *deployContext) deployRandom() error {
	return ctx.deployCoreContract("contracts", "Random", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			newBigInt(ctx.genesisConfig.Random.RandomnessBlockRetentionWindow),
		)
	})
}

func (ctx *deployContext) deployLockedGold() error {
	return ctx.deployCoreContract("contracts", "LockedGold", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			newBigInt(ctx.genesisConfig.LockedGold.UnlockingPeriod),
		)
	})
}

func (ctx *deployContext) deployValidators() error {
	return ctx.deployCoreContract("contracts", "Validators", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			ctx.genesisConfig.Validators.GroupLockedGoldRequirements.Value,
			newBigInt(ctx.genesisConfig.Validators.GroupLockedGoldRequirements.Duration),
			ctx.genesisConfig.Validators.ValidatorLockedGoldRequirements.Value,
			newBigInt(ctx.genesisConfig.Validators.ValidatorLockedGoldRequirements.Duration),
			newBigInt(ctx.genesisConfig.Validators.ValidatorScoreExponent),
			ctx.genesisConfig.Validators.ValidatorScoreAdjustmentSpeed.BigInt(),
			newBigInt(ctx.genesisConfig.Validators.MembershipHistoryLength),
			newBigInt(ctx.genesisConfig.Validators.SlashingPenaltyResetPeriod),
			newBigInt(ctx.genesisConfig.Validators.MaxGroupSize),
			newBigInt(ctx.genesisConfig.Validators.CommissionUpdateDelay),
			newBigInt(ctx.genesisConfig.Validators.DowntimeGracePeriod),
		)
	})
}

func (ctx *deployContext) deployElection() error {
	return ctx.deployCoreContract("contracts", "Election", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			newBigInt(ctx.genesisConfig.Election.MinElectableValidators),
			newBigInt(ctx.genesisConfig.Election.MaxElectableValidators),
			ctx.genesisConfig.Election.MaxVotesPerAccount,
			ctx.genesisConfig.Election.ElectabilityThreshold.BigInt(),
		)
	})
}

func (ctx *deployContext) deploySortedOracles() error {
	return ctx.deployCoreContract("contracts", "SortedOracles", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			newBigInt(ctx.genesisConfig.SortedOracles.ReportExpirySeconds),
		)
	})
}

func (ctx *deployContext) deployGasPriceMinimum() error {
	return ctx.deployCoreContract("contracts", "GasPriceMinimum", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			ctx.genesisConfig.GasPriceMinimum.MinimumFloor,
			ctx.genesisConfig.GasPriceMinimum.TargetDensity.BigInt(),
			ctx.genesisConfig.GasPriceMinimum.AdjustmentSpeed.BigInt(),
			ctx.genesisConfig.GasPriceMinimum.BaseFeeOpCodeActivationBlock,
		)
	})
}

func (ctx *deployContext) deployReserve() error {
	err := ctx.deployCoreContract("contracts-mento", "Reserve", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			env.MustProxyAddressFor("Registry"),
			newBigInt(ctx.genesisConfig.Reserve.TobinTaxStalenessThreshold),
			ctx.genesisConfig.Reserve.DailySpendingRatio.BigInt(),
			big.NewInt(0),
			big.NewInt(0),
			ctx.genesisConfig.Reserve.AssetAllocations.SymbolsABI(),
			ctx.genesisConfig.Reserve.AssetAllocations.Weights(),
			ctx.genesisConfig.Reserve.TobinTax.BigInt(),
			ctx.genesisConfig.Reserve.TobinTaxReserveRatio.BigInt(),
		)
	})
	if err != nil {
		return err
	}

	logger := ctx.logger.New("contract", "Reserve")
	contract := ctx.contract("Reserve")

	if ctx.genesisConfig.Reserve.InitialBalance != nil && ctx.genesisConfig.Reserve.InitialBalance.Cmp(big.NewInt(0)) > 0 {
		logger.Info("Setting Initial Balance")
		ctx.statedb.SetBalance(contract.Address, ctx.genesisConfig.Reserve.InitialBalance)

		if ctx.genesisConfig.Reserve.FrozenAssetsDays > 0 && ctx.genesisConfig.Reserve.FrozenAssetsStartBalance.Cmp(big.NewInt(0)) > 0 {
			err := contract.SimpleCall("setFrozenGold",
				ctx.genesisConfig.Reserve.FrozenAssetsStartBalance,
				newBigInt(ctx.genesisConfig.Reserve.FrozenAssetsDays),
			)
			if err != nil {
				return err
			}
		}
	}

	for _, spender := range ctx.genesisConfig.Reserve.Spenders {
		if err := contract.SimpleCall("addSpender", spender); err != nil {
			return err
		}
	}

	for _, otherAddress := range ctx.genesisConfig.Reserve.OtherAddresses {
		if err := contract.SimpleCall("addOtherReserveAddress", otherAddress); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *deployContext) deployStableTokens() error {
	type StableTokenConfig struct {
		contract string
		cfg      StableTokenParameters
	}
	tokens := []StableTokenConfig{
		{"StableToken", ctx.genesisConfig.StableToken},
		{"StableTokenEUR", ctx.genesisConfig.StableTokenEUR},
		{"StableTokenBRL", ctx.genesisConfig.StableTokenBRL},
	}
	for _, token := range tokens {
		err := ctx.deployCoreContract("contracts-mento", token.contract, func(contract *contract.EVMBackend) error {
			return contract.SimpleCall("initialize",
				token.cfg.Name,
				token.cfg.Symbol,
				token.cfg.Decimals,
				env.MustProxyAddressFor("Registry"),
				token.cfg.Rate.BigInt(),
				newBigInt(token.cfg.InflationFactorUpdatePeriod),
				token.cfg.InitialBalances.Accounts(),
				token.cfg.InitialBalances.Amounts(),
				token.cfg.ExchangeIdentifier,
			)
		})
		if err != nil {
			return err
		}

		stableTokenAddress := env.MustProxyAddressFor(token.contract)

		if token.cfg.Frozen {
			ctx.logger.Info("Freezing StableToken", "contract", token.contract)
			err = ctx.contract("Freezer").SimpleCall("freeze", stableTokenAddress)
			if err != nil {
				return err
			}
		}

		// Configure StableToken Oracles
		for _, oracleAddress := range token.cfg.Oracles {
			ctx.logger.Info("Adding oracle for StableToken", "contract", token.contract, "oracle", oracleAddress)
			err = ctx.contract("SortedOracles").SimpleCall("addOracle", stableTokenAddress, oracleAddress)
			if err != nil {
				return err
			}
		}

		// If requested, fix goldPrice of stable token
		if token.cfg.GoldPrice != nil {
			ctx.logger.Info("Fixing StableToken goldPrice", "contract", token.contract)

			// first check if the admin is an authorized oracle
			authorized := false
			for _, oracleAddress := range token.cfg.Oracles {
				if oracleAddress == ctx.accounts.AdminAccount().Address {
					authorized = true
					break
				}
			}

			if !authorized {
				ctx.logger.Warn("Fixing StableToken goldprice requires setting admin as oracle", "admin", ctx.accounts.AdminAccount().Address)
				err = ctx.contract("SortedOracles").SimpleCall("addOracle", stableTokenAddress, ctx.accounts.AdminAccount().Address)
				if err != nil {
					return err
				}
			}

			ctx.logger.Info("Reporting price of StableToken to oracle", "contract", token.contract)
			err = ctx.contract("SortedOracles").SimpleCall("report",
				stableTokenAddress,
				token.cfg.GoldPrice.BigInt(),
				common.ZeroAddress,
				common.ZeroAddress,
			)
			if err != nil {
				return err
			}

			ctx.logger.Info("Add StableToken to the reserve", "contract", token.contract)
			err = ctx.contract("Reserve").SimpleCall("addToken", stableTokenAddress)
			if err != nil {
				return err
			}
		}

		ctx.logger.Info("Whitelisting StableToken as a fee currency", "contract", token.contract)
		err = ctx.contract("FeeCurrencyWhitelist").SimpleCall("addToken", stableTokenAddress)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ctx *deployContext) createAccounts(accs []env.Account, namePrefix string) error {
	accounts := ctx.contract("Accounts")

	for i, acc := range accs {
		name := fmt.Sprintf("%s %03d", namePrefix, i)
		ctx.logger.Info("Create account", "address", acc.Address, "name", name)

		if err := accounts.SimpleCallFrom(acc.Address, "createAccount"); err != nil {
			return err
		}

		if err := accounts.SimpleCallFrom(acc.Address, "setName", name); err != nil {
			return err
		}

		if err := accounts.SimpleCallFrom(acc.Address, "setAccountDataEncryptionKey", acc.PublicKey()); err != nil {
			return err
		}

		// Missing: authorizeAttestationSigner
	}
	return nil
}

func (ctx *deployContext) registerValidators() error {
	validatorAccounts := ctx.accounts.ValidatorAccounts()
	requiredAmount := ctx.genesisConfig.Validators.ValidatorLockedGoldRequirements.Value

	if err := ctx.createAccounts(validatorAccounts, "validator"); err != nil {
		return err
	}

	lockedGold := ctx.contract("LockedGold")
	validators := ctx.contract("Validators")

	for _, validator := range validatorAccounts {
		address := validator.Address
		logger := ctx.logger.New("validator", address)

		ctx.statedb.AddBalance(address, requiredAmount)

		logger.Info("Lock validator gold", "amount", requiredAmount)
		if _, err := lockedGold.Call(contract.CallOpts{Origin: address, Value: requiredAmount}, "lock"); err != nil {
			return err
		}

		logger.Info("Register validator")
		blsPub, err := validator.BLSPublicKey()
		if err != nil {
			return err
		}

		// remove the 0x04 prefix from the pub key (we need the 64 bytes variant)
		pubKey := validator.PublicKey()[1:]
		err = validators.SimpleCallFrom(address, "registerValidator", pubKey, blsPub[:], validator.MustBLSProofOfPossession())
		if err != nil {
			return err
		}
	}
	return nil
}

func (ctx *deployContext) registerValidatorGroups() error {
	validatorGroupsAccounts := ctx.accounts.ValidatorGroupAccounts()

	if err := ctx.createAccounts(validatorGroupsAccounts, "group"); err != nil {
		return err
	}

	lockedGold := ctx.contract("LockedGold")
	validators := ctx.contract("Validators")

	groupRequiredGold := new(big.Int).Mul(
		ctx.genesisConfig.Validators.GroupLockedGoldRequirements.Value,
		big.NewInt(int64(ctx.accounts.ValidatorsPerGroup)),
	)
	groupCommission := ctx.genesisConfig.Validators.Commission.BigInt()

	for _, group := range validatorGroupsAccounts {
		address := group.Address
		logger := ctx.logger.New("group", address)

		ctx.statedb.AddBalance(address, groupRequiredGold)

		logger.Info("Lock group gold", "amount", groupRequiredGold)
		if _, err := lockedGold.Call(contract.CallOpts{Origin: address, Value: groupRequiredGold}, "lock"); err != nil {
			return err
		}

		logger.Info("Register group")
		if err := validators.SimpleCallFrom(address, "registerValidatorGroup", groupCommission); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *deployContext) addValidatorsToGroups() error {
	validators := ctx.contract("Validators")

	validatorGroups := ctx.accounts.ValidatorGroups()
	for groupIdx, group := range validatorGroups {
		groupAddress := group.Address
		prevGroupAddress := common.ZeroAddress
		if groupIdx > 0 {
			prevGroupAddress = validatorGroups[groupIdx-1].Address
		}

		for i, validator := range group.Validators {
			ctx.logger.Info("Add validator to group", "validator", validator.Address, "group", groupAddress)

			// affiliate validators to group
			if err := validators.SimpleCallFrom(validator.Address, "affiliate", groupAddress); err != nil {
				return err
			}

			// accept validator as group member
			if i == 0 {
				// when adding first member, we define group voting order
				// since every group start with zero votes, we just use the prevGroup Address as the greater address
				// thus ending group order is:
				// [ groupZero, groupOne, ..., lastgroup]

				if err := validators.SimpleCallFrom(groupAddress, "addFirstMember", validator.Address, common.ZeroAddress, prevGroupAddress); err != nil {
					return err
				}
			} else {
				if err := validators.SimpleCallFrom(groupAddress, "addMember", validator.Address); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (ctx *deployContext) voteForGroups() error {
	election := ctx.contract("Election")

	validatorGroups := ctx.accounts.ValidatorGroupAccounts()

	// value previously locked on registerValidatorGroups()
	lockedGoldOnGroup := new(big.Int).Mul(
		ctx.genesisConfig.Validators.GroupLockedGoldRequirements.Value,
		big.NewInt(int64(ctx.accounts.ValidatorsPerGroup)),
	)

	// current group order (see `addFirstMember` on addValidatorsToGroup) is:
	// [ groupZero, groupOne, ..., lastgroup]

	// each group votes for themselves.
	// each group votes the SAME AMOUNT
	// each group starts with 0 votes

	// so, everytime we vote, that group becomes the one with most votes (most or equal)
	// hence, we use:
	//    greater = zero (we become the one with most votes)
	//    lesser = currentLeader

	// special case: only one group (no lesser or greater)
	if len(validatorGroups) == 1 {
		groupAddress := validatorGroups[0].Address
		ctx.logger.Info("Vote for group", "group", groupAddress, "amount", lockedGoldOnGroup)
		return election.SimpleCallFrom(groupAddress, "vote", groupAddress, lockedGoldOnGroup, common.ZeroAddress, common.ZeroAddress)
	}

	// first to vote is group 0, which is already the leader. Hence lesser should go to group 1
	currentLeader := validatorGroups[1].Address
	for _, group := range validatorGroups {
		groupAddress := group.Address

		ctx.logger.Info("Vote for group", "group", groupAddress, "amount", lockedGoldOnGroup)
		if err := election.SimpleCallFrom(groupAddress, "vote", groupAddress, lockedGoldOnGroup, currentLeader, common.ZeroAddress); err != nil {
			return err
		}

		// we now become the currentLeader
		currentLeader = groupAddress
	}

	return nil
}

func (ctx *deployContext) electValidators() error {
	if err := ctx.registerValidators(); err != nil {
		return err
	}

	if err := ctx.registerValidatorGroups(); err != nil {
		return err
	}

	if err := ctx.addValidatorsToGroups(); err != nil {
		return err
	}

	if err := ctx.voteForGroups(); err != nil {
		return err
	}

	// TODO: Config checks
	// check that we have enough validators (validators >= election.minElectableValidators)
	// check that validatorsPerGroup is <= valdiators.maxGroupSize

	return nil
}

func (ctx *deployContext) contract(contractName string) *contract.EVMBackend {
	return contract.CoreContract(ctx.runtimeConfig, contractName, env.MustProxyAddressFor(contractName))
}

func (ctx *deployContext) proxyContract(contractName string) *contract.EVMBackend {
	return contract.ProxyContract(ctx.runtimeConfig, contractName, env.MustProxyAddressFor(contractName))
}

func (ctx *deployContext) verifyState() error {
	snapshotVersion := ctx.statedb.Snapshot()
	defer ctx.statedb.RevertToSnapshot(snapshotVersion)

	var reserveSpenders []common.Address
	if _, err := ctx.contract("Reserve").Query(&reserveSpenders, "getExchangeSpenders"); err != nil {
		return err
	}
	fmt.Printf("Checking getExchangeSpenders. spenders = %s\n", reserveSpenders)

	var (
		numerator   = new(*big.Int)
		denominator = new(*big.Int)
	)
	out := &[]interface{}{
		numerator,
		denominator,
	}
	if _, err := ctx.contract("SortedOracles").Query(out, "medianRate", env.MustProxyAddressFor("StableToken")); err != nil {
		return err
	}
	fmt.Printf("Checking medianRate. numerator = %s  denominator = %s \n", (*numerator).String(), (*denominator).String())

	var gasPrice *big.Int
	if _, err := ctx.contract("GasPriceMinimum").Query(&gasPrice, "getGasPriceMinimum", env.MustProxyAddressFor("StableToken")); err != nil {
		return err
	}
	fmt.Printf("Checking gas price minimum. cusdValue = %s\n", gasPrice.String())

	return nil
}
