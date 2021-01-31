package config

type GenesisAccounts struct {
	Deployer        Account   // Account that deploys all initial contracts and owner of them if governance is not enabled
	Validators      []Account // Accounts for all validators
	ValidatorGroups []Account // Accounts for all validator groups
}
