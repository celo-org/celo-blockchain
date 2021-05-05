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

package types

import (
	"container/heap"
	"errors"
	"io"
	"math/big"
	"sync/atomic"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/common/math"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/rlp"
)

//go:generate gencodec -type txdata -field-override txdataMarshaling -out gen_tx_json.go

const (
	ethCompatibleTxNumFields = 9
)

var (
	ErrInvalidSig = errors.New("invalid transaction v, r, s values")
	// ErrEthCompatibleTransactionIsntCompatible is returned if the transaction has EthCompatible: true
	// but has non-nil-or-0 values for some of the Celo-only fields
	ErrEthCompatibleTransactionIsntCompatible = errors.New("ethCompatible is true, but non-eth-compatible fields are present")
)

type Transaction struct {
	data txdata
	// caches
	hash atomic.Value
	size atomic.Value
	from atomic.Value
}

type txdata struct {
	AccountNonce        uint64          `json:"nonce"    gencodec:"required"`
	Price               *big.Int        `json:"gasPrice" gencodec:"required"`
	GasLimit            uint64          `json:"gas"      gencodec:"required"`
	FeeCurrency         *common.Address `json:"feeCurrency" rlp:"nil"`         // nil means native currency
	GatewayFeeRecipient *common.Address `json:"gatewayFeeRecipient" rlp:"nil"` // nil means no gateway fee is paid
	GatewayFee          *big.Int        `json:"gatewayFee"`
	Recipient           *common.Address `json:"to"       rlp:"nil"` // nil means contract creation
	Amount              *big.Int        `json:"value"    gencodec:"required"`
	Payload             []byte          `json:"input"    gencodec:"required"`

	// Signature values
	V *big.Int `json:"v" gencodec:"required"`
	R *big.Int `json:"r" gencodec:"required"`
	S *big.Int `json:"s" gencodec:"required"`

	// This is only used when marshaling to JSON.
	Hash *common.Hash `json:"hash" rlp:"-"`

	// Whether this is an ethereum-compatible transaction (i.e. with FeeCurrency, GatewayFeeRecipient and GatewayFee omitted)
	EthCompatible bool `json:"ethCompatible" rlp:"-"`
}

type txdataMarshaling struct {
	AccountNonce        hexutil.Uint64
	Price               *hexutil.Big
	GasLimit            hexutil.Uint64
	FeeCurrency         *common.Address
	GatewayFeeRecipient *common.Address
	GatewayFee          *hexutil.Big
	Amount              *hexutil.Big
	Payload             hexutil.Bytes
	V                   *hexutil.Big
	R                   *hexutil.Big
	S                   *hexutil.Big
	EthCompatible       bool
}

// ethCompatibleTxRlpList is used for RLP encoding/decoding of eth-compatible transactions.
// As such, it:
// (a) excludes the Celo-only fields,
// (b) doesn't need the Hash or EthCompatible fields, and
// (c) doesn't need the `json` or `gencodec` tags
type ethCompatibleTxRlpList struct {
	AccountNonce uint64
	Price        *big.Int
	GasLimit     uint64
	Recipient    *common.Address `rlp:"nil"` // nil means contract creation
	Amount       *big.Int
	Payload      []byte
	V            *big.Int
	R            *big.Int
	S            *big.Int
}

func toEthCompatibleRlpList(data txdata) ethCompatibleTxRlpList {
	return ethCompatibleTxRlpList{
		AccountNonce: data.AccountNonce,
		Price:        data.Price,
		GasLimit:     data.GasLimit,
		Recipient:    data.Recipient,
		Amount:       data.Amount,
		Payload:      data.Payload,
		V:            data.V,
		R:            data.R,
		S:            data.S,
	}
}

func fromEthCompatibleRlpList(data ethCompatibleTxRlpList) txdata {
	return txdata{
		AccountNonce:        data.AccountNonce,
		Price:               data.Price,
		GasLimit:            data.GasLimit,
		FeeCurrency:         nil,
		GatewayFeeRecipient: nil,
		GatewayFee:          big.NewInt(0),
		Recipient:           data.Recipient,
		Amount:              data.Amount,
		Payload:             data.Payload,
		V:                   data.V,
		R:                   data.R,
		S:                   data.S,
		Hash:                nil, // txdata.Hash is calculated and saved inside tx.Hash()
		EthCompatible:       true,
	}
}

