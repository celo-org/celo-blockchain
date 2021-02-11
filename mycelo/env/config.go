package env

import (
	"math/big"
)

// Config represents MyCelo configuration parameters
type Config struct {
	ChainID            *big.Int `json:"chainId"`            // chainId identifies the current chain and is used for replay protection
	Mnemonic           string   `json:"mnemonic"`           // Accounts mnemonic
	InitialValidators  int      `json:"initialValidators"`  // Number of initial validators
	ValidatorsPerGroup int      `json:"validatorsPerGroup"` // Number of validators per group in the initial set
	DeveloperAccounts  int      `json:"developerAccounts"`  // Number of developers accounts

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
