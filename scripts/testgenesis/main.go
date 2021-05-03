package main

import (
	"encoding/json"
	"fmt"

	"os"

	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
)

func main() {
	genesisPath := "/Users/mcortesi/code/celo/envs/unittests/genesis.json"

	file, err := os.Open(genesisPath)
	exitOnError(err, "Failed to read genesis file")
	defer file.Close()

	genesis := new(core.Genesis)
	err = json.NewDecoder(file).Decode(genesis)
	exitOnError(err, "invalid genesis file")

	db := rawdb.NewMemoryDatabase()

	block, err := genesis.Commit(db)
	exitOnError(err, "can't setup genesis")

	statedb, err := state.New(block.Root(), state.NewDatabase(db), nil)
	exitOnError(err, "can't create statedb")

	_ = statedb
	// caller := vmcontext.NewSystemEVM()
}

func exitOnError(err error, msg string) {
	if err != nil {
		fmt.Printf("%s: %v\n", msg, err)
		os.Exit(1)
	}
}
