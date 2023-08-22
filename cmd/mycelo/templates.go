package main

import (
	"math/big"
	"math/rand"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/decimal/fixed"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/mycelo/genesis"
	"github.com/celo-org/celo-blockchain/params"
)

type template interface {
	createEnv(workdir string) (*env.Environment, error)
	createGenesisConfig(*env.Environment, *big.Int) (*genesis.Config, error)
}

func templateFromString(templateStr string) template {
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

type localEnv struct{}

func (e localEnv) createEnv(workdir string) (*env.Environment, error) {
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

func (e localEnv) createGenesisConfig(env *env.Environment, gingerbreadBlock *big.Int) (*genesis.Config, error) {
	genesisConfig, err := genesis.CreateCommonGenesisConfig(env.Config.ChainID, env.Accounts().AdminAccount().Address, params.IstanbulConfig{
		Epoch:          10,
		ProposerPolicy: 2,
		LookbackWindow: 3,
		BlockPeriod:    1,
		RequestTimeout: 3000,
	}, gingerbreadBlock)
	if err != nil {
		return nil, err
	}

	// Add balances to validator and developer accounts
	genesis.FundAccounts(genesisConfig, append(env.Accounts().ValidatorAccounts(), env.Accounts().DeveloperAccounts()...))

	return genesisConfig, nil
}

type loadtestEnv struct{}

func (e loadtestEnv) createEnv(workdir string) (*env.Environment, error) {
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

func (e loadtestEnv) createGenesisConfig(env *env.Environment, gingerbreadBlock *big.Int) (*genesis.Config, error) {
	genesisConfig, err := genesis.CreateCommonGenesisConfig(env.Config.ChainID, env.Accounts().AdminAccount().Address, params.IstanbulConfig{
		Epoch:          1000,
		ProposerPolicy: 2,
		LookbackWindow: 3,
		BlockPeriod:    5,
		RequestTimeout: 3000,
	}, gingerbreadBlock)
	if err != nil {
		return nil, err
	}

	// 10 billion gas limit, set super high on purpose
	genesisConfig.Blockchain.BlockGasLimit = 1000000000

	// Add balances to validator and developer accounts
	genesis.FundAccounts(genesisConfig, append(env.Accounts().ValidatorAccounts(), env.Accounts().DeveloperAccounts()...))

	genesisConfig.StableToken.InflationFactorUpdatePeriod = 1 * genesis.Year

	// Disable gas price min being updated
	genesisConfig.GasPriceMinimum.TargetDensity = fixed.MustNew("0.9999")
	genesisConfig.GasPriceMinimum.AdjustmentSpeed = fixed.MustNew("0")

	return genesisConfig, nil
}

type monorepoEnv struct{}

func (e monorepoEnv) createEnv(workdir string) (*env.Environment, error) {
	envCfg := &env.Config{
		Accounts: env.AccountsConfig{
			Mnemonic:             env.MustNewMnemonic(),
			NumValidators:        3,
			ValidatorsPerGroup:   5, // monorepo uses a single validator group, max group size is 5
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

func (e monorepoEnv) createGenesisConfig(env *env.Environment, gingerbreadBlock *big.Int) (*genesis.Config, error) {
	genesisConfig, err := genesis.CreateCommonGenesisConfig(env.Config.ChainID, env.Accounts().AdminAccount().Address, params.IstanbulConfig{
		Epoch:          10,
		ProposerPolicy: 2,
		LookbackWindow: 3,
		BlockPeriod:    1,
		RequestTimeout: 3000,
	}, gingerbreadBlock)
	if err != nil {
		return nil, err
	}

	// To match the 'testing' config in monorepo
	genesisConfig.EpochRewards.TargetVotingYieldInitial = fixed.MustNew("0.00016")
	genesisConfig.EpochRewards.TargetVotingYieldMax = fixed.MustNew("0.0005")
	genesisConfig.EpochRewards.TargetVotingYieldAdjustmentFactor = fixed.MustNew("0.1")
	genesisConfig.Reserve.InitialBalance = common.MustBigInt("100000000000000000000000000") // 100M CELO

	// Add balances to validator and developer accounts
	genesis.FundAccounts(genesisConfig, append(env.Accounts().ValidatorAccounts(), env.Accounts().DeveloperAccounts()...))

	return genesisConfig, nil
}
