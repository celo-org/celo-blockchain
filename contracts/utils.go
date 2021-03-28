package contracts

import (
	"bytes"
	"errors"
	"math/big"
	"time"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
)

// SystemCaller is the caller when the EVM is invoked from the within the blockchain system.
var SystemCaller = common.HexToAddress("0x0")

type Contract struct {
	abi     *abi.ABI
	address common.Address
	caller  common.Address
}

type Backend interface {
	Call(caller vm.ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error)
	StaticCall(caller vm.ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error)
	StopGasMetering()
	StartGasMetering()
	GetStateDB() vm.StateDB
}

func NewContract(contractAbi *abi.ABI, contractAddress, defaultCaller common.Address) *Contract {
	return &Contract{
		abi:     contractAbi,
		address: contractAddress,
		caller:  defaultCaller,
	}
}

type ExecOpts struct {
	Backend Backend
	MaxGas  uint64
	Value   *big.Int
}
type QueryOpts struct {
	Backend Backend
	MaxGas  uint64
}

func (c *Contract) Execute(opts ExecOpts, result interface{}, method string, args ...interface{}) (uint64, error) {
	defer c.meterExecutionTime(method)()
	logger := log.New("method", method, "args", args)

	input, err := c.abi.Pack(method, args...)
	if err != nil {
		logger.Error("Error in generating the ABI encoding for the function call", "err", err)
		return 0, err
	}

	output, leftoverGas, err := opts.Backend.Call(vm.AccountRef(c.caller), c.address, input, opts.MaxGas, opts.Value)

	if err != nil {
		c.logCallError(logger, method, input, output, err)
		return leftoverGas, err
	}

	logger.Trace("EVM call successful", "input", hexutil.Encode(input), "output", hexutil.Encode(output))

	if result != nil {
		if err := c.abi.Unpack(result, method, output); err != nil {
			c.logUnpackError(logger, err)
			return leftoverGas, err
		}
	}

	return leftoverGas, nil
}

func (c *Contract) Query(opts QueryOpts, result interface{}, method string, args ...interface{}) (uint64, error) {
	defer c.meterExecutionTime(method)()
	logger := log.New("method", method, "args", args)

	input, err := c.abi.Pack(method, args...)
	if err != nil {
		logger.Error("Error in generating the ABI encoding for the function call", "err", err)
		return 0, err
	}

	output, leftoverGas, err := opts.Backend.StaticCall(vm.AccountRef(c.caller), c.address, input, opts.MaxGas)

	if err != nil {
		c.logCallError(logger, method, input, output, err)
		return leftoverGas, err
	}

	logger.Trace("EVM call successful", "input", hexutil.Encode(input), "output", hexutil.Encode(output))

	if result != nil {
		if err := c.abi.Unpack(result, method, output); err != nil {
			c.logUnpackError(logger, err)
			return leftoverGas, err
		}
	}

	return leftoverGas, nil
}

func (c *Contract) meterExecutionTime(method string) func() {
	// Record a metrics data point about execution time.
	timer := metrics.GetOrRegisterTimer("contract_comm/systemcall/"+method, nil)
	start := time.Now()
	return func() { timer.UpdateSince(start) }
}

func (c *Contract) logUnpackError(logger log.Logger, err error) {
	// TODO (mcortesi) Remove ErrEmptyArguments check after we change Proxy to fail on unset impl
	// `ErrEmptyArguments` is expected when when syncing & importing blocks
	// before a contract has been deployed
	if err == abi.ErrEmptyArguments {
		logger.Trace("Error in unpacking EVM call return bytes", "err", err)
	} else {
		logger.Error("Error in unpacking EVM call return bytes", "err", err)
	}
}

func (c *Contract) logCallError(logger log.Logger, method string, input, output []byte, err error) {
	msg, _ := unpackError(output)
	// Do not log execution reverted as error for getAddressFor. This only happens before the Registry is deployed.
	// TODO(nategraf): Find a more generic and complete solution to the problem of logging tolerated EVM call failures.
	if method == "getAddressFor" {
		logger.Trace("Error in calling the EVM", "input", hexutil.Encode(input), "err", err, "msg", msg)
	} else {
		logger.Error("Error in calling the EVM", "input", hexutil.Encode(input), "err", err, "msg", msg)
	}
}

var (
	errorSig     = []byte{0x08, 0xc3, 0x79, 0xa0} // Keccak256("Error(string)")[:4]
	abiString, _ = abi.NewType("string", "", nil)
)

func unpackError(result []byte) (string, error) {
	if len(result) < 4 || !bytes.Equal(result[:4], errorSig) {
		return "<tx result not Error(string)>", errors.New("TX result not of type Error(string)")
	}
	vs, err := abi.Arguments{{Type: abiString}}.UnpackValues(result[4:])
	if err != nil {
		return "<invalid tx result>", err
	}
	return vs[0].(string), nil
}
