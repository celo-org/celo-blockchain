// Copyright 2019 The go-ethereum Authors
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

package core_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/celo-org/celo-blockchain/accounts/keystore"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/common/math"
	"github.com/celo-org/celo-blockchain/shared/signer"
	"github.com/celo-org/celo-blockchain/signer/core"
)

var typesStandard = signer.Types{
	"EIP712Domain": {
		{
			Name: "name",
			Type: "string",
		},
		{
			Name: "version",
			Type: "string",
		},
		{
			Name: "chainId",
			Type: "uint256",
		},
		{
			Name: "verifyingContract",
			Type: "address",
		},
	},
	"Person": {
		{
			Name: "name",
			Type: "string",
		},
		{
			Name: "wallet",
			Type: "address",
		},
	},
	"Mail": {
		{
			Name: "from",
			Type: "Person",
		},
		{
			Name: "to",
			Type: "Person",
		},
		{
			Name: "contents",
			Type: "string",
		},
	},
}

const primaryType = "Mail"

var domainStandard = signer.TypedDataDomain{
	Name:              "Ether Mail",
	Version:           "1",
	ChainId:           math.NewHexOrDecimal256(1),
	VerifyingContract: "0xCcCCccccCCCCcCCCCCCcCcCccCcCCCcCcccccccC",
	Salt:              "",
}

var messageStandard = map[string]interface{}{
	"from": map[string]interface{}{
		"name":   "Cow",
		"wallet": "0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826",
	},
	"to": map[string]interface{}{
		"name":   "Bob",
		"wallet": "0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB",
	},
	"contents": "Hello, Bob!",
}

var typedData = signer.TypedData{
	Types:       typesStandard,
	PrimaryType: primaryType,
	Domain:      domainStandard,
	Message:     messageStandard,
}

func TestSignData(t *testing.T) {
	api, control := setup(t)
	//Create two accounts
	createAccount(control, api, t)
	createAccount(control, api, t)
	control.approveCh <- "1"
	list, err := api.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a := common.NewMixedcaseAddress(list[0])

	control.approveCh <- "Y"
	control.inputCh <- "wrongpassword"
	signature, err := api.SignData(context.Background(), core.TextPlain.Mime, a, hexutil.Encode([]byte("EHLO world")))
	if signature != nil {
		t.Errorf("Expected nil-data, got %x", signature)
	}
	if err != keystore.ErrDecrypt {
		t.Errorf("Expected ErrLocked! '%v'", err)
	}
	control.approveCh <- "No way"
	signature, err = api.SignData(context.Background(), core.TextPlain.Mime, a, hexutil.Encode([]byte("EHLO world")))
	if signature != nil {
		t.Errorf("Expected nil-data, got %x", signature)
	}
	if err != core.ErrRequestDenied {
		t.Errorf("Expected ErrRequestDenied! '%v'", err)
	}
	// text/plain
	control.approveCh <- "Y"
	control.inputCh <- "a_long_password"
	signature, err = api.SignData(context.Background(), core.TextPlain.Mime, a, hexutil.Encode([]byte("EHLO world")))
	if err != nil {
		t.Fatal(err)
	}
	if signature == nil || len(signature) != 65 {
		t.Errorf("Expected 65 byte signature (got %d bytes)", len(signature))
	}
	// data/typed
	control.approveCh <- "Y"
	control.inputCh <- "a_long_password"
	signature, err = api.SignTypedData(context.Background(), a, typedData)
	if err != nil {
		t.Fatal(err)
	}
	if signature == nil || len(signature) != 65 {
		t.Errorf("Expected 65 byte signature (got %d bytes)", len(signature))
	}
}

