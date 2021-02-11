package genesis

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/mycelo/config"
	"github.com/ethereum/go-ethereum/mycelo/contract"
)

var (
	proxyOwnerStorageLocation = common.HexToHash("0xb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d6103")
	proxyByteCode             = common.Hex2Bytes("60806040526004361061004a5760003560e01c806303386ba3146101e757806342404e0714610280578063bb913f41146102d7578063d29d44ee14610328578063f7e6af8014610379575b6000600160405180807f656970313936372e70726f78792e696d706c656d656e746174696f6e00000000815250601c019050604051809103902060001c0360001b9050600081549050600073ffffffffffffffffffffffffffffffffffffffff168173ffffffffffffffffffffffffffffffffffffffff161415610136576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260158152602001807f4e6f20496d706c656d656e746174696f6e20736574000000000000000000000081525060200191505060405180910390fd5b61013f816103d0565b6101b1576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260188152602001807f496e76616c696420636f6e74726163742061646472657373000000000000000081525060200191505060405180910390fd5b60405136810160405236600082376000803683855af43d604051818101604052816000823e82600081146101e3578282f35b8282fd5b61027e600480360360408110156101fd57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291908035906020019064010000000081111561023a57600080fd5b82018360208201111561024c57600080fd5b8035906020019184600183028401116401000000008311171561026e57600080fd5b909192939192939050505061041b565b005b34801561028c57600080fd5b506102956105c1565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b3480156102e357600080fd5b50610326600480360360208110156102fa57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061060d565b005b34801561033457600080fd5b506103776004803603602081101561034b57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506107bd565b005b34801561038557600080fd5b5061038e610871565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b60008060007fc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47060001b9050833f915080821415801561041257506000801b8214155b92505050919050565b610423610871565b73ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff16146104c3576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260148152602001807f73656e64657220776173206e6f74206f776e657200000000000000000000000081525060200191505060405180910390fd5b6104cc8361060d565b600060608473ffffffffffffffffffffffffffffffffffffffff168484604051808383808284378083019250505092505050600060405180830381855af49150503d8060008114610539576040519150601f19603f3d011682016040523d82523d6000602084013e61053e565b606091505b508092508193505050816105ba576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601e8152602001807f696e697469616c697a6174696f6e2063616c6c6261636b206661696c6564000081525060200191505060405180910390fd5b5050505050565b600080600160405180807f656970313936372e70726f78792e696d706c656d656e746174696f6e00000000815250601c019050604051809103902060001c0360001b9050805491505090565b610615610871565b73ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff16146106b5576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260148152602001807f73656e64657220776173206e6f74206f776e657200000000000000000000000081525060200191505060405180910390fd5b6000600160405180807f656970313936372e70726f78792e696d706c656d656e746174696f6e00000000815250601c019050604051809103902060001c0360001b9050610701826103d0565b610773576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260188152602001807f496e76616c696420636f6e74726163742061646472657373000000000000000081525060200191505060405180910390fd5b8181558173ffffffffffffffffffffffffffffffffffffffff167fab64f92ab780ecbf4f3866f57cee465ff36c89450dcce20237ca7a8d81fb7d1360405160405180910390a25050565b6107c5610871565b73ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff1614610865576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260148152602001807f73656e64657220776173206e6f74206f776e657200000000000000000000000081525060200191505060405180910390fd5b61086e816108bd565b50565b600080600160405180807f656970313936372e70726f78792e61646d696e000000000000000000000000008152506013019050604051809103902060001c0360001b9050805491505090565b600073ffffffffffffffffffffffffffffffffffffffff168173ffffffffffffffffffffffffffffffffffffffff161415610960576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260118152602001807f6f776e65722063616e6e6f74206265203000000000000000000000000000000081525060200191505060405180910390fd5b6000600160405180807f656970313936372e70726f78792e61646d696e000000000000000000000000008152506013019050604051809103902060001c0360001b90508181558173ffffffffffffffffffffffffffffffffffffffff167f50146d0e3c60aa1d17a70635b05494f864e86144a2201275021014fbf08bafe260405160405180910390a2505056fea165627a7a72305820f4f741dbef8c566cb1690ae708b8ef1113bdb503225629cc1f9e86bd47efd1a40029")
	adminGoldBalance, _       = new(big.Int).SetString("1000000000000000000000000000", 10)
)

