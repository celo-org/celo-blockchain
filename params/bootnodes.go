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
	// Bootnode
	"enode://5c9a3afb564b48cc2fa2e06b76d0c5d8f6910e1930ea7d0930213a0cbc20450434cd442f6483688eff436ad14dc29cb90c9592cc5c1d27ca62f28d4d8475d932@34.82.79.155:30301",
	// Forno us-west1 cluster
	"enode://6d1702b599db61bbdf23a6201b3d1595b526e536194c5140904e7b34d79162b9905a75a257e9a64bd2ee1ed3ef40e404b14e4f1eb985cc180d9912497fd81b0d@34.105.94.71:30303",
	"enode://306053e90aa90d7ef2dc3fd3fcbd14c4014c70040cdc767e7ca807e4981d596aa67ba5f798ac7fefd10ff8eb7d00aa190a703e5fa07b7b762f28068f56234c68@35.197.123.152:30303",
	"enode://e5d0a8c8d944d54d58b58e2214df15865b1f8c5363dbe871a95698494e6b04a0b230b31ee017f34dbe54a1648cb1a784c8aa9f8de3f3f3e8b96f2b4c2149e908@35.247.42.51:30303",
	"enode://c4c4cb1bd636fa25cd456462abd4d287c6ce94b91dfa1518fc40b61d7da868f7fe78d5ad30913609711b68ab06b9f5bc3a42c9dc220ecade03cbf18a50172090@34.83.140.127:30303",
	"enode://eec61e6e63e69ac4c612d06ec2f4169363a0945568a8555954e39c89a98a93ef04b3fe7925fbf832a329b6f2b4fe3c8066cca15edd3967d672f7cb9676a89dbc@35.230.33.130:30303",
	"enode://f11356a0802466235b470da8588b04e8831d0d4dccbe9c943c70b1b641f7f916c4d2534bffba1363489dfff8140bbee8dc16abe33b63abd3181b80976153f8c2@35.230.108.150:30303",
	// Forno europe-west1 cluster
	"enode://c28fe08a1969bcbaaefdd5425f1d558cab4da45122cf9ecef6033c4004945d43c09f78cb6c82fa1bcfc4d22952562252e8513fd39eb043390d64e0976eb6cbfd@35.195.185.50:30303",
	"enode://e4a2a85656c8c6e40650a52874a29f96fa8546a6d2ce3a87720b4f2e0138b138419e440935462eea070ef8647dba9c2d417231790f3ece5b53117c9c9c5cbb3f@34.76.108.87:30303",
	"enode://00975d8863bd00bfd3a10457c1daf84ce991337f4dd266564d05efb21b54968f3db8125302a3e15d2047f795aa428051894dfc3707096429e7ad6401a0c347ee@34.76.60.59:30303",
	"enode://3413ca8a97908e0260b2c2e1c0b0c50e1d75b3a944195e1e403bb6978a2854841f937fb305cce76cdee59a970ff907bbfcc1642242f0db66e8b115e4b8a1c8cd@35.205.108.193:30303",
}

// BaklavaBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Baklava test network.
var BaklavaBootnodes = []string{
	"enode://5aaf10664b12431c250597e980aacd7d5373cae00f128be5b00364344bb96bce7555b50973664bddebd1cb7a6d3fb927bec81527f80e22a26fa373c375fcdefc@35.247.103.141:30301",
	"enode://be3a43b8a04a02cdee034ec2d133c6bd331429574cad91b739200ac88872dbd6acc81c20d1f891d598da77e47bb01e405464458d214052c1043f9b3d50d94f5c@34.105.120.49:30303",
	"enode://1044681acf8d7c120bc53c1a9e84adde71ac7155789525580ae786448111a877a96b8bd9b4a39ca6a891a3c7beb90080f7ebe5eae4beef75c0d4e8725a2b16e6@35.203.142.37:30303",
	"enode://c34cdb9b4a8b7c54c007ff27395b98de934653549b4ff73cdc9b913f558f2403ad837e6c0e202fe56b19f2fe914fa863eee86dd12ddd3d200f185d0f899d36b4@34.105.26.151:30303",
	"enode://b09158aefac3370e0b75b1ef01af5c0cf54914e517e4ad6c7893346d41aafa4441fe3bf9ef66e665214d673a3dc4c3b6ff62d546db417de63d4dcec6b53581d2@35.247.40.78:30303",
	"enode://269ef649fa4a8cd50661a150559696c931d51267a20010b74fa916610ae09f8f2d606263017f203528c4ad748e8808df5f1c97726b0df4860c199af3369bd7e1@35.233.157.112:30303",
	"enode://6e5c01a689fbe51cf0f5313426a3bd48189f3cdfe068018ff170838518c50e8a9acd4ef49828afe0e50372b4e477658b6677be5978f80df0085d3bed38ddab15@35.247.123.65:30303",
	"enode://29fa29d0e985ffd1ae5c5569a5140bdcc8380d9ac847d319fb824c02ef990440506dff6c005a17b737c4f4b31fc4286c423c868c3dedbd904bc525fa6a64815f@35.197.32.163:30303",
}

// AlfajoresBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Alfajores test network.
var AlfajoresBootnodes = []string{
	"enode://322f16487eb79832b66fe9091d8cc51cad861068d3e52f06ef391ecb89fe9342cfb8bd58020ece66ad8e081e9f08cb621201e4ad160092d7ff158cc03865d114@34.83.107.15:30301",
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
