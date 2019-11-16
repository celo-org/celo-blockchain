package gold_token

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/GoldToken.json
	increaseSupplyABI = `[{
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
		}]`

	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/GoldToken.json
	totalSupplyABI = `[{
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
	  }]`
)

var (
	increaseSupplyFuncABI, _ = abi.JSON(strings.NewReader(increaseSupplyABI))
	totalSupplyFuncABI, _    = abi.JSON(strings.NewReader(totalSupplyABI))
)

func GetTotalSupply(header *types.Header, state vm.StateDB) (*big.Int, error) {
	var totalSupply *big.Int
	_, err := contract_comm.MakeStaticCall(
		params.GoldTokenRegistryId,
		totalSupplyFuncABI,
		"totalSupply",
		[]interface{}{},
		&totalSupply,
		params.MaxGasForTotalSupply,
		header,
		state,
	)
	return totalSupply, err
}

func IncreaseSupply(header *types.Header, state vm.StateDB, value *big.Int) error {
	_, err := contract_comm.MakeCall(
		params.GoldTokenRegistryId,
		increaseSupplyFuncABI,
		"increaseSupply",
		[]interface{}{value},
		nil,
		params.MaxGasForIncreaseSupply,
		common.Big0,
		header,
		state,
		false,
	)
	return err
}