// deployContext context for deployment
type deployContext struct {
	GenesisConfig *Config
	adminAccount  config.Account
	statedb       *state.StateDB
	runtimeConfig *runtime.Config
	contracts     *contract.CoreContracts
	logger        log.Logger
}

func generateGenesisState(adminAccount config.Account, cfg *Config, buildPath string) (core.GenesisAlloc, error) {
	deployment := newDeployment(cfg, adminAccount, buildPath)
	return deployment.deploy()
}

// NewDeployment generates a new deployment
func newDeployment(genesisConfig *Config, adminAccount config.Account, buildPath string) *deployContext {

	statedb, _ := state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)

	return &deployContext{
		GenesisConfig: genesisConfig,
		adminAccount:  adminAccount,
		logger:        log.New("obj", "deployment"),
		statedb:       statedb,
		contracts:     contract.NewCoreContracts(buildPath),
		runtimeConfig: &runtime.Config{
			ChainConfig: genesisConfig.ChainConfig(),
			Origin:      adminAccount.Address,
			State:       statedb,
			GasLimit:    10000000000000000000,
			GasPrice:    big.NewInt(0),
			Value:       big.NewInt(0),
			Time:        new(big.Int).SetUint64(genesisConfig.GenesisTimestamp),
			Coinbase:    adminAccount.Address,
			BlockNumber: new(big.Int).SetUint64(0),
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
		ctx.deployLibraries,
		// 01 Registry
		ctx.deployRegistry,
		// 02 Freezer
		ctx.deployFreezer,

		// 03 TransferWhitelist
		ctx.deployTransferWhitelist,

		// 03.bis FeeCurrencyWhitelist
		ctx.deployFeeCurrencyWhitelist,

		// 04 GoldToken
		ctx.deployGoldToken,

		// 05 SortedOracles
		ctx.deploySortedOracles,

		// 06 GasPriceMinimum
		ctx.deployGasPriceMinimum,

		// 07 Reserve
		ctx.deployReserve,

		// 08 ReserveSpenderMultisig (requires reserve to work)
		ctx.deployReserveSpenderMultisig,

		// 09 StableToken
		ctx.deployStableToken,

		// 10 Exchange
		ctx.deployExchange,

		// 11 Accounts
		ctx.deployAccounts,

		// 12 LockedGold
		ctx.deployLockedGold,

		// 13 Validators
		ctx.deployValidators,

		// 14 Election
		ctx.deployElection,

		// 15 EpochRewards
		ctx.deployEpochRewards,

		// 16 Random
		ctx.deployRandom,

		// // 17 Attestations
		// ctx.deployAttestations,

		// 18 Escrow
		ctx.deployEscrow,

		// 19 BlockchainParameters
		ctx.deployBlockchainParameters,

		// 20 GovernanceSlasher
		ctx.deployGovernanceSlasher,

		// 21 DoubleSigningSlasher
		ctx.deployDoubleSigningSlasher,

		// 22 DowntimeSlasher
		ctx.deployDowntimeSlasher,

		// 23 GovernanceApproverMultiSig
		ctx.deployGovernanceApproverMultiSig,

		// // 24 Governance
		// ctx.deployGovernance,
	}

	logger := ctx.logger.New()

	for i, step := range deploySteps {
		logger.Info("Running deploy step", "number", i)
		if err := step(); err != nil {
			return nil, err
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

	dump := (map[common.Address]state.DumpAccount)(ctx.statedb.RawDump(false, false, true).Accounts)
	genesisAlloc := make(map[common.Address]core.GenesisAccount)
	for acc, dumpAcc := range dump {
		var account core.GenesisAccount

		if dumpAcc.Balance != "" {
			account.Balance, _ = new(big.Int).SetString(dumpAcc.Balance, 10)
		}

		if dumpAcc.Code != "" {
			account.Code = common.Hex2Bytes(dumpAcc.Code)
		}

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
	ctx.statedb.SetBalance(ctx.adminAccount.Address, new(big.Int).Set(adminGoldBalance))
}

func (ctx *deployContext) deployLibraries() error {
	libraryByteCodes, err := ctx.contracts.LibraryDeployedBytecodes()
	if err != nil {
		return err
	}

	for addr, bytes := range libraryByteCodes {
		ctx.statedb.SetCode(addr, bytes)
	}
	return nil
}

// deployProxiedContract will deploy proxied contract
// It will deploy the proxy contract, the impl contract, and initialize both
func (ctx *deployContext) deployProxiedContract(name string, initialize func(contract *contract.EVMBackend) error) error {
	proxyAddress := ctx.contracts.ProxyAddressFor(name)
	implAddress := ctx.contracts.ImplAddressFor(name)
	bytecode := ctx.contracts.MustDeployedBytecodeFor(name)

	logger := ctx.logger.New("contract", name)
	logger.Info("Start Deploy of Proxied Contract", "proxyAddress", proxyAddress.Hex(), "implAddress", implAddress.Hex())

	logger.Info("Deploy Proxy")
	ctx.statedb.SetCode(proxyAddress, proxyByteCode)
	ctx.statedb.SetState(proxyAddress, proxyOwnerStorageLocation, ctx.adminAccount.Address.Hash())

	logger.Info("Deploy Implementation")
	ctx.statedb.SetCode(implAddress, bytecode)

	logger.Info("Set proxy implementation")
	proxyContract := ctx.proxyContract(name)

	if err := proxyContract.SimpleCall("_setImplementation", implAddress); err != nil {
		return err
	}

	logger.Info("Initialize Contract")
	if err := initialize(ctx.contract(name)); err != nil {
		return err
	}

	return nil
}

// deployCoreContract will deploy a contract + proxy, and add it to the registry
func (ctx *deployContext) deployCoreContract(name string, initialize func(contract *contract.EVMBackend) error) error {
	if err := ctx.deployProxiedContract(name, initialize); err != nil {
		return err
	}

	proxyAddress := ctx.contracts.ProxyAddressFor(name)
	ctx.logger.Info("Add entry to registry", "name", name, "address", proxyAddress)
	if err := ctx.contract("Registry").SimpleCall("setAddressFor", name, proxyAddress); err != nil {
		return err
	}

	return nil
}

func (ctx *deployContext) deployTransferWhitelist() error {
	name := "TransferWhitelist"
	logger := ctx.logger.New("contract", name)

	contract, err := contract.DeployCoreContract(
		ctx.runtimeConfig,
		"TransferWhitelist",
		ctx.contracts.MustBytecodeFor("TransferWhitelist"),
		ctx.contracts.ProxyAddressFor("Registry"),
	)
	if err != nil {
		return err
	}
	logger.Info("Contract deployed", "address", contract.Address)

	logger.Debug("setDirectlyWhitelistedAddresses")
	err = contract.SimpleCall("setDirectlyWhitelistedAddresses", ctx.GenesisConfig.TransferWhitelist.Addresses)
	if err != nil {
		return err
	}

	logger.Debug("setWhitelistedContractIdentifiers")
	err = contract.SimpleCall("setWhitelistedContractIdentifiers", ctx.GenesisConfig.TransferWhitelist.RegistryIDs)
	if err != nil {
		return err
	}

	logger.Info("Add to Registry")
	if err := ctx.contract("Registry").SimpleCall("setAddressFor", name, contract.Address); err != nil {
		return err
	}

	return nil
}

func (ctx *deployContext) deployMultiSig(name string, params MultiSigParameters) (common.Address, error) {
	err := ctx.deployProxiedContract(name, func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			params.Signatories,
			new(big.Int).SetUint64(params.NumRequiredConfirmations),
			new(big.Int).SetUint64(params.NumInternalRequiredConfirmations),
		)
	})
	if err != nil {
		return common.ZeroAddress, err
	}
	return ctx.contracts.ProxyAddressFor(name), nil
}

func (ctx *deployContext) deployReserveSpenderMultisig() error {
	multiSigAddr, err := ctx.deployMultiSig("ReserveSpenderMultiSig", ctx.GenesisConfig.ReserveSpenderMultiSig)
	if err != nil {
		return err
	}

	if err := ctx.contract("Reserve").SimpleCall("addSpender", multiSigAddr); err != nil {
		return err
	}
	return nil
}

func (ctx *deployContext) deployGovernanceApproverMultiSig() error {
	_, err := ctx.deployMultiSig("GovernanceApproverMultiSig", ctx.GenesisConfig.GovernanceApproverMultiSig)
	return err
}

func (ctx *deployContext) deployRegistry() error {
	return ctx.deployCoreContract("Registry", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize")
	})
}

