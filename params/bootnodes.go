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

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Ethereum network.
var MainnetBootnodes = []string{"enode://5c9a3afb564b48cc2fa2e06b76d0c5d8f6910e1930ea7d0930213a0cbc20450434cd442f6483688eff436ad14dc29cb90c9592cc5c1d27ca62f28d4d8475d932@34.82.79.155:30301", "enode://f65013f1ac6827e275c2d2737ce13357f620d4364124d02227a19321c57f8fbf9214a9411de49d49f180b085b031d9d23211a6ead4499fc5f9d3592b55322123@50.17.60.161:30303"}

// BaklavaBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Baklava test network.
var BaklavaBootnodes = []string{"enode://5aaf10664b12431c250597e980aacd7d5373cae00f128be5b00364344bb96bce7555b50973664bddebd1cb7a6d3fb927bec81527f80e22a26fa373c375fcdefc@35.247.75.229:30301", "enode://7c34cb0b3b9924b21e3c99134a5d46ca8242f4ecc9f722aa548ff80861be246268f1f7d8de815583a970993a7157bb37b3b5478b219c44e05888a659cb22675f@34.193.36.54:30303"}

// AlfajoresBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Alfajores test network.
var AlfajoresBootnodes = []string{"enode://e99a883d0b7d0bacb84cde98c4729933b49adbc94e718b77fdb31779c7ed9da6c49236330a9ae096f42bcbf6e803394229120201704b7a4a3ae8004993fa0876@35.247.115.108:30303"}

// DiscoveryV5Bootnodes are the enode URLs of the P2P bootstrap nodes for the
// experimental RLPx v5 topic-discovery network.
var DiscoveryV5Bootnodes = []string{}
