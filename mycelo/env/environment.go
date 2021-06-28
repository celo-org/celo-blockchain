package env

import (
	"os"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/internal/fileutils"
	"github.com/ethereum/go-ethereum/mycelo/internal/utils"
)

// Environment represents a mycelo environment
// which is the home to any mycelo operation within a computer
type Environment struct {
	paths  paths
	Config Config
}

// New creates a new environment
func New(envpath string, cfg *Config) (*Environment, error) {
	env := &Environment{
		paths:  paths{Workdir: envpath},
		Config: *cfg,
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

// Accounts retrieves accounts config
func (env *Environment) Accounts() *AccountsConfig { return &env.Config.Accounts }

// GenesisPath returns the paths to the genesis.json file (if present on the environment)
func (env *Environment) GenesisPath() string { return env.paths.genesisJSON() }

// ValidatorDatadir returns the datadir that mycelo uses to run the validator-[idx]
func (env *Environment) ValidatorDatadir(idx int) string { return env.paths.validatorDatadir(idx) }

// ValidatorIPC returns the ipc path to validator-[idx]
func (env *Environment) ValidatorIPC(idx int) string { return env.paths.validatorIPC(idx) }

// IPC returns the IPC path to the first validator
func (env *Environment) IPC() string { return env.paths.validatorIPC(0) }

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
