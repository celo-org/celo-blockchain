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

package core

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

//go:generate gencodec -type Genesis -field-override genesisSpecMarshaling -out gen_genesis.go
//go:generate gencodec -type GenesisAccount -field-override genesisAccountMarshaling -out gen_genesis_account.go

var (
	DBGenesisSupplyKey = []byte("genesis-supply-genesis")
	errGenesisNoConfig = errors.New("genesis has no chain configuration")
)

// Genesis specifies the header fields, state of a genesis block. It also defines hard
// fork switch-over blocks through the chain configuration.
type Genesis struct {
	Config    *params.ChainConfig `json:"config"`
	Timestamp uint64              `json:"timestamp"`
	ExtraData []byte              `json:"extraData"`
	Coinbase  common.Address      `json:"coinbase"`
	Alloc     GenesisAlloc        `json:"alloc"      gencodec:"required"`

	// These fields are used for consensus tests. Please don't use them
	// in actual genesis blocks.
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
}

// GenesisAlloc specifies the initial state that is part of the genesis block.
type GenesisAlloc map[common.Address]GenesisAccount

func (ga *GenesisAlloc) UnmarshalJSON(data []byte) error {
	m := make(map[common.UnprefixedAddress]GenesisAccount)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*ga = make(GenesisAlloc)
	for addr, a := range m {
		(*ga)[common.Address(addr)] = a
	}
	return nil
}

// GenesisAccount is an account in the state of the genesis block.
type GenesisAccount struct {
	Code       []byte                      `json:"code,omitempty"`
	Storage    map[common.Hash]common.Hash `json:"storage,omitempty"`
	Balance    *big.Int                    `json:"balance" gencodec:"required"`
	Nonce      uint64                      `json:"nonce,omitempty"`
	PrivateKey []byte                      `json:"secretKey,omitempty"` // for tests
}

// field type overrides for gencodec
type genesisSpecMarshaling struct {
	Timestamp math.HexOrDecimal64
	ExtraData hexutil.Bytes
	GasUsed   math.HexOrDecimal64
	Number    math.HexOrDecimal64
	Alloc     map[common.UnprefixedAddress]GenesisAccount
}

type genesisAccountMarshaling struct {
	Code       hexutil.Bytes
	Balance    *math.HexOrDecimal256
	Nonce      math.HexOrDecimal64
	Storage    map[storageJSON]storageJSON
	PrivateKey hexutil.Bytes
}

// storageJSON represents a 256 bit byte array, but allows less than 256 bits when
// unmarshaling from hex.
type storageJSON common.Hash

func (h *storageJSON) UnmarshalText(text []byte) error {
	text = bytes.TrimPrefix(text, []byte("0x"))
	if len(text) > 64 {
		return fmt.Errorf("too many hex characters in storage key/value %q", text)
	}
	offset := len(h) - len(text)/2 // pad on the left
	if _, err := hex.Decode(h[offset:], text); err != nil {
		fmt.Println(err)
		return fmt.Errorf("invalid hex storage key/value %q", text)
	}
	return nil
}

func (h storageJSON) MarshalText() ([]byte, error) {
	return hexutil.Bytes(h[:]).MarshalText()
}

// GenesisMismatchError is raised when trying to overwrite an existing
// genesis block with an incompatible one.
type GenesisMismatchError struct {
	Stored, New common.Hash
}

func (e *GenesisMismatchError) Error() string {
	return fmt.Sprintf("database contains incompatible genesis (have %x, new %x)", e.Stored, e.New)
}

// SetupGenesisBlock writes or updates the genesis block in db.
// The block that will be used is:
//
//                          genesis == nil       genesis != nil
//                       +------------------------------------------
//     db has no genesis |  main-net default  |  genesis
//     db has genesis    |  from DB           |  genesis (if compatible)
//
// The stored chain configuration will be updated if it is compatible (i.e. does not
// specify a fork block below the local head block). In case of a conflict, the
// error is a *params.ConfigCompatError and the new, unwritten config is returned.
//
// The returned chain configuration is never nil.
func SetupGenesisBlock(db ethdb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	return SetupGenesisBlockWithOverride(db, genesis, nil)
}

