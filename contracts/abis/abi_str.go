package abis

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Registry.json
const RegistryStr = `[
	{
		"constant": true,
		"inputs": [
			{
				"name": "identifier",
				"type": "bytes32"
			}
		],
		"name": "getAddressFor",
		"outputs": [
			{
				"name": "",
				"type": "address"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`

const BlockchainParametersStr = `[
	{
		"constant": true,
		"inputs": [],
		"name": "getMinimumClientVersion",
		"outputs": [
			{
			"name": "major",
			"type": "uint256"
			},
			{
			"name": "minor",
			"type": "uint256"
			},
			{
			"name": "patch",
			"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "blockGasLimit",
		"outputs": [
			{
			"name": "",
			"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "getUptimeLookbackWindow",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "lookbackWindow",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "intrinsicGasForAlternativeFeeCurrency",
		"outputs": [
			{
				"name": "",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/SortedOracles.json
const SortedOraclesStr = `[
	{
		"constant": true,
		"inputs": [
			{
				"name": "token",
				"type": "address"
			}
		],
		"name": "medianRate",
		"outputs": [
			{
				"name": "",
				"type": "uint128"
			},
			{
				"name": "",
				"type": "uint128"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/ERC20.json
const ERC20Str = `[
	{
		"constant": true,
		"inputs": [
			{
				"name": "who",
				"type": "address"
			}
		],
		"name": "balanceOf",
		"outputs": [
			{
				"name": "",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
}]`

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/FeeCurrency.json
const FeeCurrencyStr = `[
	{
		"constant": true,
		"inputs": [],
		"name": "getWhitelist",
		"outputs": [
			{
				"name": "",
				"type": "address[]"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`

const ElectionsStr string = `[
	{
		"constant": true,
		"inputs": [],
		"name": "electValidatorSigners",
		"outputs": [
			{
				"name": "",
				"type": "address[]"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "getTotalVotesForEligibleValidatorGroups",
		"outputs": [
			{
				"name": "groups",
				"type": "address[]"
			},
			{
				"name": "values",
				"type": "uint256[]"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [
			{
				"name": "group",
				"type": "address"
			},
			{
				"name": "value",
				"type": "uint256"
			},
			{
				"name": "lesser",
				"type": "address"
			},
			{
				"name": "greater",
				"type": "address"
			}
		],
		"name": "distributeEpochRewards",
		"outputs": [],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [
			{
				"name": "group",
				"type": "address"
			},
			{
				"name": "maxTotalRewards",
				"type": "uint256"
			},
			{
				"name": "uptimes",
				"type": "uint256[]"
			}
		],
		"name": "getGroupEpochRewards",
		"outputs": [
			{
				"name": "",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [
			{
				"name": "minElectableValidators",
				"type": "uint256"
			},
			{
				"name": "maxElectableValidators",
				"type": "uint256"
			}
		],
		"name": "electNValidatorSigners",
		"outputs": [
			{
				"name": "",
				"type": "address[]"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "getElectableValidators",
		"outputs": [
			{
				"name": "",
				"type": "uint256"
			},
			{
				"name": "",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/EpochRewards.json
const EpochRewardsStr string = `[
	{
		"constant": true,
		"inputs": [],
		"name": "calculateTargetEpochRewards",
		"outputs": [
			{
				"name": "",
				"type": "uint256"
			},
			{
				"name": "",
				"type": "uint256"
			},
			{
				"name": "",
				"type": "uint256"
			},
			{
				"name": "",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{ 
		"constant": true,
		"inputs": [],
		"name": "carbonOffsettingPartner",
		"outputs": [
			{ 
				"name": "",
				"type": "address"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [],
		"name": "updateTargetVotingYield",
		"outputs": [],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "isReserveLow",
		"outputs": [
			{
				"name": "",
				"type": "bool"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "frozen",
		"outputs": [
			{
				"name": "",
				"type": "bool"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]
`

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Freezer.json
const FreezerStr = `[
	{
		"constant": true,
		"inputs": [
			{
				"name": "",
				"type": "address"
			}
		],
		"name": "isFrozen",
		"outputs": [
			{
				"name": "",
				"type": "bool"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`

const GasPriceMinimumStr = `[
	{
		"constant": true,
		"inputs": [
			{
				"name": "_tokenAddress",
				"type": "address"
			}
		],
		"name": "getGasPriceMinimum",
		"outputs": [
			{
				"name": "",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [
			{
				"name": "_blockGasTotal",
				"type": "uint256"
			},
			{
				"name": "_blockGasLimit",
				"type": "uint256"
			}
		],
		"name": "updateGasPriceMinimum",
		"outputs": [
			{
				"name": "",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
		}
]`

// nolint: gosec
const GoldTokenStr = `[
	{
		"constant": false,
		"inputs": [
		  {
			"name": "amount",
			"type": "uint256"
		  }
		],
		"name": "increaseSupply",
		"outputs": [],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [
			{
				"name": "to",
				"type": "address"
			},
			{
				"name": "value",
				"type": "uint256"
			}
		],
		"name": "mint",
		"outputs": [
			{
				"name": "",
				"type": "bool"
			}
		],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "totalSupply",
		"outputs": [
		  {
			"name": "",
			"type": "uint256"
		  }
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`

const RandomStr = `[
	{
		"constant": false,
		"inputs": [
			{
				"name": "randomness",
				"type": "bytes32"
			},
			{
				"name": "newCommitment",
				"type": "bytes32"
			},
			{
				"name": "proposer",
				"type": "address"
			}
		],
		"name": "revealAndCommit",
		"outputs": [],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [
			{
				"name": "",
				"type": "address"
			}
		],
		"name": "commitments",
		"outputs": [
			{
				"name": "",
				"type": "bytes32"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [
			{
				"name": "randomness",
				"type": "bytes32"
			}
		],
		"name": "computeCommitment",
		"outputs": [
			{
				"name": "",
				"type": "bytes32"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "random",
		"outputs": [
			{
				"name": "",
				"type": "bytes32"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [
			{
				"name": "blockNumber",
				"type": "uint256"
			}
		],
		"name": "getBlockRandomness",
		"outputs": [
			{
				"name": "",
				"type": "bytes32"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Validators.json
const ValidatorsStr = `[
	{
		"constant": true,
		"inputs": [],
		"name": "getRegisteredValidatorSigners",
		"outputs": [
			{
				"name": "",
				"type": "address[]"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "getRegisteredValidators",
		"outputs": [
			{
				"name": "",
				"type": "address[]"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [
			{
				"name": "signer",
				"type": "address"
			}
		],
		"name": "getValidatorBlsPublicKeyFromSigner",
		"outputs": [
			{
				"name": "blsKey",
				"type": "bytes"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [
			{
				"name": "account",
				"type": "address"
			}
		],
		"name": "getValidator",
		"outputs": [
			{
				"name": "ecdsaPublicKey",
				"type": "bytes"
			},
			{
				"name": "blsPublicKey",
				"type": "bytes"
			},
			{
				"name": "affiliation",
				"type": "address"
			},
			{
				"name": "score",
				"type": "uint256"
			},
			{
				"name": "signer",
				"type": "address"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [
			{
				"name": "validator",
				"type": "address"
			},
			{
				"name": "maxPayment",
				"type": "uint256"
			}
		],
		"name": "distributeEpochPaymentsFromSigner",
		"outputs": [
			{
				"name": "",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [
			{
				"name": "validator",
				"type": "address"
			},
			{
				"name": "uptime",
				"type": "uint256"
			}
		],
		"name": "updateValidatorScoreFromSigner",
		"outputs": [],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [
			{
				"name": "account",
				"type": "address"
			}
		],
		"name": "getMembershipInLastEpochFromSigner",
		"outputs": [
			{
				"name": "",
				"type": "address"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`
