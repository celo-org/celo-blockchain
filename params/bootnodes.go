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
var MainnetBootnodes = []string{
	"enode://c53aec8e9b2a1b833d002e52d8b59c9db3a30f1d9f15e2838db6b66969c0f8a4d5373bf49753e8cae948c965e8b65c094cebac256b34f7e767ecb9010daa3062@34.80.31.1:30303",
	"enode://c28fe08a1969bcbaaefdd5425f1d558cab4da45122cf9ecef6033c4004945d43c09f78cb6c82fa1bcfc4d22952562252e8513fd39eb043390d64e0976eb6cbfd@35.195.185.50:30303",
	"enode://76cab6967b82eafbdc223d2b5177c25c5376e054bb8b5aff8a3e3adae53939748e988b7928a603768eeefa55d198963db4052c79b5f60457a82b3546af8dfb86@35.199.109.134:30303",
	"enode://661222d08cc1038677e05c1ade64981a29d95b2ad4cbd5c21cdbe7ead039bf8ed53012b691b7c5158a039cd96bc48509bf8f33c62a85d689d390f267953f6ece@34.75.123.188:30303",
	"enode://6d1702b599db61bbdf23a6201b3d1595b526e536194c5140904e7b34d79162b9905a75a257e9a64bd2ee1ed3ef40e404b14e4f1eb985cc180d9912497fd81b0d@34.105.94.71:30303",
	"enode://a7b13cc3f1c0c1629086322b085f53a08252a06b28d4562cb854619fa81cfa95b1cee4fea3179bbf76524bbb9f0d8fe777fe0aa0fa1807a1de569ecbe177a78b@35.229.224.92:30303",
	"enode://e4a2a85656c8c6e40650a52874a29f96fa8546a6d2ce3a87720b4f2e0138b138419e440935462eea070ef8647dba9c2d417231790f3ece5b53117c9c9c5cbb3f@34.76.108.87:30303",
	"enode://44c315b059130c22e8aca61e8cb743d5192840bdd107fdf176448b39d4a92b3486fefe0caeca709c4aea7d6a82dc2b3475173b79ec457929a41f2a50ddc7655f@34.95.203.238:30303",
	"enode://5bd878715e78773cb059800052b6d2454ef14f6b02f07b267fb6e46207a05b98354385e09297de19f4a42fc34c7148ebc545e7fcb29c5cb3fc54526384e7a2b2@35.237.159.228:30303",
	"enode://306053e90aa90d7ef2dc3fd3fcbd14c4014c70040cdc767e7ca807e4981d596aa67ba5f798ac7fefd10ff8eb7d00aa190a703e5fa07b7b762f28068f56234c68@35.197.123.152:30303",
}

// BaklavaBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Baklava test network.
var BaklavaBootnodes = []string{
	"enode://be3a43b8a04a02cdee034ec2d133c6bd331429574cad91b739200ac88872dbd6acc81c20d1f891d598da77e47bb01e405464458d214052c1043f9b3d50d94f5c@34.105.120.49:30303",
	"enode://1044681acf8d7c120bc53c1a9e84adde71ac7155789525580ae786448111a877a96b8bd9b4a39ca6a891a3c7beb90080f7ebe5eae4beef75c0d4e8725a2b16e6@35.203.142.37:30303",
	"enode://c34cdb9b4a8b7c54c007ff27395b98de934653549b4ff73cdc9b913f558f2403ad837e6c0e202fe56b19f2fe914fa863eee86dd12ddd3d200f185d0f899d36b4@34.105.26.151:30303",
}

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
