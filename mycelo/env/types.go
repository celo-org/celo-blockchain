package env

type GenesisAccounts struct {
	Admin           Account   // Administrative account that deploys all initial contracts and owner of them if governance is not enabled
	Validators      []Account // Accounts for all validators
	ValidatorGroups []Account // Accounts for all validator groups
	Developers      []Account // Accounts for developers to play with
}
