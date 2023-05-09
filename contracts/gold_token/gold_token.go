package gold_token

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/core/vm"
)

var (
	totalSupplyMethod    = contracts.NewRegisteredContractMethod(config.GoldTokenRegistryId, abis.GoldToken, "totalSupply", config.MaxGasForTotalSupply)
	increaseSupplyMethod = contracts.NewRegisteredContractMethod(config.GoldTokenRegistryId, abis.GoldToken, "increaseSupply", config.MaxGasForIncreaseSupply)
	mintMethod           = contracts.NewRegisteredContractMethod(config.GoldTokenRegistryId, abis.GoldToken, "mint", config.MaxGasForMintGas)
)

func GetTotalSupply(vmRunner vm.EVMRunner) (*big.Int, error) {
	var totalSupply *big.Int
	err := totalSupplyMethod.Query(vmRunner, &totalSupply)
	return totalSupply, err
}

func IncreaseSupply(vmRunner vm.EVMRunner, value *big.Int) error {
	err := increaseSupplyMethod.Execute(vmRunner, nil, common.Big0, value)
	return err
}

func Mint(vmRunner vm.EVMRunner, beneficiary common.Address, value *big.Int) error {
	if value.Cmp(new(big.Int)) <= 0 {
		return nil
	}

	err := mintMethod.Execute(vmRunner, nil, common.Big0, beneficiary, value)
	return err
}
