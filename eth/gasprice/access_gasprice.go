





package gasprice


import (
  "strings"
  "context"
  "math/big"

  "github.com/ethereum/go-ethereum/params"
  "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core"
)

// TODO (jarmg 5/22/18): Store contract function ABIs in a central location
var (
  getGasPriceABIString = `[{
    "constant": true,
    "inputs": [],
    "name": "getGasPriceSuggestion",
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

func GetGasPrice(ctx context.Context, iEvmH *core.InternalEVMHandler, regAdd *core.RegisteredAddresses) (*big.Int, error) {

	var gasPrice *big.Int
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)
	var (
		gasPriceOracleABI, _ = abi.JSON(strings.NewReader(getGasPriceABIString))
		_, err               = iEvmH.MakeCall(*gasPriceOracleAddress, gasPriceOracleABI, "getGasPriceSuggestion", []interface{}{}, &gasPrice, 2000, nil, nil)
	)

	return gasPrice, err
}