func (ctx *deployContext) deployBlockchainParameters() error {
	return ctx.deployCoreContract("BlockchainParameters", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			big.NewInt(ctx.GenesisConfig.Blockchain.Version.Major),
			big.NewInt(ctx.GenesisConfig.Blockchain.Version.Minor),
			big.NewInt(ctx.GenesisConfig.Blockchain.Version.Patch),
			ctx.GenesisConfig.Blockchain.GasForNonGoldCurrencies,
			ctx.GenesisConfig.Blockchain.BlockGasLimit,
			big.NewInt(ctx.GenesisConfig.Blockchain.UptimeLookbackWindow),
		)
	})
}

func (ctx *deployContext) deployFreezer() error {
	return ctx.deployCoreContract("Freezer", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize")
	})
}

func (ctx *deployContext) deployGovernanceSlasher() error {
	err := ctx.deployCoreContract("GovernanceSlasher", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.contracts.ProxyAddressFor("Registry"),
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
	err := ctx.deployCoreContract("DoubleSigningSlasher", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.contracts.ProxyAddressFor("Registry"),
			ctx.GenesisConfig.DoubleSigningSlasher.Penalty,
			ctx.GenesisConfig.DoubleSigningSlasher.Reward,
		)
	})
	if err != nil {
		return err
	}

	return ctx.addSlasher("DoubleSigningSlasher")
}

