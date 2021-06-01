package testutil

import (
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/vm"
)

// decapitalise makes a camel-case string which starts with a lower case character.
func decapitalise(input string) string {
	if len(input) == 0 {
		return input
	}

	goForm := abi.ToCamelCase(input)
	return strings.ToLower(goForm[:1]) + goForm[1:]
}

type mockStateDB struct {
	vm.StateDB
	isContract func(common.Address) bool
}

func (msdb *mockStateDB) GetCodeSize(addr common.Address) int {
	if msdb.isContract(addr) {
		return 100
	}
	return 0
}

func (msdb *mockStateDB) Finalise(bool) {}
