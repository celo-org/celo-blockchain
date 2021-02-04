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
	createEnv(workdir string, applyConfigOverrides func(*config.Config)) (*config.Environment, error)
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

func (e localEnv) createEnv(workdir string, applyConfigOverrides func(*config.Config)) (*config.Environment, error) {
	cfg := &config.Config{
		ChainID:          big.NewInt(1000 * (1 + rand.Int63n(9999))),
		GenesisTimestamp: uint64(time.Now().Unix()),
		Mnemonic:         config.MustNewMnemonic(),

		Istanbul: params.IstanbulConfig{
			Epoch:          10,
			ProposerPolicy: 2,
			LookbackWindow: 3,
			BlockPeriod:    1,
			RequestTimeout: 3000,
		},

		Hardforks: config.HardforkConfig{
			ChurritoBlock: common.Big0,
			DonutBlock:    common.Big0,
		},

		InitialValidators:  3,
		ValidatorsPerGroup: 1,
		DeveloperAccounts:  10,
	}

	if applyConfigOverrides != nil {
		applyConfigOverrides(cfg)
	}

	env, err := config.NewEnv(workdir, cfg)
	if err != nil {
		return nil, err
	}

	env.ContractParameters.Blockchain.UptimeLookbackWindow = int64(env.Config.Istanbul.LookbackWindow)

	// Make admin account manager of Governance & Reserve
	adminMultisig := config.MultiSigParameters{
		Signatories:                      []common.Address{env.AdminAccount().Address},
		NumRequiredConfirmations:         1,
		NumInternalRequiredConfirmations: 1,
	}

	env.ContractParameters.ReserveSpenderMultiSig = adminMultisig
	env.ContractParameters.GovernanceApproverMultiSig = adminMultisig

	// Add balances to developer accounts
	cusdBalances := make([]config.Balance, len(env.DeveloperAccounts()))
	goldBalances := make([]config.Balance, len(env.DeveloperAccounts()))
	for i, acc := range env.DeveloperAccounts() {
		cusdBalances[i] = config.Balance{acc.Address, config.MustBigInt("50000000000000000000000")}
		goldBalances[i] = config.Balance{acc.Address, config.MustBigInt("1000000000000000000000000")}
	}

	env.ContractParameters.StableToken.InitialBalances = cusdBalances
	env.ContractParameters.GoldToken.InitialBalances = cusdBalances

	// Ensure nothing is frozen
	env.ContractParameters.GoldToken.Frozen = false
	env.ContractParameters.StableToken.Frozen = false
	env.ContractParameters.Exchange.Frozen = false
	env.ContractParameters.Reserve.FrozenDays = nil
	env.ContractParameters.Reserve.FrozenAssetsDays = nil
	env.ContractParameters.EpochRewards.Frozen = false

	return env, nil
}

type testnetEnv struct{}

func (e testnetEnv) createEnv(workdir string, applyConfigOverrides func(*config.Config)) (*config.Environment, error) {
	panic("Not implemented")
}
