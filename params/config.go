// Copyright 2016 The go-ethereum Authors
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

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"golang.org/x/crypto/sha3"
)

// Genesis hashes to enforce below configs on.
var (
	MainnetGenesisHash   = common.HexToHash("0x19ea3339d3c8cda97235bc8293240d5b9dadcdfbb5d4b0b90ee731cac1bd11c3")
	AlfajoresGenesisHash = common.HexToHash("0xe423b034e7f0282c1b621f7bbc1cea4316a2a80b1600490769eae77777e4b67e")
	BaklavaGenesisHash   = common.HexToHash("0xbd1db1803638c0c151cdd10179c0117fedfffa4a3f0f88a8334708a4ea1a5fda")
)

var (
	MainnetNetworkId   = uint64(42220)
	BaklavaNetworkId   = uint64(62320)
	AlfajoresNetworkId = uint64(44787)
)

var NetworkIdHelp = fmt.Sprintf("Mainnet=%v, Baklava=%v, Alfajores=%v", MainnetNetworkId, BaklavaNetworkId, AlfajoresNetworkId)

// TrustedCheckpoints associates each known checkpoint with the genesis hash of
// the chain it belongs to.
var TrustedCheckpoints = map[common.Hash]*TrustedCheckpoint{}

// CheckpointOracles associates each known checkpoint oracles with the genesis hash of
// the chain it belongs to.
var CheckpointOracles = map[common.Hash]*CheckpointOracleConfig{}

var (
	// MainnetChainConfig is the chain parameters to run a node on the main network.
	MainnetChainConfig = &ChainConfig{
		ChainID:             big.NewInt(int64(MainnetNetworkId)),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		ChurritoBlock:       big.NewInt(6774000),
		DonutBlock:          big.NewInt(6774000),
		EspressoBlock:       big.NewInt(11838440),
		GForkBlock:          nil,
		Istanbul: &IstanbulConfig{
			Epoch:          17280,
			ProposerPolicy: 2,
			BlockPeriod:    5,
			RequestTimeout: 3000,
			LookbackWindow: 12,
		},
	}

	// BaklavaChainConfig contains the chain parameters to run a node on the Baklava test network.
	BaklavaChainConfig = &ChainConfig{
		ChainID:             big.NewInt(int64(BaklavaNetworkId)),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		ChurritoBlock:       big.NewInt(2719099),
		DonutBlock:          big.NewInt(5002000),
		EspressoBlock:       big.NewInt(9195000),
		GForkBlock:          nil,
		Istanbul: &IstanbulConfig{
			Epoch:          17280,
			ProposerPolicy: 2,
			BlockPeriod:    5,
			RequestTimeout: 3000,
			LookbackWindow: 12,
		},
	}

	// AlfajoresChainConfig contains the chain parameters to run a node on the Alfajores test network.
	AlfajoresChainConfig = &ChainConfig{
		ChainID:             big.NewInt(int64(AlfajoresNetworkId)),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		ChurritoBlock:       big.NewInt(4960000),
		DonutBlock:          big.NewInt(4960000),
		EspressoBlock:       big.NewInt(9472000),
		GForkBlock:          nil,
		Istanbul: &IstanbulConfig{
			Epoch:          17280,
			ProposerPolicy: 2,
			BlockPeriod:    5,
			RequestTimeout: 10000,
			LookbackWindow: 12,
		},
	}

	TestChainConfig = &ChainConfig{
		ChainID:        big.NewInt(1337),
		HomesteadBlock: big.NewInt(0),

		DAOForkBlock:   nil,
		DAOForkSupport: false,

		EIP150Block: big.NewInt(0),
		EIP150Hash:  common.Hash{},

		EIP155Block: big.NewInt(0),
		EIP158Block: big.NewInt(0),

		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),

		ChurritoBlock: big.NewInt(0),
		DonutBlock:    big.NewInt(0),
		EspressoBlock: big.NewInt(0),
		GForkBlock:    nil,

		Istanbul: &IstanbulConfig{
			Epoch:          30000,
			ProposerPolicy: 0,
		},

		FullHeaderChainAvailable: true,
		Faker:                    true,
		FakeBaseFee:              common.Big0,
	}
	IstanbulTestChainConfig = TestChainConfig.
				deepCopy().
				OverrideFakerConfig(false).
				OverrideChainIdConfig(big.NewInt(1337)).
				OverrideIstanbulConfig(
			&IstanbulConfig{
				Epoch:          300,
				ProposerPolicy: 0,
				RequestTimeout: 1000,
				BlockPeriod:    1,
			},
		)
	TestRules = TestChainConfig.Rules(new(big.Int))
)