func NewTransaction(nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, feeCurrency, gatewayFeeRecipient *common.Address, gatewayFee *big.Int, data []byte) *Transaction {
	return newTransaction(nonce, &to, amount, gasLimit, gasPrice, feeCurrency, gatewayFeeRecipient, gatewayFee, data)
}

func NewTransactionEthCompatible(nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte) *Transaction {
	tx := newTransaction(nonce, &to, amount, gasLimit, gasPrice, nil, nil, nil, data)
	tx.data.EthCompatible = true
	return tx
}

func NewContractCreation(nonce uint64, amount *big.Int, gasLimit uint64, gasPrice *big.Int, feeCurrency, gatewayFeeRecipient *common.Address, gatewayFee *big.Int, data []byte) *Transaction {
	return newTransaction(nonce, nil, amount, gasLimit, gasPrice, feeCurrency, gatewayFeeRecipient, gatewayFee, data)
}

func NewContractCreationEthCompatible(nonce uint64, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte) *Transaction {
	tx := newTransaction(nonce, nil, amount, gasLimit, gasPrice, nil, nil, nil, data)
	tx.data.EthCompatible = true
	return tx
}

func newTransaction(nonce uint64, to *common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, feeCurrency, gatewayFeeRecipient *common.Address, gatewayFee *big.Int, data []byte) *Transaction {
	if len(data) > 0 {
		data = common.CopyBytes(data)
	}
	d := txdata{
		AccountNonce:        nonce,
		Recipient:           to,
		Payload:             data,
		Amount:              new(big.Int),
		GasLimit:            gasLimit,
		FeeCurrency:         feeCurrency,
		GatewayFeeRecipient: gatewayFeeRecipient,
		GatewayFee:          new(big.Int),
		Price:               new(big.Int),
		V:                   new(big.Int),
		R:                   new(big.Int),
		S:                   new(big.Int),
	}
	if amount != nil {
		d.Amount.Set(amount)
	}
	if gatewayFee != nil {
		d.GatewayFee.Set(gatewayFee)
	}
	if gasPrice != nil {
		d.Price.Set(gasPrice)
	}

	return &Transaction{data: d}
}

// ChainId returns which chain id this transaction was signed for (if at all)
func (tx *Transaction) ChainId() *big.Int {
	return deriveChainId(tx.data.V)
}

// Protected returns whether the transaction is protected from replay protection.
func (tx *Transaction) Protected() bool {
	return isProtectedV(tx.data.V)
}

func isProtectedV(V *big.Int) bool {
	if V.BitLen() <= 8 {
		v := V.Uint64()
		return v != 27 && v != 28
	}
	// anything not 27 or 28 is considered protected
	return true
}

// EncodeRLP implements rlp.Encoder
func (tx *Transaction) EncodeRLP(w io.Writer) error {
	if tx.data.EthCompatible {
		return rlp.Encode(w, toEthCompatibleRlpList(tx.data))
	} else {
		return rlp.Encode(w, &tx.data)
	}
}

// DecodeRLP implements rlp.Decoder
func (tx *Transaction) DecodeRLP(s *rlp.Stream) (err error) {
	_, size, _ := s.Kind()
	var raw rlp.RawValue
	err = s.Decode(&raw)
	if err != nil {
		return err
	}
	headerSize := len(raw) - int(size)
	numElems, err := rlp.CountValues(raw[headerSize:])
	if err != nil {
		return err
	}
	if numElems == ethCompatibleTxNumFields {
		rlpList := ethCompatibleTxRlpList{}
		err = rlp.DecodeBytes(raw, &rlpList)
		tx.data = fromEthCompatibleRlpList(rlpList)
	} else {
		err = rlp.DecodeBytes(raw, &tx.data)
	}
	if err == nil {
		tx.size.Store(common.StorageSize(rlp.ListSize(size)))
	}

	return err
}

// MarshalJSON encodes the web3 RPC transaction format.
func (tx *Transaction) MarshalJSON() ([]byte, error) {
	hash := tx.Hash()
	data := tx.data
	data.Hash = &hash
	return data.MarshalJSON()
}

