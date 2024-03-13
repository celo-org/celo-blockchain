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

// getWarnings returns an error with all messages of type WARN of above, or nil if no warnings were present
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
	From                 common.MixedcaseAddress  `json:"from"`
	To                   *common.MixedcaseAddress `json:"to"`
	Gas                  hexutil.Uint64           `json:"gas"`
	GasPrice             *hexutil.Big             `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big             `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big             `json:"maxPriorityFeePerGas"`
	FeeCurrency          *common.MixedcaseAddress `json:"feeCurrency"`
	GatewayFeeRecipient  *common.MixedcaseAddress `json:"gatewayFeeRecipient"`
	GatewayFee           hexutil.Big              `json:"gatewayFee"`
	Value                hexutil.Big              `json:"value"`
	Nonce                hexutil.Uint64           `json:"nonce"`
	EthCompatible        bool                     `json:"ethCompatible"`
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

func (args SendTxArgs) CheckEthCompatibility() error {
	return args.ToTransaction().CheckEthCompatibility()
}

func (args *SendTxArgs) ToTransaction() *types.Transaction {
	// Upstream refactored this method to copy what txArgs.ToTransaction is doing in
	// bb1f7ebf203f40dae714a3b8445918cfcfc9a7db in order to be able to compile the code to
	// WebAssembly. Duplicating this logic right now does not seem to be worth it for celo
	// use case, so the same version as before is maintained.
	txArgs := ethapi.TransactionArgs{
		Gas:                  &args.Gas,
		GasPrice:             args.GasPrice,
		MaxFeePerGas:         args.MaxFeePerGas,
		MaxPriorityFeePerGas: args.MaxPriorityFeePerGas,
		GatewayFee:           &args.GatewayFee,
		Value:                &args.Value,
		Nonce:                &args.Nonce,
		Data:                 args.Data,
		Input:                args.Input,
		AccessList:           args.AccessList,
		ChainID:              args.ChainID,
	}
	if args.FeeCurrency != nil {
		feeCurrency := args.FeeCurrency.Address()
		txArgs.FeeCurrency = &feeCurrency
	}
	if args.GatewayFeeRecipient != nil {
		gatewayFeeRecipient := args.GatewayFeeRecipient.Address()
		txArgs.GatewayFeeRecipient = &gatewayFeeRecipient
	}
	// Add the To-field, if specified
	if args.To != nil {
		to := args.To.Address()
		txArgs.To = &to
	}
	return txArgs.ToTransaction()
}