// TrustedCheckpoint represents a set of post-processed trie roots (CHT and
// BloomTrie) associated with the appropriate section index and head hash. It is
// used to start light syncing from this checkpoint and avoid downloading the
// entire header chain while still being able to securely access old headers/logs.
type TrustedCheckpoint struct {
	SectionIndex uint64      `json:"sectionIndex"`
	SectionHead  common.Hash `json:"sectionHead"`
	CHTRoot      common.Hash `json:"chtRoot"`
	BloomRoot    common.Hash `json:"bloomRoot"`
}

// HashEqual returns an indicator comparing the itself hash with given one.
func (c *TrustedCheckpoint) HashEqual(hash common.Hash) bool {
	if c.Empty() {
		return hash == common.Hash{}
	}
	return c.Hash() == hash
}

// Hash returns the hash of checkpoint's four key fields(index, sectionHead, chtRoot and bloomTrieRoot).
func (c *TrustedCheckpoint) Hash() common.Hash {
	var sectionIndex [8]byte
	binary.BigEndian.PutUint64(sectionIndex[:], c.SectionIndex)

	w := sha3.NewLegacyKeccak256()
	w.Write(sectionIndex[:])
	w.Write(c.SectionHead[:])
	w.Write(c.CHTRoot[:])
	w.Write(c.BloomRoot[:])

	var h common.Hash
	w.Sum(h[:0])
	return h
}

// Empty returns an indicator whether the checkpoint is regarded as empty.
func (c *TrustedCheckpoint) Empty() bool {
	return c.SectionHead == (common.Hash{}) || c.CHTRoot == (common.Hash{}) || c.BloomRoot == (common.Hash{})
}

// CheckpointOracleConfig represents a set of checkpoint contract(which acts as an oracle)
// config which used for light client checkpoint syncing.
type CheckpointOracleConfig struct {
	Address   common.Address   `json:"address"`
	Signers   []common.Address `json:"signers"`
	Threshold uint64           `json:"threshold"`
}

