package genesis

import (
	"math/big"
	"math/rand"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/decimal/fixed"
	"github.com/celo-org/celo-blockchain/common/decimal/token"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/params"
)

type template interface {
	CreateConfig() (*env.Config, error)
	CreateGenesisConfig(*env.Config) (*Config, error)
}

func TemplateFromString(templateStr string) template {
	switch templateStr {
	case "local":
		return LocalEnv{}
	case "loadtest":
		return LoadtestEnv{}
	case "monorepo":
		return MonorepoEnv{}
	}
	return LocalEnv{}
}

// CreateCommonGenesisConfig generates a config starting point which templates can then customize further
func CreateCommonGenesisConfig(chainID *big.Int, adminAccountAddress common.Address, istanbulConfig params.IstanbulConfig) *Config {
	genesisConfig := BaseConfig()
	genesisConfig.ChainID = chainID
	genesisConfig.GenesisTimestamp = uint64(time.Now().Unix())
	genesisConfig.Istanbul = istanbulConfig
	genesisConfig.Hardforks = HardforkConfig{
		ChurritoBlock: common.Big0,
		DonutBlock:    common.Big0,
	}
	genesisConfig.Blockchain.UptimeLookbackWindow = genesisConfig.Istanbul.LookbackWindow

	// Make admin account manager of Governance & Reserve
	adminMultisig := MultiSigParameters{
		Signatories:                      []common.Address{adminAccountAddress},
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

func FundAccounts(genesisConfig *Config, accounts []env.Account) {
	cusdBalances := make([]Balance, len(accounts))
	ceurBalances := make([]Balance, len(accounts))
	goldBalances := make([]Balance, len(accounts))
	for i, acc := range accounts {
		cusdBalances[i] = Balance{Account: acc.Address, Amount: (*big.Int)(token.MustNew("50000"))} // 50k cUSD
		ceurBalances[i] = Balance{Account: acc.Address, Amount: (*big.Int)(token.MustNew("50000"))} // 50k cEUR
		goldBalances[i] = Balance{Account: acc.Address, Amount: (*big.Int)(token.MustNew("50000"))} // 50k CELO
	}
	genesisConfig.StableTokenEUR.InitialBalances = ceurBalances
	genesisConfig.StableToken.InitialBalances = cusdBalances
	genesisConfig.GoldToken.InitialBalances = goldBalances
}

type LocalEnv struct{}

func (e LocalEnv) CreateConfig() (*env.Config, error) {
	return &env.Config{
		Accounts: env.AccountsConfig{
			Mnemonic:             env.MustNewMnemonic(),
			NumValidators:        3,
			ValidatorsPerGroup:   1,
			NumDeveloperAccounts: 10,
		},
		ChainID: big.NewInt(1000 * (1 + rand.Int63n(9999))),
	}, nil
}

func (e LocalEnv) CreateGenesisConfig(env *env.Config) (*Config, error) {

	genesisConfig := CreateCommonGenesisConfig(env.ChainID, env.Accounts.AdminAccount().Address, params.IstanbulConfig{
		Epoch:          10,
		ProposerPolicy: 2,
		LookbackWindow: 3,
		BlockPeriod:    1,
		RequestTimeout: 3000,
	})

	// Add balances to developer accounts
	FundAccounts(genesisConfig, env.Accounts.DeveloperAccounts())

	return genesisConfig, nil
}

type LoadtestEnv struct{}

func (e LoadtestEnv) CreateConfig() (*env.Config, error) {
	return &env.Config{
		Accounts: env.AccountsConfig{
			Mnemonic:             "miss fire behind decide egg buyer honey seven advance uniform profit renew",
			NumValidators:        1,
			ValidatorsPerGroup:   1,
			NumDeveloperAccounts: 10000,
		},
		ChainID: big.NewInt(9099000),
	}, nil
}

func (e LoadtestEnv) CreateGenesisConfig(env *env.Config) (*Config, error) {
	genesisConfig := CreateCommonGenesisConfig(env.ChainID, env.Accounts.AdminAccount().Address, params.IstanbulConfig{
		Epoch:          1000,
		ProposerPolicy: 2,
		LookbackWindow: 3,
		BlockPeriod:    5,
		RequestTimeout: 3000,
	})

	// 10 billion gas limit, set super high on purpose
	genesisConfig.Blockchain.BlockGasLimit = 1000000000

	// Add balances to developer accounts
	FundAccounts(genesisConfig, env.Accounts.DeveloperAccounts())

	genesisConfig.StableToken.InflationFactorUpdatePeriod = 1 * Year

	// Disable gas price min being updated
	genesisConfig.GasPriceMinimum.TargetDensity = fixed.MustNew("0.9999")
	genesisConfig.GasPriceMinimum.AdjustmentSpeed = fixed.MustNew("0")

	return genesisConfig, nil
}

type MonorepoEnv struct{}

func (e MonorepoEnv) CreateConfig() (*env.Config, error) {
	return &env.Config{
		Accounts: env.AccountsConfig{
			Mnemonic:             env.MustNewMnemonic(),
			NumValidators:        3,
			ValidatorsPerGroup:   1,
			NumDeveloperAccounts: 0,
			UseValidatorAsAdmin:  true, // monorepo doesn't use the admin account type, uses first validator instead
		},
		ChainID: big.NewInt(1000 * (1 + rand.Int63n(9999))),
	}, nil
}

func (e MonorepoEnv) CreateGenesisConfig(env *env.Config) (*Config, error) {
	genesisConfig := CreateCommonGenesisConfig(env.ChainID, env.Accounts.AdminAccount().Address, params.IstanbulConfig{
		Epoch:          10,
		ProposerPolicy: 2,
		LookbackWindow: 3,
		BlockPeriod:    1,
		RequestTimeout: 3000,
	})

	// Add balances to validator accounts instead of developer accounts
	FundAccounts(genesisConfig, env.Accounts.ValidatorAccounts())

	return genesisConfig, nil
}
