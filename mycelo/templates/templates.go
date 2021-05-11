package templates

import (
	"math/big"
	"math/rand"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/decimal/fixed"
	"github.com/celo-org/celo-blockchain/common/decimal/token"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/mycelo/genesis"
	"github.com/celo-org/celo-blockchain/params"
)

type Template interface {
	CreateEnv(workdir string) (*env.Environment, error)
	CreateGenesisConfig(*env.Environment) (*genesis.Config, error)
}

func TemplateFromString(templateStr string) Template {
	switch templateStr {
	case "local":
		return localEnv{}
	case "loadtest":
		return loadtestEnv{}
	case "monorepo":
		return monorepoEnv{}
	}
	return localEnv{}
}

// createCommonGenesisConfig generates a config starting point which templates can then customize further
func createCommonGenesisConfig(env *env.Environment, istanbulConfig params.IstanbulConfig) *genesis.Config {
	genesisConfig := genesis.BaseConfig()
	genesisConfig.ChainID = env.Config.ChainID
	genesisConfig.GenesisTimestamp = uint64(time.Now().Unix())
	genesisConfig.Istanbul = istanbulConfig
	genesisConfig.Hardforks = genesis.HardforkConfig{
		ChurritoBlock: common.Big0,
		DonutBlock:    common.Big0,
	}
	genesisConfig.Blockchain.UptimeLookbackWindow = genesisConfig.Istanbul.LookbackWindow

	// Make admin account manager of Governance & Reserve
	adminMultisig := genesis.MultiSigParameters{
		Signatories:                      []common.Address{env.Accounts().AdminAccount().Address},
		NumRequiredConfirmations:         1,
		NumInternalRequiredConfirmations: 1,
	}

	genesisConfig.ReserveSpenderMultiSig = adminMultisig
	genesisConfig.GovernanceApproverMultiSig = adminMultisig

	// Ensure nothing is frozen
	genesisConfig.GoldToken.Frozen = false
	genesisConfig.StableToken.Frozen = false
	genesisConfig.Exchange.Frozen = false
	genesisConfig.Reserve.FrozenAssetsDays = 0
	genesisConfig.EpochRewards.Frozen = false

	return genesisConfig
}

func fundAccounts(genesisConfig *genesis.Config, accounts []env.Account) {
	cusdBalances := make([]genesis.Balance, len(accounts))
	ceurBalances := make([]genesis.Balance, len(accounts))
	goldBalances := make([]genesis.Balance, len(accounts))
	for i, acc := range accounts {
		cusdBalances[i] = genesis.Balance{Account: acc.Address, Amount: (*big.Int)(token.MustNew("50000"))} // 50k cUSD
		ceurBalances[i] = genesis.Balance{Account: acc.Address, Amount: (*big.Int)(token.MustNew("50000"))} // 50k cEUR
		goldBalances[i] = genesis.Balance{Account: acc.Address, Amount: (*big.Int)(token.MustNew("50000"))} // 50k CELO
	}
	genesisConfig.StableTokenEUR.InitialBalances = ceurBalances
	genesisConfig.StableToken.InitialBalances = cusdBalances
	genesisConfig.GoldToken.InitialBalances = goldBalances
}

type localEnv struct{}

func (e localEnv) CreateEnv(workdir string) (*env.Environment, error) {
	envCfg := &env.Config{
		Accounts: env.AccountsConfig{
			Mnemonic:             env.MustNewMnemonic(),
			NumValidators:        3,
			ValidatorsPerGroup:   1,
			NumDeveloperAccounts: 10,
		},
		ChainID: big.NewInt(1000 * (1 + rand.Int63n(9999))),
	}
	env, err := env.New(workdir, envCfg)
	if err != nil {
		return nil, err
	}

	return env, nil
}

func (e localEnv) CreateGenesisConfig(env *env.Environment) (*genesis.Config, error) {

	genesisConfig := createCommonGenesisConfig(env, params.IstanbulConfig{
		Epoch:          10,
		ProposerPolicy: 2,
		LookbackWindow: 3,
		BlockPeriod:    1,
		RequestTimeout: 3000,
	})

	// Add balances to developer accounts
	fundAccounts(genesisConfig, env.Accounts().DeveloperAccounts())

	return genesisConfig, nil
}

type loadtestEnv struct{}

func (e loadtestEnv) CreateEnv(workdir string) (*env.Environment, error) {
	envCfg := &env.Config{
		Accounts: env.AccountsConfig{
			Mnemonic:             "miss fire behind decide egg buyer honey seven advance uniform profit renew",
			NumValidators:        1,
			ValidatorsPerGroup:   1,
			NumDeveloperAccounts: 10000,
		},
		ChainID: big.NewInt(9099000),
	}

	env, err := env.New(workdir, envCfg)
	if err != nil {
		return nil, err
	}

	return env, nil
}

func (e loadtestEnv) CreateGenesisConfig(env *env.Environment) (*genesis.Config, error) {
	genesisConfig := createCommonGenesisConfig(env, params.IstanbulConfig{
		Epoch:          1000,
		ProposerPolicy: 2,
		LookbackWindow: 3,
		BlockPeriod:    5,
		RequestTimeout: 3000,
	})

	// 10 billion gas limit, set super high on purpose
	genesisConfig.Blockchain.BlockGasLimit = 1000000000

	// Add balances to developer accounts
	fundAccounts(genesisConfig, env.Accounts().DeveloperAccounts())

	genesisConfig.StableToken.InflationFactorUpdatePeriod = 1 * genesis.Year

	// Disable gas price min being updated
	genesisConfig.GasPriceMinimum.TargetDensity = fixed.MustNew("0.9999")
	genesisConfig.GasPriceMinimum.AdjustmentSpeed = fixed.MustNew("0")

	return genesisConfig, nil
}

type monorepoEnv struct{}

func (e monorepoEnv) CreateEnv(workdir string) (*env.Environment, error) {
	envCfg := &env.Config{
		Accounts: env.AccountsConfig{
			Mnemonic:             env.MustNewMnemonic(),
			NumValidators:        3,
			ValidatorsPerGroup:   1,
			NumDeveloperAccounts: 0,
			UseValidatorAsAdmin:  true, // monorepo doesn't use the admin account type, uses first validator instead
		},
		ChainID: big.NewInt(1000 * (1 + rand.Int63n(9999))),
	}
	env, err := env.New(workdir, envCfg)
	if err != nil {
		return nil, err
	}

	return env, nil
}

func (e monorepoEnv) CreateGenesisConfig(env *env.Environment) (*genesis.Config, error) {
	genesisConfig := createCommonGenesisConfig(env, params.IstanbulConfig{
		Epoch:          10,
		ProposerPolicy: 2,
		LookbackWindow: 3,
		BlockPeriod:    1,
		RequestTimeout: 3000,
	})

	// Add balances to validator accounts instead of developer accounts
	fundAccounts(genesisConfig, env.Accounts().ValidatorAccounts())

	return genesisConfig, nil
}