// UnmarshalJSON decodes the web3 RPC transaction format.
func (tx *Transaction) UnmarshalJSON(input []byte) error {
	var dec txdata
	if err := dec.UnmarshalJSON(input); err != nil {
		return err
	}

	withSignature := dec.V.Sign() != 0 || dec.R.Sign() != 0 || dec.S.Sign() != 0
	if withSignature {
		var V byte
		if isProtectedV(dec.V) {
			chainID := deriveChainId(dec.V).Uint64()
			V = byte(dec.V.Uint64() - 35 - 2*chainID)
		} else {
			V = byte(dec.V.Uint64() - 27)
		}
		if !crypto.ValidateSignatureValues(V, dec.R, dec.S, false) {
			return ErrInvalidSig
		}
	}

	*tx = Transaction{data: dec}
	return nil
}

func (tx *Transaction) Data() []byte       { return common.CopyBytes(tx.data.Payload) }
func (tx *Transaction) Gas() uint64        { return tx.data.GasLimit }
func (tx *Transaction) GasPrice() *big.Int { return new(big.Int).Set(tx.data.Price) }
func (tx *Transaction) GasPriceCmp(other *Transaction) int {
	return tx.data.Price.Cmp(other.data.Price)
}
func (tx *Transaction) GasPriceIntCmp(other *big.Int) int {
	return tx.data.Price.Cmp(other)
}
func (tx *Transaction) FeeCurrency() *common.Address         { return tx.data.FeeCurrency }
func (tx *Transaction) GatewayFeeRecipient() *common.Address { return tx.data.GatewayFeeRecipient }
func (tx *Transaction) GatewayFee() *big.Int                 { return tx.data.GatewayFee }
func (tx *Transaction) Value() *big.Int                      { return new(big.Int).Set(tx.data.Amount) }
func (tx *Transaction) ValueU64() uint64                     { return tx.data.Amount.Uint64() }
func (tx *Transaction) Nonce() uint64                        { return tx.data.AccountNonce }
func (tx *Transaction) CheckNonce() bool                     { return true }
func (tx *Transaction) EthCompatible() bool                  { return tx.data.EthCompatible }
func (tx *Transaction) Fee() *big.Int {
	gasFee := new(big.Int).Mul(tx.data.Price, big.NewInt(int64(tx.data.GasLimit)))
	return gasFee.Add(gasFee, tx.data.GatewayFee)
}

// To returns the recipient address of the transaction.
// It returns nil if the transaction is a contract creation.
func (tx *Transaction) To() *common.Address {
	if tx.data.Recipient == nil {
		return nil
	}
	to := *tx.data.Recipient
	return &to
}

// Hash hashes the RLP encoding of tx.
// It uniquely identifies the transaction.
func (tx *Transaction) Hash() common.Hash {
	if hash := tx.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := rlpHash(tx)
	tx.hash.Store(v)
	return v
}

// Size returns the true RLP encoded storage size of the transaction, either by
// encoding and returning it, or returning a previsouly cached value.
func (tx *Transaction) Size() common.StorageSize {
	if size := tx.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	rlp.Encode(&c, &tx.data)
	tx.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}

// CheckEthCompatibility checks that the Celo-only fields are nil-or-0 if EthCompatible is true
func (tx *Transaction) CheckEthCompatibility() error {
	if tx.EthCompatible() && !(tx.FeeCurrency() == nil && tx.GatewayFeeRecipient() == nil && tx.GatewayFee().Sign() == 0) {
		return ErrEthCompatibleTransactionIsntCompatible
	}
	return nil
}

// AsMessage returns the transaction as a core.Message.
//
// AsMessage requires a signer to derive the sender.
//
// XXX Rename message to something less arbitrary?
func (tx *Transaction) AsMessage(s Signer) (Message, error) {
	msg := Message{
		nonce:               tx.data.AccountNonce,
		gasLimit:            tx.data.GasLimit,
		gasPrice:            new(big.Int).Set(tx.data.Price),
		feeCurrency:         tx.data.FeeCurrency,
		gatewayFeeRecipient: tx.data.GatewayFeeRecipient,
		gatewayFee:          tx.data.GatewayFee,
		to:                  tx.data.Recipient,
		amount:              tx.data.Amount,
		data:                tx.data.Payload,
		checkNonce:          true,
		ethCompatible:       tx.data.EthCompatible,
	}

	var err error
	msg.from, err = Sender(s, tx)
	return msg, err
}