func (ctx *deployContext) deployDowntimeSlasher() error {
	err := ctx.deployCoreContract("DowntimeSlasher", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.contracts.ProxyAddressFor("Registry"),
			ctx.GenesisConfig.DowntimeSlasher.Penalty,
			ctx.GenesisConfig.DowntimeSlasher.Reward,
			new(big.Int).SetUint64(ctx.GenesisConfig.DowntimeSlasher.SlashableDowntime),
		)
	})
	if err != nil {
		return err
	}

	return ctx.addSlasher("DowntimeSlasher")
}

func (ctx *deployContext) deployEscrow() error {
	return ctx.deployCoreContract("Escrow", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize", ctx.contracts.ProxyAddressFor("Registry"))
	})
}

func (ctx *deployContext) deployFeeCurrencyWhitelist() error {
	return ctx.deployCoreContract("FeeCurrencyWhitelist", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize")
	})
}

func (ctx *deployContext) deployGoldToken() error {
	err := ctx.deployCoreContract("GoldToken", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize", ctx.contracts.ProxyAddressFor("Registry"))
	})
	if err != nil {
		return err
	}

	if ctx.GenesisConfig.GoldToken.Frozen {
		ctx.logger.Info("Freezing GoldToken")
		err = ctx.contract("Freezer").SimpleCall("freeze", ctx.contracts.ProxyAddressFor("GoldToken"))
		if err != nil {
			return err
		}
	}

	for _, bal := range ctx.GenesisConfig.GoldToken.InitialBalances {
		ctx.statedb.SetBalance(bal.Account, bal.Amount)
	}

	return nil
}

