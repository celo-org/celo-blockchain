package env

import (
	"math/big"
)

// Config represents mycelo environment parameters
type Config struct {
	ChainID    *big.Int         `json:"chainId"`    // chainId identifies the current chain and is used for replay protection
	Accounts   AccountsConfig   `json:"accounts"`   // Accounts configuration for the environment
	GethParams GethParamsConfig `json:"gethParams"` // Geth parameter configuration for the environment
	NodePort   int              `json:"nodePort"`   // Default value of Node port
	RPCPort    int              `json:"rpcPort"`    // Default value of RPC port
}

// AccountsConfig represents accounts configuration for the environment
type AccountsConfig struct {
	Mnemonic             string `json:"mnemonic"`            // Accounts mnemonic
	NumValidators        int    `json:"validators"`          // Number of initial validators
	ValidatorsPerGroup   int    `json:"validatorsPerGroup"`  // Number of validators per group in the initial set
	NumDeveloperAccounts int    `json:"developerAccounts"`   // Number of developers accounts
	UseValidatorAsAdmin  bool   `json:"useValidatorAsAdmin"` // Whether to use the first validator as the admin (for compatibility with monorepo)
}

// GethParamsConfig represents geth parameters configuration for the geth parameter
type GethParamsConfig struct {
	HTTPAddr string `json:"http.addr"` // Value of --http.addr option
	HTTPAPI  string `json:"http.api"`  // Value of --http.api option
}

// ValidatorGroup represents a group plus its validators members
type ValidatorGroup struct {
	Account
	Validators []Account
}

// NumValidatorGroups retrieves the number of validator groups for the genesis
func (ac *AccountsConfig) NumValidatorGroups() int {
	if (ac.NumValidators % ac.ValidatorsPerGroup) > 0 {
		return (ac.NumValidators / ac.ValidatorsPerGroup) + 1
	}
	return ac.NumValidators / ac.ValidatorsPerGroup
}

// AdminAccount returns the environment's admin account
func (ac *AccountsConfig) AdminAccount() *Account {
	at := AdminAT
	if ac.UseValidatorAsAdmin {
		at = ValidatorAT
	}
	acc, err := DeriveAccount(ac.Mnemonic, at, 0)
	if err != nil {
		panic(err)
	}
	return acc
}

// DeveloperAccounts returns the environment's developers accounts
func (ac *AccountsConfig) DeveloperAccounts() []Account {
	accounts, err := DeriveAccountList(ac.Mnemonic, DeveloperAT, ac.NumDeveloperAccounts)
	if err != nil {
		panic(err)
	}
	return accounts
}

// Account retrieves the account corresponding to the (accountType, idx)
func (ac *AccountsConfig) Account(accType AccountType, idx int) (*Account, error) {
	return DeriveAccount(ac.Mnemonic, accType, idx)
}

// ValidatorAccounts returns the environment's validators accounts
func (ac *AccountsConfig) ValidatorAccounts() []Account {
	accounts, err := DeriveAccountList(ac.Mnemonic, ValidatorAT, ac.NumValidators)
	if err != nil {
		panic(err)
	}
	return accounts
}

// ValidatorAccounts returns the environment's validators accounts
func (ac *AccountsConfig) TxFeeRecipientAccounts() []Account {
	accounts, err := DeriveAccountList(ac.Mnemonic, TxFeeRecipientAT, ac.NumValidators)
	if err != nil {
		panic(err)
	}
	return accounts
}

// ValidatorGroupAccounts returns the environment's validators group accounts
func (ac *AccountsConfig) ValidatorGroupAccounts() []Account {
	accounts, err := DeriveAccountList(ac.Mnemonic, ValidatorGroupAT, ac.NumValidatorGroups())
	if err != nil {
		panic(err)
	}
	return accounts
}

// ValidatorGroups return the list of validator groups on genesis
func (ac *AccountsConfig) ValidatorGroups() []ValidatorGroup {
	groups := make([]ValidatorGroup, ac.NumValidatorGroups())

	groupAccounts := ac.ValidatorGroupAccounts()
	validatorAccounts := ac.ValidatorAccounts()

	for i := 0; i < (len(groups) - 1); i++ {
		groups[i] = ValidatorGroup{
			Account:    groupAccounts[i],
			Validators: validatorAccounts[ac.ValidatorsPerGroup*i : ac.ValidatorsPerGroup*(i+1)],
		}
	}

	// last group might not be full, use an open slice for validators
	i := len(groups) - 1
	groups[i] = ValidatorGroup{
		Account:    groupAccounts[i],
		Validators: validatorAccounts[ac.ValidatorsPerGroup*i:],
	}

	return groups
}