// ChainConfig is the core config which determines the blockchain settings.
//
// ChainConfig is stored in the database on a per block basis. This means
// that any network, identified by its genesis block, can have its own
// set of configuration options.
// This configuration is intentionally not using keyed fields to force anyone
// adding flags to the config to also have to set these fields.
type ChainConfig struct {
	ChainID *big.Int `json:"chainId"` // chainId identifies the current chain and is used for replay protection

	HomesteadBlock *big.Int `json:"homesteadBlock,omitempty"` // Homestead switch block (nil = no fork, 0 = already homestead)

	DAOForkBlock   *big.Int `json:"daoForkBlock,omitempty"`   // TheDAO hard-fork switch block (nil = no fork)
	DAOForkSupport bool     `json:"daoForkSupport,omitempty"` // Whether the nodes supports or opposes the DAO hard-fork

	// EIP150 implements the Gas price changes (https://github.com/ethereum/EIPs/issues/150)
	EIP150Block *big.Int    `json:"eip150Block,omitempty"` // EIP150 HF block (nil = no fork)
	EIP150Hash  common.Hash `json:"eip150Hash,omitempty"`  // EIP150 HF hash (needed for header only clients as only gas pricing changed)

	EIP155Block *big.Int `json:"eip155Block,omitempty"` // EIP155 HF block
	EIP158Block *big.Int `json:"eip158Block,omitempty"` // EIP158 HF block

	ByzantiumBlock      *big.Int `json:"byzantiumBlock,omitempty"`      // Byzantium switch block (nil = no fork, 0 = already on byzantium)
	ConstantinopleBlock *big.Int `json:"constantinopleBlock,omitempty"` // Constantinople switch block (nil = no fork, 0 = already activated)
	PetersburgBlock     *big.Int `json:"petersburgBlock,omitempty"`     // Petersburg switch block (nil = same as Constantinople)
	IstanbulBlock       *big.Int `json:"istanbulBlock,omitempty"`       // Istanbul switch block (nil = no fork, 0 = already on istanbul)
	// MuirGlacierBlock    *big.Int `json:"muirGlacierBlock,omitempty"`    // Eip-2384 (bomb delay) switch block (nil = no fork, 0 = already activated)
	// BerlinBlock         *big.Int `json:"berlinBlock,omitempty"`         // Berlin switch block (nil = no fork, 0 = already on berlin)
	// LondonBlock         *big.Int `json:"londonBlock,omitempty"`         // London switch block (nil = no fork, 0 = already on london)
	ChurritoBlock *big.Int `json:"churritoBlock,omitempty"` // Churrito switch block (nil = no fork, 0 = already activated)
	DonutBlock    *big.Int `json:"donutBlock,omitempty"`    // Donut switch block (nil = no fork, 0 = already activated)
	EspressoBlock *big.Int `json:"espressoBlock,omitempty"` // Espresso switch block (nil = no fork, 0 = already activated)
	GForkBlock    *big.Int `json:"gForkBlock,omitempty"`    // G Fork switch block (nil = no fork, 0 = already activated)

	Istanbul *IstanbulConfig `json:"istanbul,omitempty"`
	// This does not belong here but passing it to every function is not possible since that breaks
	// some implemented interfaces and introduces churn across the geth codebase.
	FullHeaderChainAvailable bool // False for lightest Sync mode, true otherwise

	// Requests mock engine if true
	Faker       bool     `json:"faker,omitempty"`
	FakeBaseFee *big.Int `json:"baseFee,omitempty"`
}

// IstanbulConfig is the consensus engine configs for Istanbul based sealing.
type IstanbulConfig struct {
	Epoch          uint64 `json:"epoch"`                 // Epoch length to reset votes and checkpoint
	ProposerPolicy uint64 `json:"policy"`                // The policy for proposer selection
	LookbackWindow uint64 `json:"lookbackwindow"`        // The number of blocks to look back when calculating uptime
	BlockPeriod    uint64 `json:"blockperiod,omitempty"` // Default minimum difference between two consecutive block's timestamps in second

	// The base timeout for each Istanbul round in milliseconds. The first
	// round will have a timeout of exactly this and subsequent rounds will
	// have timeouts of this + additional time that increases with round
	// number.
	RequestTimeout uint64 `json:"requesttimeout,omitempty"`
}

// String implements the stringer interface, returning the consensus engine details.
func (c *IstanbulConfig) String() string {
	return "istanbul"
}

// String implements the fmt.Stringer interface.
func (c *ChainConfig) String() string {
	var engine interface{}
	if !c.Faker {
		if c.Istanbul != nil {
			engine = c.Istanbul
		} else {
			engine = "unknown"
		}
	} else {
		engine = "MockEngine"
	}
	return fmt.Sprintf("{ChainID: %v Homestead: %v DAO: %v DAOSupport: %v EIP150: %v EIP155: %v EIP158: %v Byzantium: %v Constantinople: %v Petersburg: %v Istanbul: %v Churrito: %v, Donut: %v, Espresso: %v, GFork: %v, Engine: %v}",
		c.ChainID,
		c.HomesteadBlock,
		c.DAOForkBlock,
		c.DAOForkSupport,
		c.EIP150Block,
		c.EIP155Block,
		c.EIP158Block,
		c.ByzantiumBlock,
		c.ConstantinopleBlock,
		c.PetersburgBlock,
		c.IstanbulBlock,
		// c.MuirGlacierBlock,
		// c.BerlinBlock,
		// c.LondonBlock,
		c.ChurritoBlock,
		c.DonutBlock,
		c.EspressoBlock,
		c.GForkBlock,
		engine,
	)
}

