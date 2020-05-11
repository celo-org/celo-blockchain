package sorted_oracles

import (
	"math/big"
	"reflect"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/contract_comm/errors"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const sortedOraclesJSON = `[
	{
		"constant": true,
		"inputs": [
		  {
			"name": "",
			"type": "address"
		  },
		  {
			"name": "",
			"type": "address"
		  }
		],
		"name": "isOracle",
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
	"constant": false,
	"inputs": [
	  {
		"name": "token",
		"type": "address"
	  },
	  {
		"name": "value",
		"type": "uint256"
	  },
	  {
		"name": "lesserKey",
		"type": "address"
	  },
	  {
		"name": "greaterKey",
		"type": "address"
	  }
	],
	"name": "report",
	"outputs": [],
	"payable": false,
	"stateMutability": "nonpayable",
	"type": "function"
  },
  {
	"constant": false,
	"inputs": [
	  {
		"name": "token",
		"type": "address"
	  },
	  {
		"name": "n",
		"type": "uint256"
	  }
	],
	"name": "removeExpiredReports",
	"outputs": [],
	"payable": false,
	"stateMutability": "nonpayable",
	"type": "function"
  }
]`

var (
	sortedOraclesABI, _ = abi.JSON(strings.NewReader(sortedOraclesJSON))
)

func GetAddress() *common.Address {
	address, err := contract_comm.GetRegisteredAddress(params.SortedOraclesRegistryId, nil, nil)
	if err == errors.ErrSmartContractNotDeployed || err == errors.ErrRegistryContractNotDeployed {
		log.Debug("Registry address lookup failed", "err", err, "contract", hexutil.Encode(params.SortedOraclesRegistryId[:]))
	} else if err != nil {
		log.Error(err.Error())
	}
	return address
}

func IsOracle(token *common.Address, addr *common.Address, header *types.Header, state vm.StateDB) bool {
	var isOracle = reflect.New(reflect.TypeOf(true))
	log.Warn("Testing", "token", token, "addr", addr)
	_, err := contract_comm.MakeStaticCall(
		params.SortedOraclesRegistryId,
		sortedOraclesABI,
		"isOracle",
		[]interface{}{token, addr},
		isOracle.Interface(),
		params.MaxGasForGetGasPriceMinimum,
		header,
		state,
	)
	if err != nil {
		log.Warn("Cannot determine if address is an oracle", "err", err)
		return false
	}
	return isOracle.Elem().Bool()
}

func GetTokenFromTxData(data []byte) (*common.Address, error) {
	if len(data) < 4 {
		return nil, nil
	}
	method, err := sortedOraclesABI.MethodById(data)
	if err != nil {
		log.Debug("Unknown method", "err", err)
		return nil, err
	}
	if method.Name == "report" {
		type ReportArgs struct {
			Token      common.Address
			Value      *big.Int
			LesserKey  common.Address
			GreaterKey common.Address
		}
		var res ReportArgs
		err = method.Inputs.Unpack(&res, data[4:])
		if err != nil {
			log.Warn("Bad data", "method", method.Name, "err", err)
			return nil, err
		}
		return &res.Token, nil
	}
	if method.Name == "removeExpiredReports" {
		type RemoveArgs struct {
			Token common.Address
			N     *big.Int
		}
		var res RemoveArgs
		err = method.Inputs.Unpack(&res, data[4:])
		if err != nil {
			log.Warn("Bad data", "method", method.Name, "err", err)
			return nil, err
		}
		return &res.Token, nil
	}
	return nil, nil
}
