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
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/rlp"
)

// The values in those tests are from the Transaction Tests
// at github.com/ethereum/tests.
var (
	emptyTx = NewTransaction(
		0,
		common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
		big.NewInt(0), 0, big.NewInt(0),
		nil,
		nil,
		nil,
		nil,
	)

	rightvrsTx, _ = NewTransaction(
		3,
		common.HexToAddress("b94f5374fce5edbc8e2a8697c15331677e6ebf0b"),
		big.NewInt(10),
		2000,
		big.NewInt(1),
		nil,
		nil,
		nil,
		common.FromHex("5544"),
	).WithSignature(
		HomesteadSigner{},
		common.Hex2Bytes("98ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4a8887321be575c8095f789dd4c743dfe42c1820f9231f98a962b210e3ac2452a301"),
	)
)

func TestTransactionSigHash(t *testing.T) {
	var homestead HomesteadSigner
	if homestead.Hash(emptyTx) != common.HexToHash("0884127f4e682c55978e9e8a4cecc734bf3fa14776a3c2b28adc16855cb3a491") {
		t.Errorf("empty transaction hash mismatch, got %x", homestead.Hash(emptyTx))
	}
	if homestead.Hash(rightvrsTx) != common.HexToHash("b879218b07bdecfa13fbfcca8ea298aab32d06e5ecd7b021a51b53c5f8919209") {
		t.Errorf("RightVRS transaction hash mismatch, got %x", homestead.Hash(rightvrsTx))
	}
}

func TestTransactionEncode(t *testing.T) {
	txb, err := rlp.EncodeToBytes(rightvrsTx)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	should := common.FromHex("f86403018207d080808094b94f5374fce5edbc8e2a8697c15331677e6ebf0b0a8255441ca098ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4aa08887321be575c8095f789dd4c743dfe42c1820f9231f98a962b210e3ac2452a3")
	if !bytes.Equal(txb, should) {
		t.Errorf("encoded RLP mismatch, got %x", txb)
	}
}

func decodeTx(data []byte) (*Transaction, error) {
	var tx Transaction
	t, err := &tx, rlp.Decode(bytes.NewReader(data), &tx)

	return t, err
}

func defaultTestKey() (*ecdsa.PrivateKey, common.Address) {
	key, _ := crypto.HexToECDSA("45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8")
	addr := crypto.PubkeyToAddress(key.PublicKey)
	return key, addr
}

func signAndEncodeTx(tx *Transaction) []byte {
	key, _ := defaultTestKey()

	signer := HomesteadSigner{}
	tx, _ = SignTx(tx, signer, key)

	buf := bytes.NewBuffer([]byte{})
	tx.EncodeRLP(buf)
	byteArray := make([]byte, buf.Len())
	buf.Read(byteArray)
	return byteArray
}

func TestRecipientEmpty(t *testing.T) {
	_, addr := defaultTestKey()
	tx := emptyTx
	tx.data.Recipient = nil
	tx, err := decodeTx(signAndEncodeTx(tx))
	if err != nil {
		t.Fatal(err)
	}

	from, err := Sender(HomesteadSigner{}, tx)
	if err != nil {
		t.Fatal(err)
	}
	if addr != from {
		t.Fatal("derived address doesn't match")
	}
}

func TestRecipientNormal(t *testing.T) {
	_, addr := defaultTestKey()

	tx, err := decodeTx(signAndEncodeTx(rightvrsTx))
	if err != nil {
		t.Fatal(err)
	}

	from, err := Sender(HomesteadSigner{}, tx)
	if err != nil {
		t.Fatal(err)
	}
	if addr != from {
		t.Fatal("derived address doesn't match")
	}
}

// Tests that a modified transaction does not produce a valid signature
func TestTxAmountChanged(t *testing.T) {
	_, addr := defaultTestKey()

	tx, err := decodeTx(signAndEncodeTx(rightvrsTx))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	tx.data.Amount = big.NewInt(20)

	from, err := Sender(HomesteadSigner{}, tx)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if addr == from {
		t.Error("derived address shouldn't match")
	}
}

func TestTxGatewayFeeRecipientChanged(t *testing.T) {
	_, addr := defaultTestKey()

	tx, err := decodeTx(signAndEncodeTx(rightvrsTx))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	recipientAddr := common.HexToAddress("b94f5374fce5edbc8e2a8697c15331677e6ebf0b")
	tx.data.GatewayFeeRecipient = &recipientAddr

	from, err := Sender(HomesteadSigner{}, tx)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if addr == from {
		t.Error("derived address shouldn't match")
	}
}

func TestTxGatewayFee(t *testing.T) {
	_, addr := defaultTestKey()

	tx, err := decodeTx(signAndEncodeTx(rightvrsTx))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	tx.data.GatewayFee.SetInt64(5)

	from, err := Sender(HomesteadSigner{}, tx)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if addr == from {
		t.Error("derived address shouldn't match")
	}
}

