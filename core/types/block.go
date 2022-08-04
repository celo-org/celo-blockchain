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

// Package types contains data types related to Ethereum consensus.
package types

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/rlp"
)

var (
	EmptyRootHash       = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
	EmptyRandomness     = Randomness{}
	EmptyEpochSnarkData = EpochSnarkData{}
)

//go:generate gencodec -type Header -field-override headerMarshaling -out gen_header_json.go

// Header represents a block header in the Ethereum blockchain.
type Header struct {
	ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"`
	Coinbase    common.Address `json:"miner"            gencodec:"required"`
	Root        common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	Number      *big.Int       `json:"number"           gencodec:"required"`
	GasUsed     uint64         `json:"gasUsed"          gencodec:"required"`
	Time        uint64         `json:"timestamp"        gencodec:"required"`
	Extra       []byte         `json:"extraData"        gencodec:"required"`
}

// field type overrides for gencodec
type headerMarshaling struct {
	Number  *hexutil.Big
	GasUsed hexutil.Uint64
	Time    hexutil.Uint64
	Extra   hexutil.Bytes
	Hash    common.Hash `json:"hash"` // adds call to Hash() in MarshalJSON
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *Header) Hash() common.Hash {
	// Seal is reserved in extra-data. To prove block is signed by the proposer.
	if len(h.Extra) >= IstanbulExtraVanity {
		if istanbulHeader := IstanbulFilteredHeader(h, true); istanbulHeader != nil {
			return rlpHash(istanbulHeader)
		}
	}
	return rlpHash(h)
}

var headerSize = common.StorageSize(reflect.TypeOf(Header{}).Size())

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *Header) Size() common.StorageSize {
	return headerSize + common.StorageSize(len(h.Extra)+(h.Number.BitLen()/8))
}

// SanityCheck checks a few basic things -- these checks are way beyond what
// any 'sane' production values should hold, and can mainly be used to prevent
// that the unbounded fields are stuffed with junk data to add processing
// overhead
func (h *Header) SanityCheck() error {
	if h.Number != nil && !h.Number.IsUint64() {
		return fmt.Errorf("too large block number: bitlen %d", h.Number.BitLen())
	}
	if eLen := len(h.Extra); eLen > 100*1024 {
		return fmt.Errorf("too large block extradata: size %d", eLen)
	}
	return nil
}

// EmptyBody returns true if there is no additional 'body' to complete the header
// that is: no transactions.
func (h *Header) EmptyBody() bool {
	return h.TxHash == EmptyRootHash
}

// EmptyReceipts returns true if there are no receipts for this header/block.
func (h *Header) EmptyReceipts() bool {
	return h.ReceiptHash == EmptyRootHash
}

type Randomness struct {
	Revealed  common.Hash
	Committed common.Hash
}

func (r *Randomness) Size() common.StorageSize {
	return common.StorageSize(64)
}

func (r *Randomness) DecodeRLP(s *rlp.Stream) error {
	var random struct {
		Revealed  common.Hash
		Committed common.Hash
	}
	if err := s.Decode(&random); err != nil {
		return err
	}
	r.Revealed, r.Committed = random.Revealed, random.Committed
	return nil
}

func (r *Randomness) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{r.Revealed, r.Committed})
}

type EpochSnarkData struct {
	Bitmap    *big.Int
	Signature []byte
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (r *EpochSnarkData) Size() common.StorageSize {
	return common.StorageSize(blscrypto.SIGNATUREBYTES + (r.Bitmap.BitLen() / 8))
}

func (r *EpochSnarkData) DecodeRLP(s *rlp.Stream) error {
	var epochSnarkData struct {
		Bitmap    *big.Int
		Signature []byte
	}
	if err := s.Decode(&epochSnarkData); err != nil {
		return err
	}
	r.Bitmap = epochSnarkData.Bitmap
	r.Signature = epochSnarkData.Signature
	return nil
}

func (r *EpochSnarkData) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{r.Bitmap, r.Signature})
}

func (r *EpochSnarkData) IsEmpty() bool {
	return len(r.Signature) == 0
}

