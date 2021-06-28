// Copyright 2015 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

// blspopchecker checks BLS PoPs in genesis.
package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/celo-org/celo-bls-go/bls"
)

var (
	message = flag.String("message", "", "Common message to verify for the PoPs")
	genesis = flag.String("genesis", "", "JSON file containing the genesis PoPs")
)

func init() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage:", os.Args[0], "[-message <message>] [-genesis <path>]")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, `
Verifies PoP signatures for the genesis block.`)
	}
}

type genesisItem struct {
	Address      string `json:"address"`
	BLSPublicKey string `json:"blsPublicKey"`
	BLSPoP       string `json:"blsPop"`
}

type genesisJson = []genesisItem

func main() {
	flag.Parse()

	if *message == "" {
		panic(errors.New("message is not set"))
	}

	if *genesis == "" {
		panic(errors.New("genesis file is not set"))
	}

	data, err := ioutil.ReadFile(*genesis)
	check(err)

	run(*message, data)
}

func run(message string, data []byte) {
	var genesisData genesisJson
	err := json.Unmarshal(data, &genesisData)
	check(err)

	messageBytes := common.HexToAddress(message)

	for _, v := range genesisData {
		address, err := hex.DecodeString(v.Address)
		check(err)

		blsPublicKey, err := hex.DecodeString(v.BLSPublicKey)
		check(err)

		blsPop, err := hex.DecodeString(v.BLSPoP)
		check(err)

		publicKey, err := bls.DeserializePublicKey(blsPublicKey)
		check(err)

		signature, err := bls.DeserializeSignature(blsPop)
		check(err)

		err = publicKey.VerifyPoP(messageBytes.Bytes(), signature)
		check(err)

		fmt.Printf("PoP for signer %x is verified\n", address)
	}
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}