func TestTxEthCompatible(t *testing.T) {
	key, addr := defaultTestKey()
	tx := NewTransactionEthCompatible(
		3,
		common.Address{19},
		big.NewInt(9),
		7,
		big.NewInt(13),
		common.FromHex("ff05ff"),
	)

	var encoded []byte
	var parsed *Transaction

	encoded, _ = rlp.EncodeToBytes(tx)
	parsed = &Transaction{}
	rlp.DecodeBytes(encoded, &parsed)
	if tx.Hash() != parsed.Hash() {
		t.Errorf("RLP parsed pre-signing tx differs from original, want %v, got %v", tx, parsed)
	}
	encoded, _ = tx.MarshalJSON()
	parsed = &Transaction{}
	parsed.UnmarshalJSON(encoded)
	if tx.Hash() != parsed.Hash() {
		t.Errorf("JSON parsed pre-signing tx differs from original, want %v, got %v", tx, parsed)
	}

	// Repeat the tests but now with a signed transaction
	signer := NewEIP155Signer(common.Big1)
	signed, _ := SignTx(tx, signer, key)
	sender, _ := Sender(signer, signed)
	if sender != addr {
		t.Errorf("recovered sender differs from original, want %v, got %v", addr, sender)
	}

	encoded, _ = rlp.EncodeToBytes(signed)
	parsed = &Transaction{}
	rlp.DecodeBytes(encoded, &parsed)
	if signed.Hash() != parsed.Hash() {
		t.Errorf("RLP parsed post-signing tx differs from original, want %v, got %v", signed, parsed)
	}
	encoded, _ = signed.MarshalJSON()
	parsed = &Transaction{}
	parsed.UnmarshalJSON(encoded)
	if signed.Hash() != parsed.Hash() {
		t.Errorf("JSON parsed post-signing tx differs from original, want %v, got %v", signed, parsed)
	}
}

// Tests that transactions can be correctly sorted according to their price in
// decreasing order, but at the same time with increasing nonces when issued by
// the same account.
func TestTransactionPriceNonceSort(t *testing.T) {
	// Generate a batch of accounts to start with
	keys := make([]*ecdsa.PrivateKey, 25)
	for i := 0; i < len(keys); i++ {
		keys[i], _ = crypto.GenerateKey()
	}

	signer := HomesteadSigner{}
	// Generate a batch of transactions with overlapping values, but shifted nonces
	groups := map[common.Address]Transactions{}
	for start, key := range keys {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		for i := 0; i < 25; i++ {
			tx, _ := SignTx(NewTransaction(uint64(i), common.Address{}, big.NewInt(100), 100, big.NewInt(int64(25-i)), nil, nil, nil, nil), signer, key)
			tx.SetReceivedTime(time.Unix(0, int64(start)))
			groups[addr] = append(groups[addr], tx)
		}
	}
	// Sort the transactions and cross check the nonce ordering
	txset := NewTransactionsByPriceAndNonce(signer, groups, func(tx1, tx2 *Transaction) int { return tx1.GasPrice().Cmp(tx2.GasPrice()) })

	txs := Transactions{}
	for tx := txset.Peek(); tx != nil; tx = txset.Peek() {
		txs = append(txs, tx)
		txset.Shift()
	}
	if len(txs) != 25*25 {
		t.Errorf("expected %d transactions, found %d", 25*25, len(txs))
	}
	for i, txi := range txs {
		fromi, _ := Sender(signer, txi)

		// Make sure the nonce order is valid
		for j, txj := range txs[i+1:] {
			fromj, _ := Sender(signer, txj)

			if fromi == fromj && txi.Nonce() > txj.Nonce() {
				t.Errorf("invalid nonce ordering: tx #%d (A=%x N=%v) < tx #%d (A=%x N=%v)", i, fromi[:4], txi.Nonce(), i+j, fromj[:4], txj.Nonce())
			}
		}

		// If the next tx has different from account, the price must be lower than the current one
		if i+1 < len(txs) {
			next := txs[i+1]
			fromNext, _ := Sender(signer, next)
			if fromi != fromNext && txi.GasPrice().Cmp(next.GasPrice()) < 0 {
				t.Errorf("invalid gasprice ordering: tx #%d (A=%x P=%v) < tx #%d (A=%x P=%v)", i, fromi[:4], txi.GasPrice(), i+1, fromNext[:4], next.GasPrice())
			}

			// Make sure receivedTime order is ascending if the txs have the same gas price
			if txi.GasPrice().Cmp(next.GasPrice()) == 0 && fromi != fromNext && txi.receivedTime.UnixNano() > next.receivedTime.UnixNano() {
				t.Errorf("invalid received time ordering: tx #%d (A=%x T=%d) > tx #%d (A=%x T=%d)", i, fromi[:4], txi.receivedTime.UnixNano(), i+1, fromNext[:4], next.receivedTime.UnixNano())
			}
		}
	}
}

// TestTransactionJSON tests serializing/de-serializing to/from JSON.
func TestTransactionJSON(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("could not generate key: %v", err)
	}
	signer := NewEIP155Signer(common.Big1)

	transactions := make([]*Transaction, 0, 50)
	for i := uint64(0); i < 25; i++ {
		var tx *Transaction
		switch i % 2 {
		case 0:
			tx = NewTransaction(i, common.Address{1}, common.Big0, 1, common.Big2, nil, nil, nil, []byte("abcdef"))
		case 1:
			tx = NewContractCreation(i, common.Big0, 1, common.Big2, nil, nil, nil, []byte("abcdef"))
		}
		transactions = append(transactions, tx)

		signedTx, err := SignTx(tx, signer, key)
		if err != nil {
			t.Fatalf("could not sign transaction: %v", err)
		}

		transactions = append(transactions, signedTx)
	}

	for _, tx := range transactions {
		data, err := json.Marshal(tx)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		var parsedTx *Transaction
		if err := json.Unmarshal(data, &parsedTx); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		// compare nonce, price, gaslimit, recipient, amount, payload, V, R, S
		if tx.Hash() != parsedTx.Hash() {
			t.Errorf("parsed tx differs from original tx, want %v, got %v", tx, parsedTx)
		}
		if tx.ChainId().Cmp(parsedTx.ChainId()) != 0 {
			t.Errorf("invalid chain id, want %d, got %d", tx.ChainId(), parsedTx.ChainId())
		}
	}
}
