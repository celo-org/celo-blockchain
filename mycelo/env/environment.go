package env

import (
	"os"

	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/internal/fileutils"
	"github.com/celo-org/celo-blockchain/mycelo/internal/utils"
)

// Environment represents a mycelo environment
// which is the home to any mycelo operation within a computer
type Environment struct {
	paths  paths
	Config Config

	// Derived Fields
	accounts *GenesisAccounts
}

// New creates a new environment
func New(envpath string, cfg *Config) (*Environment, error) {
	env := &Environment{
		paths:  paths{Workdir: envpath},
		Config: *cfg,
	}

	if err := env.Refresh(); err != nil {
		return nil, err
	}
	return env, nil
}

// Load will load an environment located in envpath folder
func Load(envpath string) (*Environment, error) {
	env := &Environment{
		paths: paths{Workdir: envpath},
	}

	if err := utils.ReadJson(&env.Config, env.paths.envJSON()); err != nil {
		return nil, err
	}

	accounts, err := createGenesisAccounts(&env.Config)
	if err != nil {
		return nil, err
	}

	env.accounts = accounts

	return env, nil
}

// Save will save environment's config into the environment folder
func (env *Environment) Save() error {
	env.ensureWorkdir()

	if err := utils.WriteJson(env.Config, env.paths.envJSON()); err != nil {
		return err
	}
	return nil
}

// Refresh recomputes environment derived values
// Use this function after modifing the env.Config
func (env *Environment) Refresh() error {
	accounts, err := createGenesisAccounts(&env.Config)
	if err != nil {
		return err
	}
	env.accounts = accounts
	return nil
}

// AdminAccount returns the environment's admin account
func (env *Environment) AdminAccount() Account { return env.accounts.Admin }

// DeveloperAccounts returns the environment's developers accounts
func (env *Environment) DeveloperAccounts() []Account { return env.accounts.Developers }

// ValidatorAccounts returns the environment's validators accounts
func (env *Environment) ValidatorAccounts() []Account { return env.accounts.Validators }

// GenesisPath returns the paths to the genesis.json file (if present on the environment)
func (env *Environment) GenesisPath() string { return env.paths.genesisJSON() }

// ValidatorDatadir returns the datadir that mycelo uses to run the validator-[idx]
func (env *Environment) ValidatorDatadir(idx int) string { return env.paths.validatorDatadir(idx) }

// SaveGenesis writes genesis.json within the env.json
func (env *Environment) SaveGenesis(genesis *core.Genesis) error {
	env.ensureWorkdir()
	if err := utils.WriteJson(genesis, env.paths.genesisJSON()); err != nil {
		return err
	}
	return nil
}

func (env *Environment) ensureWorkdir() {
	if !fileutils.FileExists(env.paths.Workdir) {
		os.MkdirAll(env.paths.Workdir, os.ModePerm)
	}
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