// IsHomestead returns whether num is either equal to the homestead block or greater.
func (c *ChainConfig) IsHomestead(num *big.Int) bool {
	return isForked(c.HomesteadBlock, num)
}

// IsDAOFork returns whether num is either equal to the DAO fork block or greater.
func (c *ChainConfig) IsDAOFork(num *big.Int) bool {
	return isForked(c.DAOForkBlock, num)
}

// IsEIP150 returns whether num is either equal to the EIP150 fork block or greater.
func (c *ChainConfig) IsEIP150(num *big.Int) bool {
	return isForked(c.EIP150Block, num)
}

// IsEIP155 returns whether num is either equal to the EIP155 fork block or greater.
func (c *ChainConfig) IsEIP155(num *big.Int) bool {
	return isForked(c.EIP155Block, num)
}

// IsEIP158 returns whether num is either equal to the EIP158 fork block or greater.
func (c *ChainConfig) IsEIP158(num *big.Int) bool {
	return isForked(c.EIP158Block, num)
}

// IsByzantium returns whether num is either equal to the Byzantium fork block or greater.
func (c *ChainConfig) IsByzantium(num *big.Int) bool {
	return isForked(c.ByzantiumBlock, num)
}

// IsConstantinople returns whether num is either equal to the Constantinople fork block or greater.
func (c *ChainConfig) IsConstantinople(num *big.Int) bool {
	return isForked(c.ConstantinopleBlock, num)
}

// IsPetersburg returns whether num is either
// - equal to or greater than the PetersburgBlock fork block,
// - OR is nil, and Constantinople is active
func (c *ChainConfig) IsPetersburg(num *big.Int) bool {
	return isForked(c.PetersburgBlock, num) || c.PetersburgBlock == nil && isForked(c.ConstantinopleBlock, num)
}

// IsIstanbul returns whether num is either equal to the Istanbul fork block or greater.
func (c *ChainConfig) IsIstanbul(num *big.Int) bool {
	return isForked(c.IstanbulBlock, num)
}

// IsChurrito returns whether num represents a block number after the Churrito fork
func (c *ChainConfig) IsChurrito(num *big.Int) bool {
	return isForked(c.ChurritoBlock, num)
}

// IsDonut returns whether num represents a block number after the Donut fork
func (c *ChainConfig) IsDonut(num *big.Int) bool {
	return isForked(c.DonutBlock, num)
}

// IsEspresso returns whether num represents a block number after the Espresso fork
func (c *ChainConfig) IsEspresso(num *big.Int) bool {
	return isForked(c.EspressoBlock, num)
}

// IsGFork returns whether num represents a block number after the G fork
func (c *ChainConfig) IsGFork(num *big.Int) bool {
	return isForked(c.GForkBlock, num)
}

// CheckCompatible checks whether scheduled fork transitions have been imported
// with a mismatching chain configuration.
func (c *ChainConfig) CheckCompatible(newcfg *ChainConfig, height uint64) *ConfigCompatError {
	bhead := new(big.Int).SetUint64(height)

	// Iterate checkCompatible to find the lowest conflict.
	var lasterr *ConfigCompatError
	for {
		err := c.checkCompatible(newcfg, bhead)
		if err == nil || (lasterr != nil && err.RewindTo == lasterr.RewindTo) {
			break
		}
		lasterr = err
		bhead.SetUint64(err.RewindTo)
	}
	return lasterr
}

