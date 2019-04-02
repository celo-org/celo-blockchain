// Copyright 2014 The go-ethereum Authors
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

package common

import (
	"math/big"
)

func AddressToAbi(address Address) []byte {
	// Now convert address and amount to 32 byte (256-bit) chunks.
	return LeftPadBytes(address.Bytes(), 32)
}

func AmountToAbi(amount *big.Int) []byte {
	// Get amount as 32 bytes
	return LeftPadBytes(amount.Bytes(), 32)
}

// Generates ABI for a given method and its argument.
func GetEncodedAbiWithOneArg(methodSelector []byte, var1Abi []byte) []byte {
	encodedAbi := make([]byte, len(methodSelector)+len(var1Abi))
	copy(encodedAbi[0:len(methodSelector)], methodSelector[:])
	copy(encodedAbi[len(methodSelector):len(methodSelector)+len(var1Abi)], var1Abi[:])
	return encodedAbi
}

// Generates ABI for a given method and its arguments.
func GetEncodedAbiWithTwoArgs(methodSelector []byte, var1Abi []byte, var2Abi []byte) []byte {
	encodedAbi := make([]byte, len(methodSelector)+len(var1Abi)+len(var2Abi))
	copy(encodedAbi[0:len(methodSelector)], methodSelector[:])
	copy(encodedAbi[len(methodSelector):len(methodSelector)+len(var1Abi)], var1Abi[:])
	copy(encodedAbi[len(methodSelector)+len(var1Abi):], var2Abi[:])
	return encodedAbi
}
