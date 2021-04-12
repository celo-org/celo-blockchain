package contracts

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

// Call represent a runnable call on the EVM
type Call interface {
	// Run will execute the call using the caller.
	// Execution result will be populated into result
	Run(caller vm.EVMCaller, result interface{}) (leftoverGas uint64, err error)
}

// QueryCallFromVM creates a contract Call that peforms a Query using the VMAddress as from
func QueryCallFromVM(to common.Address, maxGas uint64, msg Message) Call {
	return &contractCall{
		readOnly: true,
		from:     VMAddress,
		to:       to,
		maxGas:   maxGas,
		msg:      msg,
	}
}

// WriteCallFromVM creates a contract Call that perfoms a Write operation using the VMAddress as from
func WriteCallFromVM(to common.Address, maxGas uint64, value *big.Int, msg Message) Call {
	return &contractCall{
		readOnly: false,
		from:     VMAddress,
		to:       to,
		maxGas:   maxGas,
		msg:      msg,
		value:    value,
	}
}

// QueryCallOnRegisteredContract creates a contract Call that performs a Query on a contract from the registry
func QueryCallOnRegisteredContract(to common.Hash, maxGas uint64, msg Message) Call {
	return &registeredContractCall{
		registryId: to,
		callTemplate: contractCall{
			readOnly: true,
			from:     VMAddress,
			maxGas:   maxGas,
			msg:      msg,
		},
	}
}

// WriteCallOnRegisteredContract creates a contract Call that performs a Write operation on a contract from the registry
func WriteCallOnRegisteredContract(to common.Hash, maxGas uint64, value *big.Int, msg Message) Call {
	return &registeredContractCall{
		registryId: to,
		callTemplate: contractCall{
			readOnly: false,
			from:     VMAddress,
			maxGas:   maxGas,
			value:    value,
			msg:      msg,
		},
	}
}

// Message represents a msg to a contract
type Message struct {
	abi    *abi.ABI
	method string
	args   []interface{}
}

// NewMessage creates a new contract message
func NewMessage(abi *abi.ABI, method string, args ...interface{}) Message {
	return Message{
		abi:    abi,
		method: method,
		args:   args,
	}
}

// encodeCall will encodes the msg into []byte format for EVM consumption
func (am Message) encodeCall() ([]byte, error) { return am.abi.Pack(am.method, am.args...) }

// decodeResult will decode the result of msg execution into the result parameter
func (am Message) decodeResult(result interface{}, output []byte) error {
	if result == nil {
		return nil
	}
	return am.abi.Unpack(result, am.method, output)
}

// registeredContractCall represents a Call to a contract in the registry
type registeredContractCall struct {
	registryId   common.Hash
	callTemplate contractCall
}

// Run will execute the call using the caller.
// Execution result will be populated into result
func (rc *registeredContractCall) Run(caller vm.EVMCaller, result interface{}) (uint64, error) {
	contractAddress, err := resolveAddressForCall(caller, rc.registryId, rc.callTemplate.msg.method)
	if err != nil {
		return 0, err
	}

	// copy call template
	call := rc.callTemplate
	call.to = contractAddress

	return call.Run(caller, result)
}

// contractCall represents a Call to a contract
type contractCall struct {
	readOnly bool
	from     common.Address
	to       common.Address
	msg      Message
	value    *big.Int
	maxGas   uint64
}

// Run will execute the call using the caller.
// Execution result will be populated into result
func (call *contractCall) Run(caller vm.EVMCaller, result interface{}) (uint64, error) {

	defer meterExecutionTime(call.msg.method)()
	logger := log.New("to", call.to, "method", call.msg.method, "args", call.msg.args, "maxgas", call.maxGas)

	input, err := call.msg.encodeCall()
	if err != nil {
		logger.Error("Error invoking evm function: can't encode method arguments", "err", err)
		return 0, err
	}

	var output []byte
	var leftoverGas uint64
	if call.readOnly {
		output, leftoverGas, err = caller.StaticCall(vm.AccountRef(call.from), call.to, input, call.maxGas)
	} else {
		output, leftoverGas, err = caller.Call(vm.AccountRef(call.from), call.to, input, call.maxGas, call.value)
	}

	if err != nil {
		msg, _ := unpackError(output)
		logger.Error("Error invoking evm function: EVM call failure", "input", hexutil.Encode(input), "err", err, "msg", msg)
		return leftoverGas, err
	}

	if err := call.msg.decodeResult(result, output); err != nil {
		logger.Error("Error invoking evm function: can't unpack result", "err", err, "gasLeft", leftoverGas)
		return leftoverGas, err
	}

	logger.Trace("EVM call successful", "input", hexutil.Encode(input), "output", hexutil.Encode(output))
	return leftoverGas, nil
}
