package blockchain_params

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/hashicorp/go-version"
)

const (
	blockchainParamsABIString = `[{
		"constant": true,
		"inputs": [],
		"name": "minimumClientVersion",
		"outputs": [
		  {
			"name": "",
			"type": "string"
		  }
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	  }]`
)

const defaultGasAmount = 2000000

var (
	blockchainParamsABI, _ = abi.JSON(strings.NewReader(blockchainParamsABIString))
)

func CheckMinimumVersion(header *types.Header, state vm.StateDB) error {
	var minVersion string
	var err error
	_, err = contract_comm.MakeStaticCall(
		params.BlockchainParamsRegistryId,
		blockchainParamsABI,
		"minimumClientVersion",
		[]interface{}{},
		&minVersion,
		defaultGasAmount,
		header,
		state,
	)

	if err != nil {
		log.Warn("Error checking client version", "err", err, "contract id", params.BlockchainParamsRegistryId)
		return nil
	}

	log.Warn("Version", "version", minVersion)
	_, err = version.NewVersion(minVersion)

	return err
}