func SetupGenesisBlockWithOverride(db ethdb.Database, genesis *Genesis, overrideIstanbul *big.Int) (*params.ChainConfig, common.Hash, error) {
	if genesis != nil && (genesis.Config == nil || genesis.Config.Istanbul == nil) {
		return params.DefaultChainConfig, common.Hash{}, errGenesisNoConfig
	}
	if genesis != nil && genesis.Config != nil && genesis.Config.UseOldFormat {
		types.SetOldFormat()
	}
	// Just commit the new block if there is no stored genesis block.
	stored := rawdb.ReadCanonicalHash(db, 0)
	if (stored == common.Hash{}) {
		if genesis == nil {
			log.Info("Writing default main-net genesis block")
			genesis = DefaultGenesisBlock()
		} else {
			log.Info("Writing custom genesis block")
		}
		block, err := genesis.Commit(db)
		log.Info("HASH2", "hash", block.Hash())
		if err != nil {
			return genesis.Config, common.Hash{}, err
		}
		return genesis.Config, block.Hash(), nil
	}

	// We have the genesis block in database(perhaps in ancient database)
	// but the corresponding state is missing.
	header := rawdb.ReadHeader(db, stored, 0)
	if _, err := state.New(header.Root, state.NewDatabaseWithCache(db, 0)); err != nil {
		if genesis == nil {
			genesis = DefaultGenesisBlock()
		}
		// Ensure the stored genesis matches with the given one.
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
		block, err := genesis.Commit(db)
		if err != nil {
			return genesis.Config, hash, err
		}
		return genesis.Config, block.Hash(), nil
	}

	// Check whether the genesis block is already written.
	if genesis != nil {
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
	}

	// Get the existing chain configuration.
	newcfg := genesis.configOrDefault(stored)
	if overrideIstanbul != nil {
		newcfg.IstanbulBlock = overrideIstanbul
	}
	if err := newcfg.CheckConfigForkOrder(); err != nil {
		return newcfg, common.Hash{}, err
	}
	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)
		return newcfg, stored, nil
	}
	// Special case: don't change the existing config of a non-mainnet chain if no new
	// config is supplied. These chains would get AllProtocolChanges (and a compat error)
	// if we just continued here.
	if genesis == nil && stored != params.MainnetGenesisHash {
		return storedcfg, stored, nil
	}

	// Check config compatibility and write the config. Compatibility errors
	// are returned to the caller unless we're already at block zero.
	height := rawdb.ReadHeaderNumber(db, rawdb.ReadHeadHeaderHash(db))
	if height == nil {
		return newcfg, stored, fmt.Errorf("missing block number for head header hash")
	}
	compatErr := storedcfg.CheckCompatible(newcfg, *height)
	if compatErr != nil && *height != 0 && compatErr.RewindTo != 0 {
		return newcfg, stored, compatErr
	}
	rawdb.WriteChainConfig(db, stored, newcfg)
	return newcfg, stored, nil
}

func (g *Genesis) configOrDefault(ghash common.Hash) *params.ChainConfig {
	switch {
	case g != nil:
		return g.Config
	case ghash == params.MainnetGenesisHash:
		return params.MainnetChainConfig
	case ghash == params.TestnetGenesisHash:
		return params.TestnetChainConfig
	default:
		return params.DefaultChainConfig
	}
}

// ToBlock creates the genesis block and writes state of a genesis specification
// to the given database (or discards it if nil).
func (g *Genesis) ToBlock(db ethdb.Database) *types.Block {
	if db == nil {
		db = rawdb.NewMemoryDatabase()
	}
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
	for addr, account := range g.Alloc {
		statedb.AddBalance(addr, account.Balance)
		statedb.SetCode(addr, account.Code)
		statedb.SetNonce(addr, account.Nonce)
		for key, value := range account.Storage {
			statedb.SetState(addr, key, value)
		}
	}
	root := statedb.IntermediateRoot(false)
	head := &types.Header{
		Number:     new(big.Int).SetUint64(g.Number),
		Time:       g.Timestamp,
		ParentHash: g.ParentHash,
		Extra:      g.ExtraData,
		GasUsed:    g.GasUsed,
		Coinbase:   g.Coinbase,
		Root:       root,
	}
	statedb.Commit(false)
	statedb.Database().TrieDB().Commit(root, true)

	return types.NewBlock(head, nil, nil, nil)
}

// StoreGenesisSupply computes the total supply of the genesis block and stores
// it in the db.
func (g *Genesis) StoreGenesisSupply(db ethdb.Database) error {
	if db == nil {
		db = rawdb.NewMemoryDatabase()
	}
	genesisSupply := big.NewInt(0)
	for _, account := range g.Alloc {
		genesisSupply.Add(genesisSupply, account.Balance)
	}
	return db.Put(DBGenesisSupplyKey, genesisSupply.Bytes())
}

// Commit writes the block and state of a genesis specification to the database.
// The block is committed as the canonical head block.
func (g *Genesis) Commit(db ethdb.Database) (*types.Block, error) {
	block := g.ToBlock(db)
	if block.Number().Sign() != 0 {
		return nil, fmt.Errorf("can't commit genesis block with number > 0")
	}
	config := g.Config
	if config == nil {
		config = params.DefaultChainConfig
	}
	if err := config.CheckConfigForkOrder(); err != nil {
		return nil, err
	}
	rawdb.WriteTd(db, block.Hash(), block.NumberU64(), block.TotalDifficulty())
	rawdb.WriteBlock(db, block)
	rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), nil)
	rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
	rawdb.WriteHeadBlockHash(db, block.Hash())
	rawdb.WriteHeadFastBlockHash(db, block.Hash())
	rawdb.WriteHeadHeaderHash(db, block.Hash())
	if err := g.StoreGenesisSupply(db); err != nil {
		log.Error("Unable to store genesisSupply in db", "err", err)
		return nil, err
	}

	rawdb.WriteChainConfig(db, block.Hash(), config)
	return block, nil
}