// MarshalJSON marshals as JSON.
func (r EpochSnarkData) MarshalJSON() ([]byte, error) {
	type EpochSnarkData struct {
		Bitmap    hexutil.Bytes
		Signature hexutil.Bytes
	}
	var enc EpochSnarkData
	enc.Bitmap = r.Bitmap.Bytes()
	enc.Signature = r.Signature
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (r *EpochSnarkData) UnmarshalJSON(input []byte) error {
	type EpochSnarkData struct {
		Bitmap    hexutil.Bytes
		Signature hexutil.Bytes
	}
	var dec EpochSnarkData
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	r.Bitmap = new(big.Int).SetBytes(dec.Bitmap)
	r.Signature = dec.Signature
	return nil
}

// Body is a simple (mutable, non-safe) data container for storing and moving
// a block's data contents (transactions and uncles) together.
type Body struct {
	Transactions   []*Transaction
	Randomness     *Randomness
	EpochSnarkData *EpochSnarkData
}

// Block represents an entire block in the Ethereum blockchain.
type Block struct {
	header         *Header
	transactions   Transactions
	randomness     *Randomness
	epochSnarkData *EpochSnarkData

	// caches
	hash atomic.Value
	size atomic.Value

	// Td is used by package core to store the total difficulty
	// of the chain up to and including the block.
	td *big.Int

	// These fields are used by package eth to track
	// inter-peer block relay.
	ReceivedAt   time.Time
	ReceivedFrom interface{}
}

// "external" block encoding. used for eth protocol, etc.
type extblock struct {
	Header         *Header
	Txs            []*Transaction
	Randomness     *Randomness
	EpochSnarkData *EpochSnarkData
}

// NewBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs and receipts.
func NewBlock(header *Header, txs []*Transaction, receipts []*Receipt, randomness *Randomness, hasher TrieHasher) *Block {
	b := &Block{header: CopyHeader(header), td: new(big.Int), randomness: randomness, epochSnarkData: &EmptyEpochSnarkData}

	// TODO: panic if len(txs) != len(receipts)
	if len(txs) == 0 {
		b.header.TxHash = EmptyRootHash
	} else {
		b.header.TxHash = DeriveSha(Transactions(txs), hasher)
		b.transactions = make(Transactions, len(txs))
		copy(b.transactions, txs)
	}

	if len(receipts) == 0 {
		b.header.ReceiptHash = EmptyRootHash
	} else {
		b.header.ReceiptHash = DeriveSha(Receipts(receipts), hasher)
		b.header.Bloom = CreateBloom(receipts)
	}

	if randomness == nil {
		b.randomness = &EmptyRandomness
	}

	return b
}

// NewBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewBlockWithHeader(header *Header) *Block {
	return &Block{header: CopyHeader(header), randomness: &EmptyRandomness, epochSnarkData: &EmptyEpochSnarkData}
}

// CopyHeader creates a deep copy of a block header to prevent side effects from
// modifying a header variable.
func CopyHeader(h *Header) *Header {
	cpy := *h
	if cpy.Number = new(big.Int); h.Number != nil {
		cpy.Number.Set(h.Number)
	}
	if len(h.Extra) > 0 {
		cpy.Extra = make([]byte, len(h.Extra))
		copy(cpy.Extra, h.Extra)
	}
	return &cpy
}

// DecodeRLP decodes the Ethereum
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	var eb extblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.transactions, b.randomness, b.epochSnarkData = eb.Header, eb.Txs, eb.Randomness, eb.EpochSnarkData
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

// EncodeRLP serializes b into the Ethereum RLP block format.
func (b *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extblock{
		Header:         b.header,
		Txs:            b.transactions,
		Randomness:     b.randomness,
		EpochSnarkData: b.epochSnarkData,
	})
}

// TODO: copies

func (b *Block) Transactions() Transactions      { return b.transactions }
func (b *Block) Randomness() *Randomness         { return b.randomness }
func (b *Block) EpochSnarkData() *EpochSnarkData { return b.epochSnarkData }