func (ctx *deployContext) deployExchange() error {
	err := ctx.deployCoreContract("Exchange", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.contracts.ProxyAddressFor("Registry"),
			ctx.contracts.ProxyAddressFor("StableToken"),
			ctx.GenesisConfig.Exchange.Spread.BigInt(),
			ctx.GenesisConfig.Exchange.ReserveFraction.BigInt(),
			new(big.Int).SetUint64(ctx.GenesisConfig.Exchange.UpdateFrequency),
			new(big.Int).SetUint64(ctx.GenesisConfig.Exchange.MinimumReports),
		)
	})
	if err != nil {
		return err
	}

	if ctx.GenesisConfig.Exchange.Frozen {
		ctx.logger.Info("Freezing Exchange")
		err = ctx.contract("Freezer").SimpleCall("freeze", ctx.contracts.ProxyAddressFor("Exchange"))
		if err != nil {
			return err
		}
	}
	return nil
}

func (ctx *deployContext) deployEpochRewards() error {
	err := ctx.deployCoreContract("EpochRewards", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.contracts.ProxyAddressFor("Registry"),
			ctx.GenesisConfig.EpochRewards.TargetVotingYieldInitial.BigInt(),
			ctx.GenesisConfig.EpochRewards.TargetVotingYieldMax.BigInt(),
			ctx.GenesisConfig.EpochRewards.TargetVotingYieldAdjustmentFactor.BigInt(),
			ctx.GenesisConfig.EpochRewards.RewardsMultiplierMax.BigInt(),
			ctx.GenesisConfig.EpochRewards.RewardsMultiplierAdjustmentFactorsUnderspend.BigInt(),
			ctx.GenesisConfig.EpochRewards.RewardsMultiplierAdjustmentFactorsOverspend.BigInt(),
			ctx.GenesisConfig.EpochRewards.TargetVotingGoldFraction.BigInt(),
			ctx.GenesisConfig.EpochRewards.MaxValidatorEpochPayment,
			ctx.GenesisConfig.EpochRewards.CommunityRewardFraction.BigInt(),
			ctx.GenesisConfig.EpochRewards.CarbonOffsettingPartner,
			ctx.GenesisConfig.EpochRewards.CarbonOffsettingFraction.BigInt(),
		)
	})
	if err != nil {
		return err
	}

	if ctx.GenesisConfig.EpochRewards.Frozen {
		ctx.logger.Info("Freezing EpochRewards")
		err = ctx.contract("Freezer").SimpleCall("freeze", ctx.contracts.ProxyAddressFor("EpochRewards"))
		if err != nil {
			return err
		}
	}
	return nil
}

func (ctx *deployContext) deployAccounts() error {
	return ctx.deployCoreContract("Accounts", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize", ctx.contracts.ProxyAddressFor("Registry"))
	})
}

func (ctx *deployContext) deployRandom() error {
	return ctx.deployCoreContract("Random", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.GenesisConfig.Random.RandomnessBlockRetentionWindow,
		)
	})
}

func (ctx *deployContext) deployLockedGold() error {
	return ctx.deployCoreContract("LockedGold", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.contracts.ProxyAddressFor("Registry"),
			ctx.GenesisConfig.LockedGold.UnlockingPeriod,
		)
	})
}

func (ctx *deployContext) deployValidators() error {
	return ctx.deployCoreContract("Validators", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.contracts.ProxyAddressFor("Registry"),
			ctx.GenesisConfig.Validators.GroupLockedGoldRequirements.Value,
			ctx.GenesisConfig.Validators.GroupLockedGoldRequirements.Duration,
			ctx.GenesisConfig.Validators.ValidatorLockedGoldRequirements.Value,
			ctx.GenesisConfig.Validators.ValidatorLockedGoldRequirements.Duration,
			ctx.GenesisConfig.Validators.ValidatorScoreExponent,
			ctx.GenesisConfig.Validators.ValidatorScoreAdjustmentSpeed.BigInt(),
			ctx.GenesisConfig.Validators.MembershipHistoryLength,
			ctx.GenesisConfig.Validators.SlashingPenaltyResetPeriod,
			ctx.GenesisConfig.Validators.MaxGroupSize,
			ctx.GenesisConfig.Validators.CommissionUpdateDelay,
		)
	})
}