// MustCommit writes the genesis block and state to db, panicking on error.
// The block is committed as the canonical head block.
func (g *Genesis) MustCommit(db ethdb.Database) *types.Block {
	block, err := g.Commit(db)
	if err != nil {
		panic(err)
	}
	return block
}

// GenesisBlockForTesting creates and writes a block in which addr has the given wei balance.
func GenesisBlockForTesting(db ethdb.Database, addr common.Address, balance *big.Int) *types.Block {
	g := Genesis{Config: params.TestnetChainConfig, Alloc: GenesisAlloc{addr: {Balance: balance}}}
	return g.MustCommit(db)
}

// DefaultGenesisBlock returns the Ethereum main net genesis block.
func DefaultGenesisBlock() *Genesis {
	return &Genesis{
		Config:    params.MainnetChainConfig,
		ExtraData: hexutil.MustDecode("0x11bbe8db4e347b4e8c937c1c8370e4b5ed33adb3db69cbdb7a38e1e50b1b82fa"),
		Alloc:     decodePrealloc(mainnetAllocData),
	}
}

// DefaultTestnetGenesisBlock returns the Ropsten network genesis block.
func DefaultTestnetGenesisBlock() *Genesis {
	return &Genesis{
		Config:    params.TestnetChainConfig,
		ExtraData: hexutil.MustDecode("0x3535353535353535353535353535353535353535353535353535353535353535"),
		Alloc:     decodePrealloc(testnetAllocData),
	}
}