func (b *Block) Transaction(hash common.Hash) *Transaction {
	for _, transaction := range b.transactions {
		if transaction.Hash() == hash {
			return transaction
		}
	}
	return nil
}

func (b *Block) Number() *big.Int          { return new(big.Int).Set(b.header.Number) }
func (b *Block) GasUsed() uint64           { return b.header.GasUsed }
func (b *Block) Time() uint64              { return b.header.Time }
func (b *Block) TotalDifficulty() *big.Int { return new(big.Int).Add(b.header.Number, big.NewInt(1)) }
func (b *Block) NumberU64() uint64         { return b.header.Number.Uint64() }
func (b *Block) Bloom() Bloom              { return b.header.Bloom }
func (b *Block) Coinbase() common.Address  { return b.header.Coinbase }
func (b *Block) Root() common.Hash         { return b.header.Root }
func (b *Block) ParentHash() common.Hash   { return b.header.ParentHash }
func (b *Block) TxHash() common.Hash       { return b.header.TxHash }
func (b *Block) ReceiptHash() common.Hash  { return b.header.ReceiptHash }
func (b *Block) Extra() []byte             { return common.CopyBytes(b.header.Extra) }

func (b *Block) Header() *Header        { return CopyHeader(b.header) }
func (b *Block) MutableHeader() *Header { return b.header }

// Body returns the non-header content of the block.
func (b *Block) Body() *Body { return &Body{b.transactions, b.randomness, b.epochSnarkData} }

// Size returns the true RLP encoded storage size of the block, either by encoding
// and returning it, or returning a previsouly cached value.
func (b *Block) Size() common.StorageSize {
	if size := b.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	rlp.Encode(&c, b)
	b.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}

// SanityCheck can be used to prevent that unbounded fields are
// stuffed with junk data to add processing overhead
func (b *Block) SanityCheck() error {
	return b.header.SanityCheck()
}

type writeCounter common.StorageSize

func (c *writeCounter) Write(b []byte) (int, error) {
	*c += writeCounter(len(b))
	return len(b), nil
}

// WithHeader returns a new block with the data from b but the header replaced with
// the sealed one.
func (b *Block) WithHeader(header *Header) *Block {
	cpy := *header

	return &Block{
		header:         &cpy,
		transactions:   b.transactions,
		randomness:     b.randomness,
		epochSnarkData: b.epochSnarkData,
	}
}

// WithSeal returns a new block with the data from b but the header replaced with
// the sealed one.
func (b *Block) WithSeal(header *Header) *Block {
	cpy := *header

	return &Block{
		header:         &cpy,
		transactions:   b.transactions,
		randomness:     b.randomness,
		epochSnarkData: b.epochSnarkData,
	}
}

// WithRandomness returns a new block with the given randomness.
func (b *Block) WithRandomness(randomness *Randomness) *Block {
	block := &Block{
		header:         b.header,
		transactions:   b.transactions,
		randomness:     randomness,
		epochSnarkData: b.epochSnarkData,
	}
	return block
}

// WithEpochSnarkData returns a new block with the given epoch SNARK data.
func (b *Block) WithEpochSnarkData(epochSnarkData *EpochSnarkData) *Block {
	block := &Block{
		header:         b.header,
		transactions:   b.transactions,
		randomness:     b.randomness,
		epochSnarkData: epochSnarkData,
	}
	return block
}

// WithBody returns a new block with the given transaction contents.
func (b *Block) WithBody(transactions []*Transaction, randomness *Randomness, epochSnarkData *EpochSnarkData) *Block {
	block := &Block{
		header:         CopyHeader(b.header),
		transactions:   make([]*Transaction, len(transactions)),
		randomness:     randomness,
		epochSnarkData: epochSnarkData,
	}
	copy(block.transactions, transactions)
	if randomness == nil {
		block.randomness = &EmptyRandomness
	}
	if epochSnarkData == nil {
		block.epochSnarkData = &EmptyEpochSnarkData
	}
	return block
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *Block) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.header.Hash()
	b.hash.Store(v)
	return v
}

type Blocks []*Block