// WithSignature returns a new transaction with the given signature.
// This signature needs to be in the [R || S || V] format where V is 0 or 1.
func (tx *Transaction) WithSignature(signer Signer, sig []byte) (*Transaction, error) {
	r, s, v, err := signer.SignatureValues(tx, sig)
	if err != nil {
		return nil, err
	}
	cpy := &Transaction{data: tx.data}
	cpy.data.R, cpy.data.S, cpy.data.V = r, s, v
	return cpy, nil
}

// Cost returns amount + gasprice * gaslimit + gatewayfee.
func (tx *Transaction) Cost() *big.Int {
	total := new(big.Int).Mul(tx.data.Price, new(big.Int).SetUint64(tx.data.GasLimit))
	total.Add(total, tx.data.Amount)
	total.Add(total, tx.data.GatewayFee)
	return total
}

func (tx *Transaction) CostU64() (uint64, bool) {
	if tx.data.Price.BitLen() > 63 || tx.data.Amount.BitLen() > 63 {
		return 0, false
	}
	cost, overflowMul := math.SafeMul(tx.data.Price.Uint64(), tx.data.GasLimit)
	total, overflowAdd := math.SafeAdd(cost, tx.data.Amount.Uint64())
	return total, overflowMul || overflowAdd
}

// RawSignatureValues returns the V, R, S signature values of the transaction.
// The return values should not be modified by the caller.
func (tx *Transaction) RawSignatureValues() (v, r, s *big.Int) {
	return tx.data.V, tx.data.R, tx.data.S
}

// Transactions is a Transaction slice type for basic sorting.
type Transactions []*Transaction

// Len returns the length of s.
func (s Transactions) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s.
func (s Transactions) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// GetRlp implements Rlpable and returns the i'th element of s in rlp.
func (s Transactions) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}

// TxDifference returns a new set which is the difference between a and b.
func TxDifference(a, b Transactions) Transactions {
	keep := make(Transactions, 0, len(a))

	remove := make(map[common.Hash]struct{})
	for _, tx := range b {
		remove[tx.Hash()] = struct{}{}
	}

	for _, tx := range a {
		if _, ok := remove[tx.Hash()]; !ok {
			keep = append(keep, tx)
		}
	}

	return keep
}

// TxByNonce implements the sort interface to allow sorting a list of transactions
// by their nonces. This is usually only useful for sorting transactions from a
// single account, otherwise a nonce comparison doesn't make much sense.
type TxByNonce Transactions

func (s TxByNonce) Len() int           { return len(s) }
func (s TxByNonce) Less(i, j int) bool { return s[i].data.AccountNonce < s[j].data.AccountNonce }
func (s TxByNonce) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// TxByPrice implements both the sort and the heap interface, making it useful
// for all at once sorting as well as individually adding and removing elements.
type TxByPrice struct {
	txs       Transactions
	txCmpFunc func(tx1, tx2 *Transaction) int
}

func (s TxByPrice) Len() int           { return len(s.txs) }
func (s TxByPrice) Less(i, j int) bool { return s.txCmpFunc(s.txs[i], s.txs[j]) > 0 }
func (s TxByPrice) Swap(i, j int)      { s.txs[i], s.txs[j] = s.txs[j], s.txs[i] }

func (s *TxByPrice) Push(x interface{}) {
	s.txs = append(s.txs, x.(*Transaction))
}

func (s *TxByPrice) Pop() interface{} {
	old := s.txs
	n := len(old)
	x := old[n-1]
	s.txs = old[0 : n-1]
	return x
}

func (s *TxByPrice) Peek() *Transaction {
	return s.txs[0]
}

func (s *TxByPrice) Add(tx *Transaction) {
	s.txs[0] = tx
}

// TransactionsByPriceAndNonce represents a set of transactions that can return
// transactions in a profit-maximizing sorted order, while supporting removing
// entire batches of transactions for non-executable accounts.
type TransactionsByPriceAndNonce struct {
	txs    map[common.Address]Transactions // Per account nonce-sorted list of transactions
	heads  TxByPrice                       // Next transaction for each unique account (price heap)
	signer Signer                          // Signer for the set of transactions
}

