package env

import "math/big"

// GenesisAccounts represents the account that were generated on the genesis block
type GenesisAccounts struct {
	Admin           Account   // Administrative account that deploys all initial contracts and owner of them if governance is not enabled
	Validators      []Account // Accounts for all validators
	ValidatorGroups []Account // Accounts for all validator groups
	Developers      []Account // Accounts for developers to play with
}

// Config represents mycelo environment parameters
type Config struct {
	ChainID            *big.Int `json:"chainId"`            // chainId identifies the current chain and is used for replay protection
	Mnemonic           string   `json:"mnemonic"`           // Accounts mnemonic
	InitialValidators  int      `json:"initialValidators"`  // Number of initial validators
	ValidatorsPerGroup int      `json:"validatorsPerGroup"` // Number of validators per group in the initial set
	DeveloperAccounts  int      `json:"developerAccounts"`  // Number of developers accounts

}
