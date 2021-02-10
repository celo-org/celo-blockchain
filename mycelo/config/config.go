package config

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/internal/fileutils"
)

type Environment struct {
	Paths  Paths
	Config Config

	// Derived Fields
	accounts *GenesisAccounts
}

func NewEnv(envpath string, cfg *Config) (*Environment, error) {
	env := &Environment{
		Paths:  Paths{Workdir: envpath},
		Config: *cfg,
	}

	accounts, err := createGenesisAccounts(cfg)
	if err != nil {
		return nil, err
	}

	env.accounts = accounts
	return env, nil
}

func (env *Environment) CreateGenesisAccounts() error {
	accounts, err := createGenesisAccounts(&env.Config)
	if err != nil {
		return err
	}
	env.accounts = accounts
	return nil
}

func (env *Environment) WriteEnv() error {
	if !fileutils.FileExists(env.Paths.Workdir) {
		os.MkdirAll(env.Paths.Workdir, os.ModePerm)
	}

	if err := WriteJson(env.Config, env.Paths.Config()); err != nil {
		return err
	}
	return nil
}

func ReadEnv(envpath string) (*Environment, error) {
	env := &Environment{
		Paths: Paths{Workdir: envpath},
	}

	if err := ReadJson(&env.Config, env.Paths.Config()); err != nil {
		return nil, err
	}

	accounts, err := createGenesisAccounts(&env.Config)
	if err != nil {
		return nil, err
	}

	env.accounts = accounts

	return env, nil
}

func (env *Environment) AdminAccount() Account        { return env.accounts.Admin }
func (env *Environment) DeveloperAccounts() []Account { return env.accounts.Developers }
func (env *Environment) ValidatorAccounts() []Account { return env.accounts.Validators }

// Config represents MyCelo configuration parameters
type Config struct {
	ChainID            *big.Int `json:"chainId"`            // chainId identifies the current chain and is used for replay protection
	Mnemonic           string   `json:"mnemonic"`           // Accounts mnemonic
	InitialValidators  int      `json:"initialValidators"`  // Number of initial validators
	ValidatorsPerGroup int      `json:"validatorsPerGroup"` // Number of validators per group in the initial set
	DeveloperAccounts  int      `json:"developerAccounts"`  // Number of developers accounts

}

func ReadJson(out interface{}, filepath string) error {
	byteValue, err := ioutil.ReadFile(filepath)
	if err != nil {
		return err
	}

	return json.Unmarshal(byteValue, out)
}

func WriteJson(in interface{}, filepath string) error {
	byteValue, err := json.MarshalIndent(in, " ", " ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath, byteValue, 0644)
}

func createGenesisAccounts(cfg *Config) (*GenesisAccounts, error) {
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
