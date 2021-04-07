// Copyright 2015 The go-ethereum Authors
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

package params

import "github.com/celo-org/celo-blockchain/common"

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Ethereum network.
var MainnetBootnodes = []string{"enode://5c9a3afb564b48cc2fa2e06b76d0c5d8f6910e1930ea7d0930213a0cbc20450434cd442f6483688eff436ad14dc29cb90c9592cc5c1d27ca62f28d4d8475d932@34.82.79.155:30301"}

// BaklavaBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Baklava test network.
var BaklavaBootnodes = []string{"enode://5aaf10664b12431c250597e980aacd7d5373cae00f128be5b00364344bb96bce7555b50973664bddebd1cb7a6d3fb927bec81527f80e22a26fa373c375fcdefc@35.247.103.141:30301"}

// AlfajoresBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Alfajores test network.
var AlfajoresBootnodes = []string{
	"enode://e99a883d0b7d0bacb84cde98c4729933b49adbc94e718b77fdb31779c7ed9da6c49236330a9ae096f42bcbf6e803394229120201704b7a4a3ae8004993fa0876@34.83.92.243:30303",
	"enode://b3b42a9a6ef1125006f39b95850c624422eadb6340ac86e4023e47d5452914bb3d17340f9455cd1cdd0d246058b1fec2d3c500eeddbeafa49abd71f8f144b04e@35.199.145.251:30303",
	"enode://af5677afe5bf99a00bdb86d0f80f948b2e25f8978867b38cba8e860a6426507cbc37e15900f798305ceff6b7ac7f4057195827274d6b5f6a7e547ce512ff26ba@35.230.21.97:30303",
	"enode://02d18a52c4e097c12412ac3da9a9af24745cff182306e21fb1d3d691fe0c25f65c60586302b933bb9ec300b7fb00ed719d26a4d57af91d447691bac2b2d778af@35.247.59.117:30303",
	"enode://05977f6b7d3e16a99d27b714f8a029a006e41ec7732167d373dd920d31f72b3a1776650798d8763560854369d36867e9564dad13b4b60a90c347feeb491d83a9@34.83.42.50:30303",
	"enode://822366c6b9f80c3f3fdf7748209399ddd888bd54882958f6c75d05b8156c6274f54c8a1a6b468c3e85cade93a7dee2a2b701ccfc820b7669507d4bee855ebbf1@35.247.105.237:30303",
	"enode://5bb26439782fb6f9d0d997f907968f4ada618d49d83a2e6d202a107d7b17c67c680877ee733e2f92656714697e6f5eb0da25f26180c3e13d5dc39dc037160651@34.83.234.156:30303",
	"enode://29373f661cbea23f4f566d54470fae6ef5419c2a88aa52f306628df2d143f86807c02fd19b3d2d6d2e2a98d99a2db44643c6274e3aadd3632bd744a8be498768@34.105.6.12:30303",
	"enode://cc11ee6b035c8948dfaca5b09d676e28b0986585dac7a819376f12a8ee8b6b0ffd31907bb0d8bda27ef0a2296ee614d31773c7c5ea4333a121d80e0ba9bae801@34.83.51.187:30303",
	"enode://703cf979becdc501c4221090296fe75299cb9520f19a344098154c14c7133ebf6b649dad7f3f42947ad96312930bea5380a8ff86faa5a3795b0b6cc483adcfc8@35.230.23.131:30303",
}

// KnownDNSNetwork returns the address of a public DNS-based node list for the given
// genesis hash and protocol. See https://github.com/ethereum/discv4-dns-lists for more
// information.
func KnownDNSNetwork(genesis common.Hash, protocol string) string {
	// For now, Celo doesn't use DNS discovery, so urls are blank
	return ""
}
