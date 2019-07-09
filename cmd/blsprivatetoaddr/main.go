// Copyright 2016 The go-ethereum Authors
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

package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	bls "github.com/celo-org/bls-zexe/go"
	"github.com/ethereum/go-ethereum/crypto"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
)

var (
	privateKeyFlag = flag.String("privatekey", "", "Private key")
)

func main() {
	// Parse and ensure all needed inputs are specified
	flag.Parse()

	if *privateKeyFlag == "" {
		fmt.Printf("Private key unspecified\n")
		os.Exit(-1)
	}
	privateKeyHex := *privateKeyFlag

	privateKeyECDSA, _ := crypto.HexToECDSA(privateKeyHex)
	privateKeyBytes, _ := blscrypto.ECDSAToBLS(privateKeyECDSA)

	privateKey, _ := bls.DeserializePrivateKey(privateKeyBytes)
	defer privateKey.Destroy()
	publicKey, _ := privateKey.ToPublic()
	defer publicKey.Destroy()
	publicKeyBytes, _ := publicKey.Serialize()

	address := blscrypto.IstanbulPubkeyToAddress(publicKeyBytes)
	fmt.Printf("%s", hex.EncodeToString(address[:]))
}