func (ctx *deployContext) deployElection() error {
	return ctx.deployCoreContract("Election", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.contracts.ProxyAddressFor("Registry"),
			ctx.GenesisConfig.Election.MinElectableValidators,
			ctx.GenesisConfig.Election.MaxElectableValidators,
			ctx.GenesisConfig.Election.MaxVotesPerAccount,
			ctx.GenesisConfig.Election.ElectabilityThreshold.BigInt(),
		)
	})
}

func (ctx *deployContext) deploySortedOracles() error {
	return ctx.deployCoreContract("SortedOracles", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			big.NewInt(ctx.GenesisConfig.SortedOracles.ReportExpirySeconds),
		)
	})
}

func (ctx *deployContext) deployGasPriceMinimum() error {
	return ctx.deployCoreContract("GasPriceMinimum", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.contracts.ProxyAddressFor("Registry"),
			ctx.GenesisConfig.GasPriceMinimum.MinimunFloor,
			ctx.GenesisConfig.GasPriceMinimum.TargetDensity.BigInt(),
			ctx.GenesisConfig.GasPriceMinimum.AdjustmentSpeed.BigInt(),
		)
	})
}

func (ctx *deployContext) deployReserve() error {
	err := ctx.deployCoreContract("Reserve", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.contracts.ProxyAddressFor("Registry"),
			ctx.GenesisConfig.Reserve.TobinTaxStalenessThreshold,
			ctx.GenesisConfig.Reserve.DailySpendingRatio,
			big.NewInt(0),
			big.NewInt(0),
			ctx.GenesisConfig.Reserve.AssetAllocations.SymbolsABI(),
			ctx.GenesisConfig.Reserve.AssetAllocations.Weights(),
			ctx.GenesisConfig.Reserve.TobinTax,
			ctx.GenesisConfig.Reserve.TobinTaxReserveRatio,
		)
	})
	if err != nil {
		return err
	}

	logger := ctx.logger.New("contract", "Reserve")
	contract := ctx.contract("Reserve")

	if ctx.GenesisConfig.Reserve.InitialBalance != nil && ctx.GenesisConfig.Reserve.InitialBalance.Cmp(big.NewInt(0)) > 0 {
		logger.Info("Setting Initial Balance")
		ctx.statedb.SetBalance(contract.Address, ctx.GenesisConfig.Reserve.InitialBalance)

		if ctx.GenesisConfig.Reserve.FrozenAssetsDays.Cmp(big.NewInt(0)) > 0 && ctx.GenesisConfig.Reserve.FrozenAssetsStartBalance.Cmp(big.NewInt(0)) > 0 {
			err := contract.SimpleCall("setFrozenGold",
				ctx.GenesisConfig.Reserve.FrozenAssetsStartBalance,
				ctx.GenesisConfig.Reserve.FrozenAssetsDays,
			)
			if err != nil {
				return err
			}
		}
	}

	for _, spender := range ctx.GenesisConfig.Reserve.Spenders {
		if err := contract.SimpleCall("addSpender", spender); err != nil {
			return err
		}
	}

	for _, otherAddress := range ctx.GenesisConfig.Reserve.OtherAddresses {
		if err := contract.SimpleCall("addOtherReserveAddress", otherAddress); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *deployContext) deployStableToken() error {
	err := ctx.deployCoreContract("StableToken", func(contract *contract.EVMBackend) error {
		return contract.SimpleCall("initialize",
			ctx.GenesisConfig.StableToken.Name,
			ctx.GenesisConfig.StableToken.Symbol,
			ctx.GenesisConfig.StableToken.Decimals,
			ctx.contracts.ProxyAddressFor("Registry"),
			ctx.GenesisConfig.StableToken.Rate.BigInt(),
			ctx.GenesisConfig.StableToken.InflationFactorUpdatePeriod,
			ctx.GenesisConfig.StableToken.InitialBalances.Accounts(),
			ctx.GenesisConfig.StableToken.InitialBalances.Amounts(),
		)
	})
	if err != nil {
		return err
	}

	stableTokenAddress := ctx.contracts.ProxyAddressFor("StableToken")

	if ctx.GenesisConfig.StableToken.Frozen {
		ctx.logger.Info("Freezing StableToken")
		err = ctx.contract("Freezer").SimpleCall("freeze", stableTokenAddress)
		if err != nil {
			return err
		}
	}

	// Configure StableToken Oracles
	for _, oracleAddress := range ctx.GenesisConfig.StableToken.Oracles {
		ctx.logger.Info("Adding oracle for StableToken", "oracle", oracleAddress)
		err = ctx.contract("SortedOracles").SimpleCall("addOracle", stableTokenAddress, oracleAddress)
		if err != nil {
			return err
		}
	}

	// If requested, fix golPrice of stable token
	if ctx.GenesisConfig.StableToken.GoldPrice != nil {
		ctx.logger.Info("Fixing StableToken goldPrice")

		// first check if the admin is an authorized oracle
		authorized := false
		for _, oracleAddress := range ctx.GenesisConfig.StableToken.Oracles {
			if oracleAddress == ctx.adminAccount.Address {
				authorized = true
				break
			}
		}

		if !authorized {
			ctx.logger.Warn("Fixing StableToken goldprice requires setting admin as oracle", "admin", ctx.adminAccount.Address)
			err = ctx.contract("SortedOracles").SimpleCall("addOracle", stableTokenAddress, ctx.adminAccount.Address)
			if err != nil {
				return err
			}
		}

		ctx.logger.Info("Reporting price of StableToken to oracle")
		err = ctx.contract("SortedOracles").SimpleCall("report",
			stableTokenAddress,
			ctx.GenesisConfig.StableToken.GoldPrice.BigInt(),
			common.ZeroAddress,
			common.ZeroAddress,
		)
		if err != nil {
			return err
		}

		ctx.logger.Info("Add StableToken to the reserve")
		err = ctx.contract("Reserve").SimpleCall("addToken", stableTokenAddress)
		if err != nil {
			return err
		}
	}

	ctx.logger.Info("Whitelisting StableToken as a fee currency")
	err = ctx.contract("FeeCurrencyWhitelist").SimpleCall("addToken", stableTokenAddress)
	if err != nil {
		return err
	}

	return nil
}