var gnosisTypedData = `
{
	"types": {
		"EIP712Domain": [
			{ "type": "address", "name": "verifyingContract" }
		],
		"SafeTx": [
			{ "type": "address", "name": "to" },
			{ "type": "uint256", "name": "value" },
			{ "type": "bytes", "name": "data" },
			{ "type": "uint8", "name": "operation" },
			{ "type": "uint256", "name": "safeTxGas" },
			{ "type": "uint256", "name": "baseGas" },
			{ "type": "uint256", "name": "gasPrice" },
			{ "type": "address", "name": "gasToken" },
			{ "type": "address", "name": "refundReceiver" },
			{ "type": "uint256", "name": "nonce" }
		]
	},
	"domain": {
		"verifyingContract": "0x25a6c4BBd32B2424A9c99aEB0584Ad12045382B3"
	},
	"primaryType": "SafeTx",
	"message": {
		"to": "0x9eE457023bB3De16D51A003a247BaEaD7fce313D",
		"value": "20000000000000000",
		"data": "0x",
		"operation": 0,
		"safeTxGas": 27845,
		"baseGas": 0,
		"gasPrice": "0",
		"gasToken": "0x0000000000000000000000000000000000000000",
		"refundReceiver": "0x0000000000000000000000000000000000000000",
		"nonce": 3
	}
}`

var gnosisTx = `
{
      "safe": "0x25a6c4BBd32B2424A9c99aEB0584Ad12045382B3",
      "to": "0x9eE457023bB3De16D51A003a247BaEaD7fce313D",
      "value": "20000000000000000",
      "data": null,
      "operation": 0,
      "gasToken": "0x0000000000000000000000000000000000000000",
      "safeTxGas": 27845,
      "baseGas": 0,
      "gasPrice": "0",
      "refundReceiver": "0x0000000000000000000000000000000000000000",
      "nonce": 3,
      "executionDate": null,
      "submissionDate": "2020-09-15T21:59:23.815748Z",
      "modified": "2020-09-15T21:59:23.815748Z",
      "blockNumber": null,
      "transactionHash": null,
      "safeTxHash": "0x28bae2bd58d894a1d9b69e5e9fde3570c4b98a6fc5499aefb54fb830137e831f",
      "executor": null,
      "isExecuted": false,
      "isSuccessful": null,
      "ethGasPrice": null,
      "gasUsed": null,
      "fee": null,
      "origin": null,
      "dataDecoded": null,
      "confirmationsRequired": null,
      "confirmations": [
        {
          "owner": "0xAd2e180019FCa9e55CADe76E4487F126Fd08DA34",
          "submissionDate": "2020-09-15T21:59:28.281243Z",
          "transactionHash": null,
          "confirmationType": "CONFIRMATION",
          "signature": "0x5e562065a0cb15d766dac0cd49eb6d196a41183af302c4ecad45f1a81958d7797753f04424a9b0aa1cb0448e4ec8e189540fbcdda7530ef9b9d95dfc2d36cb521b",
          "signatureType": "EOA"
        }
      ],
      "signatures": null
    }
`

// TestGnosisTypedData tests the scenario where a user submits a full EIP-712
// struct without using the gnosis-specific endpoint
func TestGnosisTypedData(t *testing.T) {
	var td signer.TypedData
	err := json.Unmarshal([]byte(gnosisTypedData), &td)
	if err != nil {
		t.Fatalf("unmarshalling failed '%v'", err)
	}
	_, sighash, err := sign(td)
	if err != nil {
		t.Fatal(err)
	}
	expSigHash := common.FromHex("0x28bae2bd58d894a1d9b69e5e9fde3570c4b98a6fc5499aefb54fb830137e831f")
	if !bytes.Equal(expSigHash, sighash) {
		t.Fatalf("Error, got %x, wanted %x", sighash, expSigHash)
	}
}

// TestGnosisCustomData tests the scenario where a user submits only the gnosis-safe
// specific data, and we fill the TypedData struct on our side
func TestGnosisCustomData(t *testing.T) {
	var tx core.GnosisSafeTx
	err := json.Unmarshal([]byte(gnosisTx), &tx)
	if err != nil {
		t.Fatal(err)
	}
	var td = tx.ToTypedData()
	_, sighash, err := sign(td)
	if err != nil {
		t.Fatal(err)
	}
	expSigHash := common.FromHex("0x28bae2bd58d894a1d9b69e5e9fde3570c4b98a6fc5499aefb54fb830137e831f")
	if !bytes.Equal(expSigHash, sighash) {
		t.Fatalf("Error, got %x, wanted %x", sighash, expSigHash)
	}
}
