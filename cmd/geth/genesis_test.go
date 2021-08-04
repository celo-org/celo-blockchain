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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

var customGenesisTests = []struct {
	genesis string
	query   string
	result  string
}{
	// Genesis file with an empty chain configuration (ensure missing fields work)
	{
		genesis: `{
			"alloc"      : {},
			"coinbase"   : "0x0000000000000000000000000000000000000000",
			"extraData"  : "",
			"parentHash" : "0x0000000000000000000000000000000000000000000000000000000000000000",
			"timestamp"  : "0xabcdef",
			"config"     : {
				"istanbul": {}
			}
		}`,
		query:  "eth.getBlock(0).timestamp",
		result: "11259375",
	},
	// Genesis file with specific chain configurations
	{
		genesis: `{
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
			}
		}`,
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
		geth := runGeth(t, "--nousb", "--networkid", "1337", "--syncmode=full",
			"--datadir", datadir, "--maxpeers", "0", "--port", "0", "--light.maxpeers", "0",
			"--nodiscover", "--nat", "none", "--ipcdisable",
			"--exec", tt.query, "console")
		geth.ExpectRegexp(tt.result)
		geth.ExpectExit()
	}
}
