package config

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/internal/fileutils"
	"github.com/ethereum/go-ethereum/params"
)

type Environment struct {
	Paths         Paths
	EnvConfig     EnvConfig
	GenesisConfig GenesisConfig

	// Derived Fields
	accounts *GenesisAccounts
}

func NewEnv(envpath string, cfg *EnvConfig) (*Environment, error) {
	env := &Environment{
		Paths:         Paths{Workdir: envpath},
		EnvConfig:     *cfg,
		GenesisConfig: *BaseContractsConfig(),
	}

	accounts, err := createGenesisAccounts(cfg)
	if err != nil {
		return nil, err
	}

	env.accounts = accounts
	return env, nil
}

func (env *Environment) WriteGenesisConfig() error {
	if !fileutils.FileExists(env.Paths.Workdir) {
		os.MkdirAll(env.Paths.Workdir, os.ModePerm)
	}

	if err := writeJson(env.GenesisConfig, env.Paths.GenesisConfig()); err != nil {
		return err
	}
	return nil
}

func (env *Environment) WriteEnvConfig() error {
	if !fileutils.FileExists(env.Paths.Workdir) {
		os.MkdirAll(env.Paths.Workdir, os.ModePerm)
	}

	if err := writeJson(env.EnvConfig, env.Paths.EnvConfig()); err != nil {
		return err
	}
	return nil
}

func ReadBuildEnv(envpath string) (*Environment, error) {
	env := &Environment{
		Paths: Paths{Workdir: envpath},
	}

	if err := readJson(&env.EnvConfig, env.Paths.EnvConfig()); err != nil {
		return nil, err
	}

	if err := readJson(&env.GenesisConfig, env.Paths.GenesisConfig()); err != nil {
		return nil, err
	}

	accounts, err := createGenesisAccounts(&env.EnvConfig)
	if err != nil {
		return nil, err
	}

	env.accounts = accounts

	return env, nil
}

func ReadEnv(envpath string) (*Environment, error) {
	env := &Environment{
		Paths: Paths{Workdir: envpath},
	}

	if err := readJson(&env.EnvConfig, env.Paths.EnvConfig()); err != nil {
		return nil, err
	}

	accounts, err := createGenesisAccounts(&env.EnvConfig)
	if err != nil {
		return nil, err
	}

	env.accounts = accounts

	return env, nil
}

// ChainConfig returns chain config for the environment
func (env *Environment) ChainConfig() *params.ChainConfig { return env.GenesisConfig.ChainConfig() }

func (env *Environment) AdminAccount() Account        { return env.accounts.Admin }
func (env *Environment) DeveloperAccounts() []Account { return env.accounts.Developers }
func (env *Environment) ValidatorAccounts() []Account { return env.accounts.Validators }

// EnvConfig represents MyCelo configuration parameters
type EnvConfig struct {
	ChainID            *big.Int `json:"chainId"`            // chainId identifies the current chain and is used for replay protection
	Mnemonic           string   `json:"mnemonic"`           // Accounts mnemonic
	InitialValidators  int      `json:"initialValidators"`  // Number of initial validators
	ValidatorsPerGroup int      `json:"validatorsPerGroup"` // Number of validators per group in the initial set
	DeveloperAccounts  int      `json:"developerAccounts"`  // Number of developers accounts

}

func readJson(out interface{}, filepath string) error {
	byteValue, err := ioutil.ReadFile(filepath)
	if err != nil {
		return err
	}

	return json.Unmarshal(byteValue, out)
}

func writeJson(in interface{}, filepath string) error {
	byteValue, err := json.MarshalIndent(in, " ", " ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath, byteValue, 0644)
}

func DefaultConfig() *EnvConfig {
	var cfg EnvConfig
	cfg.ApplyDefaults()
	return &cfg
}

func (cfg *EnvConfig) ApplyDefaults() {
	if cfg.Mnemonic == "" {
		cfg.Mnemonic = MustNewMnemonic()
	}

	if cfg.DeveloperAccounts == 0 {
		cfg.DeveloperAccounts = 10
	}
	if cfg.InitialValidators == 0 {
		cfg.InitialValidators = 3
	}
	if cfg.ValidatorsPerGroup == 0 {
		cfg.ValidatorsPerGroup = 1
	} else {
		if cfg.InitialValidators%cfg.ValidatorsPerGroup != 0 {
			panic("For simplicity. Each ValidatorGroup must have same number of validators")
		}
	}
}

func createGenesisAccounts(cfg *EnvConfig) (*GenesisAccounts, error) {
	admin, err := GenerateAccounts(cfg.Mnemonic, Admin, 1)
	if err != nil {
		return nil, err
	}

	validators, err := GenerateAccounts(cfg.Mnemonic, Validator, cfg.InitialValidators)
	if err != nil {
		return nil, err
	}

	groups, err := GenerateAccounts(cfg.Mnemonic, ValidatorGroup, cfg.InitialValidators/cfg.ValidatorsPerGroup)
	if err != nil {
		return nil, err
	}

	developers, err := GenerateAccounts(cfg.Mnemonic, Developer, cfg.DeveloperAccounts)
	if err != nil {
		return nil, err
	}

	return &GenesisAccounts{
		Admin:           admin[0],
		Validators:      validators,
		ValidatorGroups: groups,
		Developers:      developers,
	}, nil

}