// NewTransactionsByPriceAndNonce creates a transaction set that can retrieve
// price sorted transactions in a nonce-honouring way.
//
// Note, the input map is reowned so the caller should not interact any more with
// if after providing it to the constructor.
func NewTransactionsByPriceAndNonce(signer Signer, txs map[common.Address]Transactions, txCmpFunc func(tx1, tx2 *Transaction) int) *TransactionsByPriceAndNonce {
	// Initialize a price based heap with the head transactions
	heads := TxByPrice{
		txs:       make(Transactions, 0, len(txs)),
		txCmpFunc: txCmpFunc,
	}
	for from, accTxs := range txs {
		heads.Push(accTxs[0])
		// Ensure the sender address is from the signer
		acc, _ := Sender(signer, accTxs[0])
		txs[acc] = accTxs[1:]
		if from != acc {
			delete(txs, from)
		}
	}
	heap.Init(&heads)

	// Assemble and return the transaction set
	return &TransactionsByPriceAndNonce{
		txs:    txs,
		heads:  heads,
		signer: signer,
	}
}

// Peek returns the next transaction by price.
func (t *TransactionsByPriceAndNonce) Peek() *Transaction {
	if t.heads.Len() == 0 {
		return nil
	}
	return t.heads.Peek()
}

// Shift replaces the current best head with the next one from the same account.
func (t *TransactionsByPriceAndNonce) Shift() {
	acc, _ := Sender(t.signer, t.heads.Peek())
	if txs, ok := t.txs[acc]; ok && len(txs) > 0 {
		next := txs[0]
		t.txs[acc] = txs[1:]
		t.heads.Add(next)
		heap.Fix(&t.heads, 0)
	} else {
		heap.Pop(&t.heads)
	}
}

// Pop removes the best transaction, *not* replacing it with the next one from
// the same account. This should be used when a transaction cannot be executed
// and hence all subsequent ones should be discarded from the same account.
func (t *TransactionsByPriceAndNonce) Pop() {
	heap.Pop(&t.heads)
}

// Message is a fully derived transaction and implements core.Message
//
// NOTE: In a future PR this will be removed.
type Message struct {
	to                  *common.Address
	from                common.Address
	nonce               uint64
	amount              *big.Int
	gasLimit            uint64
	gasPrice            *big.Int
	feeCurrency         *common.Address
	gatewayFeeRecipient *common.Address
	gatewayFee          *big.Int
	data                []byte
	ethCompatible       bool
	checkNonce          bool
}

func NewMessage(from common.Address, to *common.Address, nonce uint64, amount *big.Int, gasLimit uint64, gasPrice *big.Int, feeCurrency, gatewayFeeRecipient *common.Address, gatewayFee *big.Int, data []byte, ethCompatible, checkNonce bool) Message {
	return Message{
		from:                from,
		to:                  to,
		nonce:               nonce,
		amount:              amount,
		gasLimit:            gasLimit,
		gasPrice:            gasPrice,
		feeCurrency:         feeCurrency,
		gatewayFeeRecipient: gatewayFeeRecipient,
		gatewayFee:          gatewayFee,
		data:                data,
		ethCompatible:       ethCompatible,
		checkNonce:          checkNonce,
	}
}

func (m Message) From() common.Address                 { return m.from }
func (m Message) To() *common.Address                  { return m.to }
func (m Message) GasPrice() *big.Int                   { return m.gasPrice }
func (m Message) EthCompatible() bool                  { return m.ethCompatible }
func (m Message) FeeCurrency() *common.Address         { return m.feeCurrency }
func (m Message) GatewayFeeRecipient() *common.Address { return m.gatewayFeeRecipient }
func (m Message) GatewayFee() *big.Int                 { return m.gatewayFee }
func (m Message) Value() *big.Int                      { return m.amount }
func (m Message) Gas() uint64                          { return m.gasLimit }
func (m Message) Nonce() uint64                        { return m.nonce }
func (m Message) Data() []byte                         { return m.data }
func (m Message) CheckNonce() bool                     { return m.checkNonce }
func (m Message) Fee() *big.Int {
	gasFee := new(big.Int).Mul(m.gasPrice, big.NewInt(int64(m.gasLimit)))
	return gasFee.Add(gasFee, m.gatewayFee)
}
