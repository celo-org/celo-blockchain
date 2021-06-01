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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// This extra data is not adding any meaningful data. It's only a valid extra data that will
// generate a valid output when it's being validated
var dummyExtraData = "0xecc833a7747eaa8327335e8e0c6b6d8aa3a38d0063591e43ce116ccf5c89753ef90262f869945296a071434eb2943c1bde8a2e1d555d911cd1d594bc236d14fbbe74b4d3bc132309b861dcf1c527c594ab3233a00d1e03b3e0c7c9912421b56f498901ef9421d87e9445d3b355807c91937e70e6f9914f2a1d94f6a964ad845a8a8a4a50b884c140ab8c3ed8252cf901eab86056631e24a6ec7fb89b5d967b6e7169eed00c0d802b9050d5a749b9d69a2c70d047b5add1d78646c4c3d579e7da7691001f0c832e9b26dd02b960b610ebfa63cd3a9dc0d6da8abfd8c16ffc5679debc682216c2934af7dc93d82e4a42564b4d80b86065be0e12d1ac62a4ddf326cb0dde804d8c1aab376dff994c9db706c6a5c0d5ba57319105491658f9ae5d5a4f48752b006137148123d8a526b2237d7284d5f0ab664f186ef0e7649d586a9754f54ca042539b3ba7ea25d0209f639cce30262081b860f1dade6f52a125a236a546a5bf5468ef8c95da980a51ee2ea595919e80bb56b3941bd17315b2681a411b6f7b6a2aaa01815a62fff57a14cae0cbef5a540dbd34f098ae18c07f93137eefc25132ac1971c8e74f2ddf24ceeeece87dcd18f19500b8601e4c01b96874cbc25fe98a4a8300035865d02724ec7b62d1662eba6b49777aaad51b73eb228d136d8d1f436e391e7e01d2c597cfcc17465a3aa1951d610360365bb116ab759887d5a74064c663aeeb3facf55ddc9a212e9f06b9925d27011700b860306ff63a1746b45b891a33d8ad7a02c92bf0ec628499f5308c55b15e7b5a2ab3d97113ef669c5ae446cb69b965c28d0056c780a4f2462b989bcb15380cc71fcf163d7e7be97c8c2a70b72e8273ff87f7d249f3c552fe7ecc28331f4ce90d92808080c3808080c3808080"

var customGenesisTests = []struct {
	genesis string
	query   string
	result  string
}{
	// Genesis file with an empty chain configuration (ensure missing fields work)
	{
		genesis: fmt.Sprintf(`{
			"alloc"      : {},
			"coinbase"   : "0x0000000000000000000000000000000000000000",
			"extraData"  : "",
			"parentHash" : "0x0000000000000000000000000000000000000000000000000000000000000000",
			"timestamp"  : "0xabcdef",
			"config"     : {
				"istanbul": {}
			},
			"extraData"  : "%s"
		}`, dummyExtraData),
		query:  "eth.getBlock(0).timestamp",
		result: "11259375",
	},
	// Genesis file with specific chain configurations
	{
		genesis: fmt.Sprintf(`{
			"alloc"      : {},
			"coinbase"   : "0x0000000000000000000000000000000000000000",
			"extraData"  : "",
			"parentHash" : "0x0000000000000000000000000000000000000000000000000000000000000000",
			"timestamp"  : "0xabcdf0",
			"config"     : {
				"homesteadBlock" : 42,
				"daoForkBlock"   : 141,
				"daoForkSupport" : true,
				"istanbul": {}
			},
			"extraData"  : "%s"
		}`, dummyExtraData),
		query:  "eth.getBlock(0).timestamp",
		result: "11259376",
	},
}

// Tests that initializing Geth with a custom genesis block and chain definitions
// work properly.
func TestCustomGenesis(t *testing.T) {
	for i, tt := range customGenesisTests {
		// Create a temporary data directory to use and inspect later
		datadir := tmpdir(t)
		defer os.RemoveAll(datadir)

		// Initialize the data directory with the custom genesis block
		json := filepath.Join(datadir, "genesis.json")
		if err := ioutil.WriteFile(json, []byte(tt.genesis), 0600); err != nil {
			t.Fatalf("test %d: failed to write genesis file: %v", i, err)
		}
		runGeth(t, "--datadir", datadir, "init", json).WaitExit()

		// Query the custom genesis block
		geth := runGeth(t,
			"--datadir", datadir, "--maxpeers", "0", "--port", "0", "--light.maxpeers", "0",
			"--nodiscover", "--nat", "none", "--ipcdisable",
			"--exec", tt.query, "console")
		geth.ExpectRegexp(tt.result)
		geth.ExpectExit()
	}
}