// CheckConfigForkOrder checks that we don't "skip" any forks, geth isn't pluggable enough
// to guarantee that forks can be implemented in a different order than on official networks
func (c *ChainConfig) CheckConfigForkOrder() error {
	type fork struct {
		name     string
		block    *big.Int
		optional bool // if true, the fork may be nil and next fork is still allowed
	}
	var lastFork fork
	for _, cur := range []fork{
		{name: "homesteadBlock", block: c.HomesteadBlock},
		{name: "eip150Block", block: c.EIP150Block},
		{name: "eip155Block", block: c.EIP155Block},
		{name: "eip158Block", block: c.EIP158Block},
		{name: "byzantiumBlock", block: c.ByzantiumBlock},
		{name: "constantinopleBlock", block: c.ConstantinopleBlock},
		{name: "petersburgBlock", block: c.PetersburgBlock},
		{name: "istanbulBlock", block: c.IstanbulBlock},
		{name: "churritoBlock", block: c.ChurritoBlock},
		{name: "donutBlock", block: c.DonutBlock},
		{name: "espressoBlock", block: c.EspressoBlock},
		{name: "gForkBlock", block: c.GForkBlock},
	} {
		if lastFork.name != "" {
			// Next one must be higher number
			if lastFork.block == nil && cur.block != nil {
				return fmt.Errorf("unsupported fork ordering: %v not enabled, but %v enabled at %v",
					lastFork.name, cur.name, cur.block)
			}
			if lastFork.block != nil && cur.block != nil {
				if lastFork.block.Cmp(cur.block) > 0 {
					return fmt.Errorf("unsupported fork ordering: %v enabled at %v, but %v enabled at %v",
						lastFork.name, lastFork.block, cur.name, cur.block)
				}
			}
		}
		// If it was optional and not set, then ignore it
		if !cur.optional || cur.block != nil {
			lastFork = cur
		}
	}
	return nil
}

func (c *ChainConfig) checkCompatible(newcfg *ChainConfig, head *big.Int) *ConfigCompatError {
	if isForkIncompatible(c.HomesteadBlock, newcfg.HomesteadBlock, head) {
		return newCompatError("Homestead fork block", c.HomesteadBlock, newcfg.HomesteadBlock)
	}
	if isForkIncompatible(c.DAOForkBlock, newcfg.DAOForkBlock, head) {
		return newCompatError("DAO fork block", c.DAOForkBlock, newcfg.DAOForkBlock)
	}
	if c.IsDAOFork(head) && c.DAOForkSupport != newcfg.DAOForkSupport {
		return newCompatError("DAO fork support flag", c.DAOForkBlock, newcfg.DAOForkBlock)
	}
	if isForkIncompatible(c.EIP150Block, newcfg.EIP150Block, head) {
		return newCompatError("EIP150 fork block", c.EIP150Block, newcfg.EIP150Block)
	}
	if isForkIncompatible(c.EIP155Block, newcfg.EIP155Block, head) {
		return newCompatError("EIP155 fork block", c.EIP155Block, newcfg.EIP155Block)
	}
	if isForkIncompatible(c.EIP158Block, newcfg.EIP158Block, head) {
		return newCompatError("EIP158 fork block", c.EIP158Block, newcfg.EIP158Block)
	}
	if c.IsEIP158(head) && !configNumEqual(c.ChainID, newcfg.ChainID) {
		return newCompatError("EIP158 chain ID", c.EIP158Block, newcfg.EIP158Block)
	}
	if isForkIncompatible(c.ByzantiumBlock, newcfg.ByzantiumBlock, head) {
		return newCompatError("Byzantium fork block", c.ByzantiumBlock, newcfg.ByzantiumBlock)
	}
	if isForkIncompatible(c.ConstantinopleBlock, newcfg.ConstantinopleBlock, head) {
		return newCompatError("Constantinople fork block", c.ConstantinopleBlock, newcfg.ConstantinopleBlock)
	}
	if isForkIncompatible(c.PetersburgBlock, newcfg.PetersburgBlock, head) {
		// the only case where we allow Petersburg to be set in the past is if it is equal to Constantinople
		// mainly to satisfy fork ordering requirements which state that Petersburg fork be set if Constantinople fork is set
		if isForkIncompatible(c.ConstantinopleBlock, newcfg.PetersburgBlock, head) {
			return newCompatError("Petersburg fork block", c.PetersburgBlock, newcfg.PetersburgBlock)
		}
	}
	if isForkIncompatible(c.IstanbulBlock, newcfg.IstanbulBlock, head) {
		return newCompatError("Istanbul fork block", c.IstanbulBlock, newcfg.IstanbulBlock)
	}

	return c.checkCeloCompatible(newcfg, head)
}

