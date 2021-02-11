package env

import (
	"os"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/internal/fileutils"
	"github.com/ethereum/go-ethereum/mycelo/internal/utils"
)

type Environment struct {
	paths  paths
	Config Config

	// Derived Fields
	accounts *GenesisAccounts
}

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

func Load(envpath string) (*Environment, error) {
	env := &Environment{
		paths: paths{Workdir: envpath},
	}

	if err := utils.ReadJson(&env.Config, env.paths.envJson()); err != nil {
		return nil, err
	}

	accounts, err := createGenesisAccounts(&env.Config)
	if err != nil {
		return nil, err
	}

	env.accounts = accounts

	return env, nil
}

func (env *Environment) Save() error {
	env.ensureWorkdir()

	if err := utils.WriteJson(env.Config, env.paths.envJson()); err != nil {
		return err
	}
	return nil
}

func (env *Environment) Refresh() error {
	accounts, err := createGenesisAccounts(&env.Config)
	if err != nil {
		return err
	}
	env.accounts = accounts
	return nil
}

func (env *Environment) AdminAccount() Account { return env.accounts.Admin }

func (env *Environment) DeveloperAccounts() []Account { return env.accounts.Developers }

func (env *Environment) ValidatorAccounts() []Account { return env.accounts.Validators }

func (env *Environment) GenesisPath() string { return env.paths.genesisJSON() }

func (env *Environment) ValidatorDatadir(idx int) string { return env.paths.validatorDatadir(idx) }

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