func (ctx *deployContext) getAddressFromRegistry(name string) (common.Address, error) {
	var result common.Address
	_, err := ctx.contract("Registry").Query(&result, "getAddressForString", name)
	return result, err
}

func (ctx *deployContext) contract(contractName string) *contract.EVMBackend {
	return contract.CoreContract(ctx.runtimeConfig, contractName, ctx.contracts.ProxyAddressFor(contractName))
}

func (ctx *deployContext) proxyContract(contractName string) *contract.EVMBackend {
	return contract.ProxyContract(ctx.runtimeConfig, contractName, ctx.contracts.ProxyAddressFor(contractName))
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
	if _, err := ctx.contract("SortedOracles").Query(out, "medianRate", ctx.contracts.ProxyAddressFor("StableToken")); err != nil {
		return err
	}
	fmt.Printf("Checking medianRate. numerator = %s  denominator = %s \n", (*numerator).String(), (*denominator).String())

	var gasPrice *big.Int
	if _, err := ctx.contract("GasPriceMinimum").Query(&gasPrice, "getGasPriceMinimum", ctx.contracts.ProxyAddressFor("StableToken")); err != nil {
		return err
	}
	fmt.Printf("Checking gas price minimun. cusdValue = %s\n", gasPrice.String())

	return nil
}