func (c *ChainConfig) checkCeloCompatible(newcfg *ChainConfig, head *big.Int) *ConfigCompatError {
	if isForkIncompatible(c.ChurritoBlock, newcfg.ChurritoBlock, head) {
		return newCompatError("Churrito fork block", c.ChurritoBlock, newcfg.ChurritoBlock)
	}
	if isForkIncompatible(c.DonutBlock, newcfg.DonutBlock, head) {
		return newCompatError("Donut fork block", c.DonutBlock, newcfg.DonutBlock)
	}
	if isForkIncompatible(c.EspressoBlock, newcfg.EspressoBlock, head) {
		return newCompatError("Espresso fork block", c.EspressoBlock, newcfg.EspressoBlock)
	}
	if isForkIncompatible(c.GForkBlock, newcfg.GForkBlock, head) {
		return newCompatError("G fork block", c.GForkBlock, newcfg.GForkBlock)
	}
	return nil
}

// isForkIncompatible returns true if a fork scheduled at s1 cannot be rescheduled to
// block s2 because head is already past the fork.
func isForkIncompatible(s1, s2, head *big.Int) bool {
	return (isForked(s1, head) || isForked(s2, head)) && !configNumEqual(s1, s2)
}

// isForked returns whether a fork scheduled at block s is active at the given head block.
func isForked(s, head *big.Int) bool {
	if s == nil || head == nil {
		return false
	}
	return s.Cmp(head) <= 0
}

func configNumEqual(x, y *big.Int) bool {
	if x == nil {
		return y == nil
	}
	if y == nil {
		return x == nil
	}
	return x.Cmp(y) == 0
}

// ConfigCompatError is raised if the locally-stored blockchain is initialised with a
// ChainConfig that would alter the past.
type ConfigCompatError struct {
	What string
	// block numbers of the stored and new configurations
	StoredConfig, NewConfig *big.Int
	// the block number to which the local chain must be rewound to correct the error
	RewindTo uint64
}

func newCompatError(what string, storedblock, newblock *big.Int) *ConfigCompatError {
	var rew *big.Int
	switch {
	case storedblock == nil:
		rew = newblock
	case newblock == nil || storedblock.Cmp(newblock) < 0:
		rew = storedblock
	default:
		rew = newblock
	}
	err := &ConfigCompatError{what, storedblock, newblock, 0}
	if rew != nil && rew.Sign() > 0 {
		err.RewindTo = rew.Uint64() - 1
	}
	return err
}

func (err *ConfigCompatError) Error() string {
	return fmt.Sprintf("mismatching %s in database (have %d, want %d, rewindto %d)", err.What, err.StoredConfig, err.NewConfig, err.RewindTo)
}

// Rules wraps ChainConfig and is merely syntactic sugar or can be used for functions
// that do not have or require information about the block.
//
// Rules is a one time interface meaning that it shouldn't be used in between transition
// phases.
type Rules struct {
	ChainID                                                 *big.Int
	IsHomestead, IsEIP150, IsEIP155, IsEIP158               bool
	IsByzantium, IsConstantinople, IsPetersburg, IsIstanbul bool
	// IsBerlin, IsLondon, IsCatalyst                          bool
	IsChurrito, IsDonut, IsEspresso, IsGFork bool
}