// DefaultRinkebyGenesisBlock returns the Rinkeby network genesis block.
func DefaultRinkebyGenesisBlock() *Genesis {
	return &Genesis{
		Config:    params.RinkebyChainConfig,
		Timestamp: 1492009146,
		ExtraData: hexutil.MustDecode("0x52657370656374206d7920617574686f7269746168207e452e436172746d616e000000000000000000000000000000000000000042eb768f2244c8811c63729a21a3569731535f067ffc57839b00206d1ad20c69a1981b489f772031b279182d99e65703f0076e4812653aab85fca0f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		Alloc:     decodePrealloc(rinkebyAllocData),
	}
}

// DefaultGoerliGenesisBlock returns the Görli network genesis block.
func DefaultGoerliGenesisBlock() *Genesis {
	return &Genesis{
		Config:    params.GoerliChainConfig,
		Timestamp: 1548854791,
		ExtraData: hexutil.MustDecode("0x22466c6578692069732061207468696e6722202d204166726900000000000000e0a2bd4258d2768837baa26a28fe71dc079f84c70000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		Alloc:     decodePrealloc(goerliAllocData),
	}
}

// DefaultBaklavaGenesisBlock returns the Baklava network genesis block.
func DefaultBaklavaGenesisBlock() *Genesis {
	baklavaAlloc := &GenesisAlloc{}
	baklavaAlloc.UnmarshalJSON([]byte(baklavaAllocJSON))
	return &Genesis{
		Config:    params.BaklavaChainConfig,
		Timestamp: 0x5e8ca380,
		ExtraData: hexutil.MustDecode("0x0000000000000000000000000000000000000000000000000000000000000000f918cbf9042f94893c4d601ed879b4ad36fc31f0c0214d547113eb9475af50cac2b2eb330b975c1b999fef571c87087094ffff741c41fb487f4d64fcc9e32fbb38e2a8372c94c496e9791d39a6f0ae54ed52897e581d168a5b4594b182f37daee2285f14b4091b702ceccb00d5031294beeb24d10c9fe58715a0c853db8ddd8e0019177194aa3b76b6618afe574b278da9b71af0e66aa6f64694e2368b04a1d14f286faf8c90153e33dc0b0879fb9472bda6988c4551083c14dddd2709edee96a9b4a894f0d17b624521c0a599b063d73a73f9719307b48f9438f3eebb5a820ba4b83b1f6a324c09ec1db2d2f9942f916156a2dcd5dfb6054eb62a677c55c3a4037a94ee80387e93e4d6d41c4bf51623bc8a42ba55a44994d32e429fe2155971825e14e2fd89785301ab6dc3948c517b3bab0e2d65d5cdf3750ca360ff05b3fc1d9462d56ba5bb3b841fade256d97e7d4d57364fb88194beb2b0ea812caaaea3a1f894fa9138231e2c7d3894430c9d55cf6116c65482379c039584b9b965f32394821a6b00d9f658e1732ce741d91984c688e25c5c948339eae8ee5b8ea60d80711c9fe30ae92c0eba2f94a5f837aa6be966d9f293de6f8cc65fac7064220f94d2462858d04dfdb0c7ad26dfa21933526491c1c39442a237fd620f714536d6f63c0f46d092c9ea012294dce0096c358e90c4645d68a53e897416e786d05f94d10ddf202de766dbaaea7c436a90f5aa71e5bdc4942ec7df44dbbc3d458acc4108aac8d1cb0bec11c094b8f90aed575898c38c11deda226e1294f4d9dfcd948a63fe13a4be506e8b51d296f60f192458672e629480be4e53c6bf959445598965c2910c2c91ec81f7944663bf9f8c6b32f095126e138678732c0d183f7e94bafa573a3d5333fc88560de1c312992c56d517aa948a0d880f275d3741f507a0180e843a2805ec1e2e94cc8c30b74073f537a6f790ae68649788a66f2d7194ba7d40c43dc59e2686fb38edcbdd69c3e53eba34941db8f844aa731e4a20e139c7e7ebfeda26888cc9943acce236f234aa330395d50d6155f86c5169731894cd97f1e7cddff4523f03a5072e9be8ea7edbdaec94537bf2c6227ea6ce4eb250da02e6a6aaa3de4f7b9448cedc58b10af13d688631bc3cb78a05b8a6e56a94f410a55e2f4b49996a1cb884c3107490aee09330944ec65178abe63805b2b0ec2718280454d9b6353d9423836969f0d095afeb652c75523f740898e0c4629423e15d949c13e3010b75afd19b1d3b294310c7e6940ec5a403212d732d8d7ced050e9510f6327453c6940ec5a403212d732d8d7ced050e9510f6327453c694e224ceb83a39d2ffa7eaa0c501e6954db5667e59943c8afff5c9332161a20db995e2c831baf02d9f91940f409814cad27ce583de0d6a10ecf53d48b9535f94f6a969790936c5285f7a7d1af642707d0c8e2418947a450c89257bf0ead22f143493250907411cd1d694fb041f37e22b25e30e10cfd662e6a5f2d1305476f91386b8603695297deb0f970e1cd8c64a852b45ad788e151734ab05a20206be63f7f42817fb5e3e035d7f9dba56917a0bc75a3f0061e767a5cc5a60469709f170f4f7d76dcf6e30f04a520745ac8f3f131d6b854ea2a899c3317cc48bf06cdb07f4514a81b8606b628400822daba0c52481cd1f8708056c40c392b362a9028d17032b6324b114620118a2770e0ddccc717685c2380600d035299ff9fc354180d251acaa778cac2a2eb64466e977204e582ea1342c0100c01ff5ea558be47b1f36cfbaee7a1e80b860f6c310b565f6f975faf4326707551a9154b848a222c14c6e18b87c0bd9bcf3524d95b70d5faffe1d911955186f160d01c82d4df63cc8dbc52ef2e106c89d2b838c57a4e6fd033d7d4cd49698111fd78321a962d03b1a31eefdc84397c0d51b80b860baa5538d5579ce32668fbf4e80cd9fd31f09447ba473da1ae44a51ce6feb4f46d8a14008fc515fa7e0e9d720c83b7001bdbfbce56284c382c5a267e7a245daeaa9e2914dc92cf2aa2ebcde339e309c5d1a0df1533cbd631168fc3e4a1d248801b8605a13510aa9b74ecd0bb7d8bd7f65dd5622ff4a61eebc1976529a26954d70732d591589dcf49e3f8a87f450642336e200391e1e8b57afe3d7240b80020deef9db2d9b260fc3f30ab4168521de586c211baccb505b75af030d47bd1d6fd49ab480b860382d76c49b99ba0d0aab030afe569b89288f399c727e3c1dcefbe1208f0c2d86498f62e1fee2d50486e2d6aa0d61130172cd07019fdfe79cbbec315f53465d9a8121fcdcfe71efede293111915281e82add7252b88b540b3a0dcd2fcfec91c81b860348aa54a71033d583a29d00e3dfb1f696633c6520bc71099fd26a228d1882f380ab86ef19b7efe544d47ba8aeddf770125075f6c15b82611a31e746668c1aa8af2355efe6e4cd297c0f570fce18ecc2f274bef5115a8d8ba9a18a6563e6c6181b86017a5c4c6a8aa852ab1ac7771e3caa6546da92938bda6d4e0c0ea05da85adbe08ca2ee58e657a82b619d1a729b6510401686ca15382abdbd856716fa867573c35ab8605d2d57b0573ff589c087cf57e5408cbf7be57ce5946a6455b1955d9d300b8609887deb26c20c4b5b448e04efe7b18781355f9648feb47d978382aa3c789f545416d20bca757df66b70867ccffeba0014bb1b45fafe0366aff4d513c023a55ee35ccc5a86d5214f813996e2ded810e3cac11cb15b7ca0b97aad1a307897d4d01b86057b5158540d1672de49073e469e5426766094ce6514ac29580803ff9d22cdef3b6fed28a8ad9de63bf938c1c7b0e7a0156fb03f5363d09bf337713d8d45475df48ccea0fa793f591fbb62673545fc846c04e6aa9e2eaaab72c3b9ba4069ea280b860720216212176780b12cd297d1feff6a964329448d821fd69e0a4fcc53c4edc00ca9c89ab2c35abe7b68cd47bc230bd00597450ca0bea04562f78b0102d406f4b651957989c1471674eb16a531c2f762c84c3432ccd7fab87b0a185a942d00301b86068438d70fb61a2d0adc3710b619aa5f9a603168f93bf0504401464df3acb14cc1815bf449b7f8791fef7e309b60e7b0145d425515d1f4f380b66c256a4218700ae48e2d2185a636487d6f531f546b9127b957084d0821acccf5ad0776b06be80b86092b17fb70e62710f3b87642830e23258a0fd1041da9606c7eea7019f86da50ff4cd06ad48a4d0ca93e62a6c7ed6c5e0036f4443d0cfe9122156a346836a9163404d58856bbd447939189bf1cc8a7ee8be156415642992f8e0659526d15bb0c00b860bfd4b2392664c68bbd30d6a77cd1c7230e340ac65ecc07449c0d704096af06926238c60e7764ffcd0b9a53d82d7050009cfd9481ebd7630fd2540b834cef70b5276d07b75913a20222d74f36db8beabfcd39f7c5d57c4a28f87e8e737123bd80b8602a10c24fb6cfb029650a90f10f6586f26e61e3a51d68342785862e666f6c730bb929ff2c5a20f5ceff41cb53a2333c00aa0e4b0e47c821c668d6b84fac85c5b5cd360c53dfdf9a6c48e0f4ff8c3997ff191463f90fa644fc2870174d67885b80b8608aed791b4381c7bd38f8358cbb7f64bb6b74c34b85b27026745df7cae91f5cdd50ba6cadd919494ec9849e628138870188ef21da8282207be4acd30803288e2674a7c46322f41f3ecf685f5b635970c0d19802defc344cc4023f8e41e50e3c00b8605008d7aece8407c044c4a7a77b37690a08b0edbde0942446f31e3362031e689b30872d8732f74dd3e45bbe85c072a4005ad20cf2d9e0824eed9a607b980d0cd5d160bcbd5ffc34a6a87176e257c927cd419ac080df27cce888eb94c7bc5a3880b860a4ef4602775c99871c1f910af02cc997cb6f6b1524450898a1de0a3369750ce17deda194ce40c80650025770e1c60a008916618d314d7008a44e312f2f032fedd08177ef854cf78dc269122dd7eef415989e975f13d501830df4ecd3fe248700b860f01e9ee01d1a76e596827b897e39a1e497b00d6ed276675201e81a07509d4043354c89d0ed515098b0730958cc9dd200fd9cf9d1f2fdd7823937af05e03eb1b043de63e2c8ff7bfebc9cd4f3983243bd275d0266bbaee5e70244126687470f00b8601f599e48d2f04e623c42ae66d379f15ff8c1a3bc4584a27f234fc00e7feb64dcee6a0faa4cfc3737e507b9c38d640401b23b2c6fb9bb7f00a2144b8031b9b2446ac121e7ca7f06e0b6e084b082ada9fc31837d094b0d6c3d24965a9a61fa7a01b860c2cc4ba57abb3768f552b30768ad8de8d6b934d8fd2eca699a2b2f712e8d56b50ea2e59993175a396e471e3a6028b30054cf156b0c75b9df6ea2e15e03811960aa5999411522b01286bc1cf17e4ef366a48e3e075998394e090f46c1d8212c81b8603201cd2f5f5f41b6949a03a600347bf9920c891b883dc4c8a530188fec4f98ac96d6caf9e87a05c81962760c20c30c0198745f18efc87a19ec94a06cb01d72313a71f4ff30e94f04504e609cd495ed4b78dccdbdc6ece12d0200996763439c00b860b1d7e87a67d355c42cf7db5c599da2ce9624372cb941f338d68b80bb8376fc8600e6d925d9902f32270d6372d4215c0068879d52d16dda84ac2efbebbde57d7ca5d451f269aa9903926163d7ce75b68d2547c34e07303b9db69d0044c9219200b860872c5c11351fbe7f31bb38ff5f7b07aa8bc4902a06b81f4fe70e7bc5be86ad260be53c26ef3d1e6293e3308c33ef8d000c13182ad835adf86e816496d6519ae00bd607f6f5d974fab0792802d69688e6ef37078285a1a2e9f8489bfe5a87e600b8606afab29c7f64915ebcd2851799af2f43cebbab8fe051965103989c47e576181533f7bf64619a4ee50bd787bcb65f5b012d60c2eadaad3118cba6088cfb9204f5cc465d47dd4c834b568f313d5b0689c6b6f3734cd19d4df40185d007b22d3681b86055470892fbef78fa87bf31aa738a58fdca928a16d99fff12e8ec3d71fb89489286206e32aed5063f1fdd6294f8a3a2011e19cb83873a777741f3d8521ea33edac732704e36f1aac01f212eaf72405cf8d980dd88bcb88751a6bbe7207a5e1081b860207a4c7a86274921d013932e2f88ec795dfcb1f85df6bba7039fd14526cd44245f29d5916d17bebf40ed08be3d0541006fb3443238819cdf36eff79d8706ceda92b0b3bf05d6dfd9a723da2685acc34e59928fed096dea64f742fb55c7f34c80b8603891197bd1712a3cf13fc13f2f2308bb4acdd4e46e3a62abd28a8e3add64ebcabfb939c743e8e4d54da2a32c39719d013ca29923eb2c685ab851a4f644824d70b1fd36812f513f5f23fa8f82ea228ba2d8f8b771b109e99e1905e16b1899eb00b860efff9b35b865ad56dea6f6c80d96ae4ba88997a60451b1535ea51ef6ae1d26a1c25246f33139f64046909d4a30a90f00a782c6a5a910499b65d7f2c3064dca95cb6dda83c10c2f9dc8f60d1afeb91c067205dff722dfe964f66a9a98e4047800b8606dd6f3f642e00ffc73a2d76c8f58128aa434b526c0487487f193b6717665a2eccff314d7d0769dc17fad47db19981a00c8a11f156a2bdb36164b479cb1bcf636896066e3c3b561c478d357b343bfbc6aed51b08f60b8f804ebd78e2efb525c80b8608202e06fb2d18ad9a85b3af760290ca4aae79fe2d354ec4c462f65e98339f9bfbf6c24503c73b054bfa740ea595241011c0d599d95697238c700a380326dea544cd7b085a3b8db22ae900ac82a5c23ade8c4290e022b0e425a55f742352dc480b860acc4826a9317688bdf782adc5acdf12656ffc2f327ca699418c287a15c2c22e0d16e632e592f5b0748b40a8e374e33009d8cfdfef7f81493a77843054cd006858b1b217043954423bb0924d0387b6609a504e15e6c054fe1288e0bd337253100b8606c93b4d43d69c248efadf8d03a4c9672ea222830a1530b29deab346933fc4492a211223c446ea24af059d9528ab3c100f0c4769fb226335489969277140da7c8c7f036d76f73c1d5f359744018b76b13c0eb07812f08923649e4a45b3b69fc80b86086ef363fcabe2e3c27769de9974f4b8604b47cdb2fe57208f0a59899b77306b7770f7403b222b6222efeb67747d06e01fc2ac657a21f37ab982d8463ba9c7fafaeda20370ec3f6cc565dbec4a69f59f9d7404725e3b701681d41a870b1f42281b86051026c0f52d8612414359ee4e6049a275387d3033cf51a8e23b3e037bd7006d916017b5253da7068532e6242b1d96900d8130a60b9b0b3ecb06a08472728bddd55f778e485188c3717b911039ebe3b2cdd89573229bdacaecba9a220b2decb80b8606a2cc88ae37c395815dcedb56f5ee6b701e06e97bdb6232c2b0e21f719a1504d0f3b259a63e2e54d6b1a27cbb0b051007f0c39d42f699cfc608c25b636ea221500d0eb75fe0f26d35beec47e1161675a7f571ed70684778ee7c4bc6221d8af80b860c5f05101b67ffd245abb878fc8d68058e3a65b4672453b90cb116a0cb8f59879a73944d8a2c25e8a2865e07e1e4e0100cd255dbd7055b5228eb1d4cccea7a1288ef83bab770633b0054f8c8d9ad4e7fc15af631c2b2282337faddb2220e54901b8603178c750f9274f0f1ea4103997e7d469b2ef4b9dd2f1dff7dc33ae5aaa5fdfa32a9e44e844de658502fbd8925e9a8a00d4422f22a7a5c3e4a790d21334eb70a744da6a863c4e060481781b8bb3fd1e9291ebfb548f7d8ec122c2c0c43eb46000b8607983d25868e1915ca8c20603bb70475b398acb04fef221106475f0fc25a3df25d181050214da347bfcd8f8292e5257014ba77cf67ee5f42202257317808f126930b4eaaf4bec801c4465ced54bd78331f8d3dadfdf2f91de294836c13e946001b86061810f2366f415b7b67e0ff9c82ae55fa213a95ff61df8718e806931ce135c0324462a33ccbde34ff901025ac334640064b41f7854c6eabcf3d9f7a4e4ff90522a4ba2681a0df03d08c67fb79f0fe98cd8c6f5ad3df155a0210ff84fc93d1500b8606a50718246d5c167429ce48b757a28441b217eb0903d762a3039d0ee59c00584b05418920be4eacb2fe2051d492a3d003a2a00065986e15815c26434c68caf177cbea44a38a886e86b34dba8a17dca43b729b9d69007e25d735e0ec62f7b6d81b860424b8239213c6f2c3e181b815294b9dd78663e29ee1234e1511e584b7b736571a6799135381cced36dcbd12dd19a5901f479679592608ab7f98aa1f607a3fe4b5b29a6580f724086610c901f143be653dcaf9fd477ac4d21d263e8ad8d048681b860f03d1c933feb791285892c9e6552bc6ccf07dd7f66862ff5554844db033c17a005087b5847718d4611e3ecb293ce670144a99a13ad3356ca56535815a0c6c21c2f84bbc0b40c78f6b63137a17709fa4be124be10e90a42bda72aee977ca86881b860fb9de050c059a11175be23daeef1bfc374a688d8fd776e7d2dc81fe301068a3af03ad522bd915a154dc8b5b8e12d38003f17bf9f3001e8bc682e38cfc93c6901417afe1ddbfe11b04f42ca4f321f7ba73fa98f0e8c8d75781b14c0dea847a500b860fb9de050c059a11175be23daeef1bfc374a688d8fd776e7d2dc81fe301068a3af03ad522bd915a154dc8b5b8e12d38003f17bf9f3001e8bc682e38cfc93c6901417afe1ddbfe11b04f42ca4f321f7ba73fa98f0e8c8d75781b14c0dea847a500b8600fa447ead6255657aa79229944a3ddf36984c7b9011e859f90bc6567ab61133b27b6bfa3a5e322dcf36abff543cc0d0131066520ad92d12b8603c25c561bf48806f178a995d233862038c46e81933e1ab0a7211b9b0933499cbc5867db736481b8602e45f40c799aec72e87e0ba4713056dd17cc9bdb29bb221c8fd6377336ae773afa84b8c0bb8f1f2edc88975b8ec986006f321462e28706618d690d24337a9e1c7b96e74d8c8f2fe740bac461635b4828046227f92f97b13da8540bdcc2f1a380b8602b8ab0795a174e96061dbb8c32bd38468497e56379b2ac79f6a7506f711b817cb57c8c1d8743d88c436cdcde3b51980132c5508f79cfc477bbd1b94fefd5c408a7072e85041f17042c1ad055c04e67a780f497e60aac3c38ddb2f0004237aa01b8603729df3ecff9e168c1ab2b2698251797494847ee14bffe9c4ce121d90d381b0d51f7208c793f55a9cac9d50e25d63d0018ee5daa523319bbd5087dc35cef8ec53338d810adf8b4565f39fccdddd7596a63198f045a2f3d92bd0b72b4487bcd00b86044a466fb5cf01c878071e12dc391753cb59005e39c6604fa0d1444ae36518df771f252891072c26652e12a5a5b1f3c00f479f3e3ace6412768ee5729825bac3529cfde3b44cc258b847dbd87f6d55dd68124d054314eea0a9169b36e46e34081b860c6ec185bce68a0a1fc78f78a5a9da12d39635f94b6981a64d7ed7a095839b6839da577383ddf9ef940d93409ec305801cda7a06f3c3adde48e9060ca396bd8a2821fbc435842bba5ba1dc0b81cfef04cdcf0ca7b02b6e4d70ca9caed8438708080b8410000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000f86480b86000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000080f86480b86000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000080"),
		Alloc:     *baklavaAlloc,
	}
}

func DefaultAlfajoresGenesisBlock() *Genesis {
	alfajoresAlloc := &GenesisAlloc{}
	alfajoresAlloc.UnmarshalJSON([]byte(alfajoresAllocJSON))
	return &Genesis{
		Config:    params.AlfajoresChainConfig,
		Timestamp: 0x5b843511,
		ExtraData: hexutil.MustDecode("0x0000000000000000000000000000000000000000000000000000000000000000f905bbf8d294456f41406b32c45d59e539e4bba3d7898c3584da94dd1f519f63423045f526b8c83edc0eb4ba6434a494050f34537f5b2a00b9b9c752cb8500a3fce3da7d94cda518f6b5a797c3ec45d37c65b83e0b0748edca94b4e92c94a2712e98c020a81868264bde52c188cb94ae1ec841923811219b98aceb1db297aade2f46f394621843731fe33418007c06ee48cfd71e0ea828d9942a43f97f8bf959e31f69a894ebd80a88572c855394ad682035be6ab6f06e478d2bdab0eab6477b460e9430d060f129817c4de5fbc1366d53e19f43c8c64ff903d4b86011877b768127c8eb0f122fbe69553bc9d142d27c06a85c6eeb7b8b457f511e50c33a57fcbc5fd6d1823f69a111f8010151a17f6a8798a25343f5403b1e6a595c7d9698af3db78b013d26a761fc201b3cf793be5f0a0a849b3f68a8bfa81e7001b860d882cd4cc09109928e9517644d5303610155978cf5e3b7ad6122daa19c3dab3da8c439bc763d6d3eef18a38ebb0d3200664b94fab11adbb3f44b963969763b590af45931c482396be88a185214c9c8690615aae5197e852bc1d04b3dbd03ab80b86051588d46ba8998d944a30cde93bfe946e774ef1f6fe2fb559a74ffebf60d1ad967b876a038c6e312d0c20752cbc8440012293b6ea417f32a163caedeaaae7aad3c1b31be1fe86c405924b1be7d0aaae6f3ba567ee907d0d4c00dce5091442380b8601f2becc31c1f0141e8c5768c5f07d02d1342c086c037cce70aaf3629b40ea017884a81163f58697b020b21fe39c440006970bc1f52b847d7262599ae92ee7db45ad38efe5612c8ed42d9db9380da0769bab713f5259b7c015998296bf02a0a01b860d02ec615b916bba4fe7e65a3d79e607aa27bb5a84b0c2f242e9d8f379512cf40051a43030e55aca965d91c905b656d006434d95b7034bfc2e5e2ef7384e8cd640efae740558216f6f9db24c6d1acf755746dfbb68c76961593741105725d5680b860d6e86d5e73db3b3a2c96c6caa1a7e153e17adb13fb541943a44bfa90beab38aa73ad453d918fea2ba57c0a67115d0401c56946d8894f346d796864e9344fd1439dd1345de762f85d7e18e311b35c3cbe492886ef8bc872b4aabfa23c2e38a901b8601cf59939da60cdb9aff09f76e6070a17fa21356ca7016390ef4444243e12ab7ed7a233d7ca48b0d17870ba015a4410014e5cac8d456e03ec2908d347627d5e9ecd496ce990d10900ddc529300eef3d037e48d79f03ad2b6bcd48affe2ddf2681b8601cfe8876c0b89ef15128bb27eb69e7939b4a888b0a81195d5fd1bbda748a29838274e652dcf857f4090bb85343055300ca3e75a980b100403d3b6d34f62c6a86bbd75203391c63dd405725c69241a828e6892f623ed5b35c8dc132b032061201b860a6fc71d63c5adedb7b30b9e0ba3d83debf86d12ba235c13584a9cbad410f082030427be4f8a9127889979c3eea58860031af128deece487df5aef9d999c8dc2fb51f308eb1ee229e6bbd6860138d4fcf4209eb7bec62ca70dd8643104003c200b8606b7adb5d01e3fd72ae2c4ff17e6620dc383431e0ebe06c9af5b94207f380287429043e7bbe417b82d0aed2e43dc7b8002bb52886773e4a2c23bf0ebfd401471e8da3cf3a0a7e0949d9ad4de38138a787a975993ba311525ce8be331cd60d670080b8410000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000f86480b86000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000080f86480b86000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000080"),
		Alloc:     *alfajoresAlloc,
	}
}

// DefaultOttomanGenesisBlock returns the Ottoman network genesis block.
func DefaultOttomanGenesisBlock() *Genesis {
	return &Genesis{
		Config:    params.OttomanChainConfig,
		Timestamp: 1496993285,
		ExtraData: hexutil.MustDecode("0x0000000000000000000000000000000000000000000000000000000000000000f89af85494475cc98b5521ab2a1335683e7567c8048bfe79ed9407d8299de61faed3686ba4c4e6c3b9083d7e2371944fe035ce99af680d89e2c4d73aca01dbfc1bd2fd94dc421209441a754f79c4a4ecd2b49c935aad0312b8410000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000c0"),
		Alloc:     decodePrealloc(ottomanAllocData),
	}
}

// DeveloperGenesisBlock returns the 'geth --dev' genesis block. Note, this must
// be seeded with the
func DeveloperGenesisBlock(period uint64, faucet common.Address) *Genesis {
	// Override the default period to the user requested one
	config := *params.DefaultChainConfig

	// Assemble and return the genesis with the precompiles and faucet pre-funded
	return &Genesis{
		Config:    &config,
		ExtraData: append(append(make([]byte, 52), faucet[:]...), make([]byte, crypto.SignatureLength)...),
		Alloc: map[common.Address]GenesisAccount{
			common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, // ECRecover
			common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, // SHA256
			common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, // RIPEMD
			common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, // Identity
			common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, // ModExp
			common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, // ECAdd
			common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, // ECScalarMul
			common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, // ECPairing
			faucet:                           {Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))},
		},
	}
}

func decodePrealloc(data string) GenesisAlloc {
	var p []struct{ Addr, Balance *big.Int }
	if err := rlp.NewStream(strings.NewReader(data), 0).Decode(&p); err != nil {
		panic(err)
	}
	ga := make(GenesisAlloc, len(p))
	for _, account := range p {
		ga[common.BigToAddress(account.Addr)] = GenesisAccount{Balance: account.Balance}
	}
	return ga
}
