package main

import (
	"math/big"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/mycelo/config"
	"github.com/ethereum/go-ethereum/params"
)

type template interface {
	createEnv(workdir string) (*config.Environment, error)
}

func templateFromString(templateStr string) template {
	switch templateStr {
	case "local":
		return localEnv{}
	case "testnet":
		return testnetEnv{}
	}
	return localEnv{}
}

type localEnv struct{}

func (e localEnv) createEnv(workdir string) (*config.Environment, error) {
	envCfg := &config.EnvConfig{
		Mnemonic:           config.MustNewMnemonic(),
		InitialValidators:  3,
		ValidatorsPerGroup: 1,
		DeveloperAccounts:  10,
	}

	env, err := config.NewEnv(workdir, envCfg)
	if err != nil {
		return nil, err
	}

	env.GenesisConfig.ChainID = big.NewInt(1000 * (1 + rand.Int63n(9999)))
	env.GenesisConfig.GenesisTimestamp = uint64(time.Now().Unix())
	env.GenesisConfig.Istanbul = params.IstanbulConfig{
		Epoch:          10,
		ProposerPolicy: 2,
		LookbackWindow: 3,
		BlockPeriod:    1,
		RequestTimeout: 3000,
	}
	env.GenesisConfig.Hardforks = config.HardforkConfig{
		ChurritoBlock: common.Big0,
		DonutBlock:    common.Big0,
	}
	env.GenesisConfig.Blockchain.UptimeLookbackWindow = int64(env.GenesisConfig.Istanbul.LookbackWindow)

	// Make admin account manager of Governance & Reserve
	adminMultisig := config.MultiSigParameters{
		Signatories:                      []common.Address{env.AdminAccount().Address},
		NumRequiredConfirmations:         1,
		NumInternalRequiredConfirmations: 1,
	}

	env.GenesisConfig.ReserveSpenderMultiSig = adminMultisig
	env.GenesisConfig.GovernanceApproverMultiSig = adminMultisig

	// Add balances to developer accounts
	cusdBalances := make([]config.Balance, len(env.DeveloperAccounts()))
	goldBalances := make([]config.Balance, len(env.DeveloperAccounts()))
	for i, acc := range env.DeveloperAccounts() {
		cusdBalances[i] = config.Balance{acc.Address, config.MustBigInt("50000000000000000000000")}
		goldBalances[i] = config.Balance{acc.Address, config.MustBigInt("1000000000000000000000000")}
	}

	env.GenesisConfig.StableToken.InitialBalances = cusdBalances
	env.GenesisConfig.GoldToken.InitialBalances = cusdBalances

	// Ensure nothing is frozen
	env.GenesisConfig.GoldToken.Frozen = false
	env.GenesisConfig.StableToken.Frozen = false
	env.GenesisConfig.Exchange.Frozen = false
	env.GenesisConfig.Reserve.FrozenDays = nil
	env.GenesisConfig.Reserve.FrozenAssetsDays = nil
	env.GenesisConfig.EpochRewards.Frozen = false

	return env, nil
}

type testnetEnv struct{}

func (e testnetEnv) createEnv(workdir string) (*config.Environment, error) {
	panic("Not implemented")
}