// Rules ensures c's ChainID is not nil.
func (c *ChainConfig) Rules(num *big.Int) Rules {
	chainID := c.ChainID
	if chainID == nil {
		chainID = new(big.Int)
	}
	return Rules{
		ChainID:          new(big.Int).Set(chainID),
		IsHomestead:      c.IsHomestead(num),
		IsEIP150:         c.IsEIP150(num),
		IsEIP155:         c.IsEIP155(num),
		IsEIP158:         c.IsEIP158(num),
		IsByzantium:      c.IsByzantium(num),
		IsConstantinople: c.IsConstantinople(num),
		IsPetersburg:     c.IsPetersburg(num),
		IsIstanbul:       c.IsIstanbul(num),
		// IsBerlin:         c.IsBerlin(num),
		// IsLondon:         c.IsLondon(num),
		// IsCatalyst:       c.IsCatalyst(num),
		IsChurrito: c.IsChurrito(num),
		IsDonut:    c.IsDonut(num),
		IsEspresso: c.IsEspresso(num),
		IsGFork:    c.IsGFork(num),
	}
}

func (c *ChainConfig) OverrideIstanbulConfig(istanbulConfig *IstanbulConfig) *ChainConfig {
	c.Istanbul = istanbulConfig
	return c
}

func (c *ChainConfig) OverrideFakerConfig(faker bool) *ChainConfig {
	c.Faker = faker
	return c
}

func (c *ChainConfig) OverrideChainIdConfig(chainId *big.Int) *ChainConfig {
	c.ChainID = chainId
	return c
}

func (c *ChainConfig) deepCopy() *ChainConfig {
	cpy := &ChainConfig{
		ChainID:             copyBigIntOrNil(c.ChainID),
		HomesteadBlock:      copyBigIntOrNil(c.HomesteadBlock),
		DAOForkBlock:        copyBigIntOrNil(c.DAOForkBlock),
		DAOForkSupport:      c.DAOForkSupport,
		EIP150Block:         copyBigIntOrNil(c.EIP150Block),
		EIP150Hash:          c.EIP150Hash,
		EIP155Block:         copyBigIntOrNil(c.EIP155Block),
		EIP158Block:         copyBigIntOrNil(c.EIP158Block),
		ByzantiumBlock:      copyBigIntOrNil(c.ByzantiumBlock),
		ConstantinopleBlock: copyBigIntOrNil(c.ConstantinopleBlock),
		PetersburgBlock:     copyBigIntOrNil(c.PetersburgBlock),
		IstanbulBlock:       copyBigIntOrNil(c.IstanbulBlock),
		ChurritoBlock:       copyBigIntOrNil(c.ChurritoBlock),
		DonutBlock:          copyBigIntOrNil(c.DonutBlock),
		EspressoBlock:       copyBigIntOrNil(c.EspressoBlock),
		GForkBlock:          copyBigIntOrNil(c.GForkBlock),

		Istanbul: &IstanbulConfig{
			Epoch:          c.Istanbul.Epoch,
			ProposerPolicy: c.Istanbul.ProposerPolicy,
			LookbackWindow: c.Istanbul.LookbackWindow,
			BlockPeriod:    c.Istanbul.BlockPeriod,
			RequestTimeout: c.Istanbul.RequestTimeout,
			// V2Block:        copyBigIntOrNil(c.Istanbul.V2Block),
		},

		FullHeaderChainAvailable: c.FullHeaderChainAvailable,
		Faker:                    c.Faker,
		FakeBaseFee:              c.FakeBaseFee,
	}
	return cpy
}

func copyBigIntOrNil(num *big.Int) *big.Int {
	if num == nil {
		return nil
	}
	return new(big.Int).Set(num)
}
