// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package apitypes

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/internal/ethapi"
)

type ValidationInfo struct {
	Typ     string `json:"type"`
	Message string `json:"message"`
}
type ValidationMessages struct {
	Messages []ValidationInfo
}

const (
	WARN = "WARNING"
	CRIT = "CRITICAL"
	INFO = "Info"
)

func (vs *ValidationMessages) Crit(msg string) {
	vs.Messages = append(vs.Messages, ValidationInfo{CRIT, msg})
}
func (vs *ValidationMessages) Warn(msg string) {
	vs.Messages = append(vs.Messages, ValidationInfo{WARN, msg})
}
func (vs *ValidationMessages) Info(msg string) {
	vs.Messages = append(vs.Messages, ValidationInfo{INFO, msg})
}

/// getWarnings returns an error with all messages of type WARN of above, or nil if no warnings were present
func (v *ValidationMessages) GetWarnings() error {
	var messages []string
	for _, msg := range v.Messages {
		if msg.Typ == WARN || msg.Typ == CRIT {
			messages = append(messages, msg.Message)
		}
	}
	if len(messages) > 0 {
		return fmt.Errorf("validation failed: %s", strings.Join(messages, ","))
	}
	return nil
}

// SendTxArgs represents the arguments to submit a transaction
// This struct is identical to ethapi.TransactionArgs, except for the usage of
// common.MixedcaseAddress in From and To
type SendTxArgs struct {
<<<<<<< HEAD:signer/core/types.go
	From                common.MixedcaseAddress  `json:"from"`
	To                  *common.MixedcaseAddress `json:"to"`
	Gas                 hexutil.Uint64           `json:"gas"`
	GasPrice            hexutil.Big              `json:"gasPrice"`
	FeeCurrency         *common.MixedcaseAddress `json:"feeCurrency"`
	GatewayFeeRecipient *common.MixedcaseAddress `json:"gatewayFeeRecipient"`
	GatewayFee          hexutil.Big              `json:"gatewayFee"`
	Value               hexutil.Big              `json:"value"`
	Nonce               hexutil.Uint64           `json:"nonce"`
	EthCompatible       bool                     `json:"ethCompatible"`
||||||| e78727290:signer/core/types.go
	From     common.MixedcaseAddress  `json:"from"`
	To       *common.MixedcaseAddress `json:"to"`
	Gas      hexutil.Uint64           `json:"gas"`
	GasPrice hexutil.Big              `json:"gasPrice"`
	Value    hexutil.Big              `json:"value"`
	Nonce    hexutil.Uint64           `json:"nonce"`
=======
	From                 common.MixedcaseAddress  `json:"from"`
	To                   *common.MixedcaseAddress `json:"to"`
	Gas                  hexutil.Uint64           `json:"gas"`
	GasPrice             *hexutil.Big             `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big             `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big             `json:"maxPriorityFeePerGas"`
	Value                hexutil.Big              `json:"value"`
	Nonce                hexutil.Uint64           `json:"nonce"`

>>>>>>> v1.10.7:signer/core/apitypes/types.go
	// We accept "data" and "input" for backwards-compatibility reasons.
	// "input" is the newer name and should be preferred by clients.
	// Issue detail: https://github.com/ethereum/go-ethereum/issues/15628
	Data  *hexutil.Bytes `json:"data"`
	Input *hexutil.Bytes `json:"input,omitempty"`

	// For non-legacy transactions
	AccessList *types.AccessList `json:"accessList,omitempty"`
	ChainID    *hexutil.Big      `json:"chainId,omitempty"`
}

func (args SendTxArgs) String() string {
	s, err := json.Marshal(args)
	if err == nil {
		return string(s)
	}
	return err.Error()
}

<<<<<<< HEAD:signer/core/types.go
func (args SendTxArgs) CheckEthCompatibility() error {
	return args.toTransaction().CheckEthCompatibility()
}

func (args *SendTxArgs) toTransaction() *types.Transaction {
	var input []byte
	if args.Data != nil {
		input = *args.Data
	} else if args.Input != nil {
		input = *args.Input
||||||| e78727290:signer/core/types.go
func (args *SendTxArgs) toTransaction() *types.Transaction {
	var input []byte
	if args.Data != nil {
		input = *args.Data
	} else if args.Input != nil {
		input = *args.Input
=======
func (args *SendTxArgs) ToTransaction() *types.Transaction {
	txArgs := ethapi.TransactionArgs{
		Gas:                  &args.Gas,
		GasPrice:             args.GasPrice,
		MaxFeePerGas:         args.MaxFeePerGas,
		MaxPriorityFeePerGas: args.MaxPriorityFeePerGas,
		Value:                &args.Value,
		Nonce:                &args.Nonce,
		Data:                 args.Data,
		Input:                args.Input,
		AccessList:           args.AccessList,
		ChainID:              args.ChainID,
>>>>>>> v1.10.7:signer/core/apitypes/types.go
	}
<<<<<<< HEAD:signer/core/types.go
	var feeCurrency *common.Address = nil
	if args.FeeCurrency != nil {
		tmp := args.FeeCurrency.Address()
		feeCurrency = &tmp
	}
	var gatewayFeeRecipient *common.Address = nil
	if args.GatewayFeeRecipient != nil {
		tmp := args.GatewayFeeRecipient.Address()
		gatewayFeeRecipient = &tmp
	}
	if args.To == nil {
		if args.EthCompatible {
			return types.NewContractCreationEthCompatible(uint64(args.Nonce), (*big.Int)(&args.Value), uint64(args.Gas), (*big.Int)(&args.GasPrice), input)
		}
		return types.NewContractCreation(uint64(args.Nonce), (*big.Int)(&args.Value), uint64(args.Gas), (*big.Int)(&args.GasPrice), feeCurrency, gatewayFeeRecipient, (*big.Int)(&args.GatewayFee), input)
||||||| e78727290:signer/core/types.go
	if args.To == nil {
		return types.NewContractCreation(uint64(args.Nonce), (*big.Int)(&args.Value), uint64(args.Gas), (*big.Int)(&args.GasPrice), input)
=======
	// Add the To-field, if specified
	if args.To != nil {
		to := args.To.Address()
		txArgs.To = &to
>>>>>>> v1.10.7:signer/core/apitypes/types.go
	}
<<<<<<< HEAD:signer/core/types.go
	if args.EthCompatible {
		return types.NewTransactionEthCompatible(uint64(args.Nonce), args.To.Address(), (*big.Int)(&args.Value), (uint64)(args.Gas), (*big.Int)(&args.GasPrice), input)
	}
	return types.NewTransaction(uint64(args.Nonce), args.To.Address(), (*big.Int)(&args.Value), (uint64)(args.Gas), (*big.Int)(&args.GasPrice), feeCurrency, gatewayFeeRecipient, (*big.Int)(&args.GatewayFee), input)
||||||| e78727290:signer/core/types.go
	return types.NewTransaction(uint64(args.Nonce), args.To.Address(), (*big.Int)(&args.Value), (uint64)(args.Gas), (*big.Int)(&args.GasPrice), input)
=======
	return txArgs.ToTransaction()
>>>>>>> v1.10.7:signer/core/apitypes/types.go
}
