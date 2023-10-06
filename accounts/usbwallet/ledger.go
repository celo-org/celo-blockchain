// Copyright 2017 The go-ethereum Authors
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

// This file contains the implementation for interacting with the Ledger hardware
// wallets. The wire protocol spec can be found in the Ledger Blue GitHub repo:
// https://raw.githubusercontent.com/LedgerHQ/blue-app-eth/master/doc/ethapp.asc

package usbwallet

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/celo-org/celo-blockchain/accounts"
	"github.com/celo-org/celo-blockchain/accounts/usbwallet/ledger"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/rlp"
)

// ledgerOpcode is an enumeration encoding the supported Ledger opcodes.
type ledgerOpcode byte

// ledgerParam1 is an enumeration encoding the supported Ledger parameters for
// specific opcodes. The same parameter values may be reused between opcodes.
type ledgerParam1 byte

// ledgerParam2 is an enumeration encoding the supported Ledger parameters for
// specific opcodes. The same parameter values may be reused between opcodes.
type ledgerParam2 byte

const (
	ledgerOpRetrieveAddress  ledgerOpcode = 0x02 // Returns the public key and Celo address for a given BIP 32 path
	ledgerOpSignTransaction  ledgerOpcode = 0x04 // Signs a Celo transaction after having the user validate the parameters
	ledgerOpGetConfiguration ledgerOpcode = 0x06 // Returns specific wallet application configuration
	ledgerOpSignMessage      ledgerOpcode = 0x08 // Signs a Celo message after having the user validate the parameters
	ledgerOpProvideERC20     ledgerOpcode = 0x0A // Provides ERC20 information for tokens

	/* TODO: add functionality to the Ledger's Celo app
	ledgerOpSignTypedMessage ledgerOpcode = 0x0c // Signs a Celo message following the EIP 712 specification
	*/

	ledgerP1DirectlyFetchAddress ledgerParam1 = 0x00 // Return address directly from the wallet
	ledgerP1ShowFetchAddress     ledgerParam1 = 0x01 // Return address from the wallet after showing it
	/* TODO: add functionality to the Ledger's Celo app
	ledgerP1InitTypedMessageData    ledgerParam1 = 0x00 // First chunk of Typed Message data
	*/
	ledgerP1InitTransactionData     ledgerParam1 = 0x00 // First transaction data block for signing
	ledgerP1ContTransactionData     ledgerParam1 = 0x80 // Subsequent transaction data block for signing
	ledgerP2DiscardAddressChainCode ledgerParam2 = 0x00 // Do not return the chain code along with the address

	statusCodeOK = 0x9000
)

// errLedgerReplyInvalidHeader is the error message returned by a Ledger data exchange
// if the device replies with a mismatching header. This usually means the device
// is in browser mode.
var errLedgerReplyInvalidHeader = errors.New("ledger: invalid reply header")

// errLedgerInvalidVersionReply is the error message returned by a Ledger version retrieval
// when a response does arrive, but it does not contain the expected data.
var errLedgerInvalidVersionReply = errors.New("ledger: invalid version reply")

// errLedgerBadStatusCode is the error message returned by any Ledger command
// when a response arrives with a bad status code.
var errLedgerBadStatusCode = errors.New("ledger: bad status code")

// ledgerDriver implements the communication with a Ledger hardware wallet.
type ledgerDriver struct {
	device  io.ReadWriter  // USB device connection to communicate through
	version [3]byte        // Current version of the Ledger firmware (zero if app is offline)
	browser bool           // Flag whether the Ledger is in browser mode (reply channel mismatch)
	failure error          // Any failure that would make the device unusable
	log     log.Logger     // Contextual logger to tag the ledger with its id
	tokens  *ledger.Tokens // Tokens list
}

// newLedgerDriver creates a new instance of a Ledger USB protocol driver.
func newLedgerDriver(logger log.Logger) driver {
	return &ledgerDriver{
		log: logger,
	}
}

// Status implements usbwallet.driver, returning various states the Ledger can
// currently be in.
func (w *ledgerDriver) Status() (string, error) {
	if w.failure != nil {
		return fmt.Sprintf("Failed: %v", w.failure), w.failure
	}
	if w.browser {
		return "Celo app in browser mode", w.failure
	}
	if w.offline() {
		return "Celo app offline", w.failure
	}
	return fmt.Sprintf("Celo app v%d.%d.%d online", w.version[0], w.version[1], w.version[2]), w.failure
}

// offline returns whether the wallet and the Celo app is offline or not.
//
// The method assumes that the state lock is held!
func (w *ledgerDriver) offline() bool {
	return w.version == [3]byte{0, 0, 0}
}

// Open implements usbwallet.driver, attempting to initialize the connection to the
// Ledger hardware wallet. The Ledger does not require a user passphrase, so that
// parameter is silently discarded.
func (w *ledgerDriver) Open(device io.ReadWriter, passphrase string) error {
	w.device, w.failure = device, nil

	_, err := w.ledgerDerive(accounts.DefaultBaseDerivationPath, false)
	if err != nil {
		// Celo app is not running or in browser mode, nothing more to do, return
		if err == errLedgerReplyInvalidHeader {
			w.browser = true
		}
		return nil
	}

	// Try to resolve the Celo app's version
	if w.version, err = w.ledgerVersion(); err != nil {
		w.version = [3]byte{1, 0, 0} // Assume worst case
	}

	/*
	  This is an example of how to enforce version numbers for features

	  // Ensure the wallet is capable of signing the given transaction
	  if chainID != nil && w.version[0] <= 1 && w.version[1] == 0 && w.version[2] <= 2 {
	    return common.Address{}, nil, fmt.Errorf("Ledger v%d.%d.%d doesn't support signing this transaction, please update to v1.0.3 at least", w.version[0], w.version[1], w.version[2])
	  }
	*/
	if w.tokens, err = ledger.LoadTokens(ledger.TokensBlob); err != nil {
		return err
	}

	return nil
}

// ConfirmAddress implements usbwallet.driver, showing the address on the device.
func (w *ledgerDriver) ConfirmAddress(path accounts.DerivationPath) (common.Address, error) {
	return w.ledgerDerive(path, true)
}

// Close implements usbwallet.driver, cleaning up and metadata maintained within
// the Ledger driver.
func (w *ledgerDriver) Close() error {
	w.browser, w.version = false, [3]byte{}
	return nil
}

// Heartbeat implements usbwallet.driver, performing a sanity check against the
// Ledger to see if it's still online.
func (w *ledgerDriver) Heartbeat() error {
	if _, err := w.ledgerVersion(); err != nil && err != errLedgerInvalidVersionReply {
		w.failure = err
		return err
	}
	return nil
}

// Derive implements usbwallet.driver, sending a derivation request to the Ledger
// and returning the Celo address located on that derivation path.
func (w *ledgerDriver) Derive(path accounts.DerivationPath) (common.Address, error) {
	return w.ledgerDerive(path, false)
}

// SignTx implements usbwallet.driver, sending the transaction to the Ledger and
// waiting for the user to confirm or deny the transaction.
//
// Note, if the version of the Celo application running on the Ledger wallet is
// too old to sign EIP-155 transactions, but such is requested nonetheless, an error
// will be returned opposed to silently signing in Homestead mode.
func (w *ledgerDriver) SignTx(path accounts.DerivationPath, tx *types.Transaction, chainID *big.Int) (common.Address, *types.Transaction, error) {
	// If the Celo app doesn't run, abort
	if w.offline() {
		return common.Address{}, nil, accounts.ErrWalletClosed
	}
	// All infos gathered and metadata checks out, request signing
	return w.ledgerSign(path, tx, chainID)
}

// SignPersonalMessage implements usbwallet.driver, sending the message to the Ledger and
// waiting for the user to confirm or deny the message.
func (w *ledgerDriver) SignPersonalMessage(path accounts.DerivationPath, message []byte) (common.Address, []byte, []byte, error) {
	// If the Celo app doesn't run, abort
	if w.offline() {
		return common.Address{}, nil, nil, accounts.ErrWalletClosed
	}

	// All infos gathered and metadata checks out, request signing
	return w.ledgerSignData(path, message)
}

/*
	TODO: add functionality to the Ledger's Celo app

// SignTypedMessage implements usbwallet.driver, sending the message to the Ledger and
// waiting for the user to sign or deny the transaction.
//
// Note: this was introduced in the ledger 1.5.0 firmware

	func (w *ledgerDriver) SignTypedMessage(path accounts.DerivationPath, domainHash []byte, messageHash []byte) ([]byte, error) {
		// If the Ethereum app doesn't run, abort
		if w.offline() {
			return nil, accounts.ErrWalletClosed
		}
		// Ensure the wallet is capable of signing the given transaction
		if w.version[0] < 1 && w.version[1] < 5 {
			//lint:ignore ST1005 brand name displayed on the console
			return nil, fmt.Errorf("Ledger version >= 1.5.0 required for EIP-712 signing (found version v%d.%d.%d)", w.version[0], w.version[1], w.version[2])
		}
		// All infos gathered and metadata checks out, request signing
		return w.ledgerSignTypedMessage(path, domainHash, messageHash)
	}
*/
func (w *ledgerDriver) SignTypedMessage(path accounts.DerivationPath, domainHash []byte, messageHash []byte) ([]byte, error) {
	return nil, accounts.ErrNotSupported
}

// ledgerVersion retrieves the current version of the Ethereum wallet app running
// on the Ledger wallet.
//
// The version retrieval protocol is defined as follows:
//
//	CLA | INS | P1 | P2 | Lc | Le
//	----+-----+----+----+----+---
//	 E0 | 06  | 00 | 00 | 00 | 04
//
// With no input data, and the output data being:
//
//	Description                                        | Length
//	---------------------------------------------------+--------
//	Flags 01: arbitrary data signature enabled by user | 1 byte
//	Application major version                          | 1 byte
//	Application minor version                          | 1 byte
//	Application patch version                          | 1 byte
func (w *ledgerDriver) ledgerVersion() ([3]byte, error) {
	// Send the request and wait for the response
	reply, err := w.ledgerExchange(ledgerOpGetConfiguration, 0, 0, nil)
	if err != nil {
		return [3]byte{}, err
	}
	if len(reply) != 4 {
		return [3]byte{}, errLedgerInvalidVersionReply
	}
	// Cache the version for future reference
	var version [3]byte
	copy(version[:], reply[1:])
	return version, nil
}

// ledgerDerive retrieves the currently active Celo address from a Ledger
// wallet at the specified derivation path.
//
// The address derivation protocol is defined as follows:
//
//	CLA | INS | P1 | P2 | Lc  | Le
//	----+-----+----+----+-----+---
//	 E0 | 02  | 00 return address
//	            01 display address and confirm before returning
//	               | 00: do not return the chain code
//	               | 01: return the chain code
//	                    | var | 00
//
// Where the input data is:
//
//	Description                                      | Length
//	-------------------------------------------------+--------
//	Number of BIP 32 derivations to perform (max 10) | 1 byte
//	First derivation index (big endian)              | 4 bytes
//	...                                              | 4 bytes
//	Last derivation index (big endian)               | 4 bytes
//
// And the output data is:
//
//	Description             | Length
//	------------------------+-------------------
//	Public Key length       | 1 byte
//	Uncompressed Public Key | arbitrary
//	Celo address length | 1 byte
//	Celo address        | 40 bytes hex ascii
//	Chain code if requested | 32 bytes
func (w *ledgerDriver) ledgerDerive(derivationPath []uint32, showOnWallet bool) (common.Address, error) {
	// Flatten the derivation path into the Ledger request
	path := make([]byte, 1+4*len(derivationPath))
	path[0] = byte(len(derivationPath))
	for i, component := range derivationPath {
		binary.BigEndian.PutUint32(path[1+4*i:], component)
	}
	// Send the request and wait for the response
	p1 := ledgerP1DirectlyFetchAddress
	if showOnWallet {
		p1 = ledgerP1ShowFetchAddress
	}
	reply, err := w.ledgerExchange(ledgerOpRetrieveAddress, p1, ledgerP2DiscardAddressChainCode, path)
	if err != nil {
		return common.Address{}, err
	}
	// Discard the public key, we don't need that for now
	if len(reply) < 1 || len(reply) < 1+int(reply[0]) {
		return common.Address{}, errors.New("reply lacks public key entry")
	}
	reply = reply[1+int(reply[0]):]

	// Extract the Celo hex address string
	if len(reply) < 1 || len(reply) < 1+int(reply[0]) {
		return common.Address{}, errors.New("reply lacks address entry")
	}
	hexstr := reply[1 : 1+int(reply[0])]

	// Decode the hex sting into an Celo address and return
	var address common.Address
	if _, err = hex.Decode(address[:], hexstr); err != nil {
		return common.Address{}, err
	}
	return address, nil
}

// ledgerSign sends the transaction to the Ledger wallet, and waits for the user
// to confirm or deny the transaction.
//
// The transaction signing protocol is defined as follows:
//
//	CLA | INS | P1 | P2 | Lc  | Le
//	----+-----+----+----+-----+---
//	 E0 | 04  | 00: first transaction data block
//	            80: subsequent transaction data block
//	               | 00 | variable | variable
//
// Where the input for the first transaction block (first 255 bytes) is:
//
//	Description                                      | Length
//	-------------------------------------------------+----------
//	Number of BIP 32 derivations to perform (max 10) | 1 byte
//	First derivation index (big endian)              | 4 bytes
//	...                                              | 4 bytes
//	Last derivation index (big endian)               | 4 bytes
//	RLP transaction chunk                            | arbitrary
//
// And the input for subsequent transaction blocks (first 255 bytes) are:
//
//	Description           | Length
//	----------------------+----------
//	RLP transaction chunk | arbitrary
//
// And the output data is:
//
//	Description | Length
//	------------+---------
//	signature V | 1 byte
//	signature R | 32 bytes
//	signature S | 32 bytes
func (w *ledgerDriver) ledgerSign(derivationPath []uint32, tx *types.Transaction, chainID *big.Int) (common.Address, *types.Transaction, error) {
	// Flatten the derivation path into the Ledger request
	path := make([]byte, 1+4*len(derivationPath))
	path[0] = byte(len(derivationPath))
	for i, component := range derivationPath {
		binary.BigEndian.PutUint32(path[1+4*i:], component)
	}

	var (
		txrlp []byte
		err   error
	)

	if ledger.IsERC20Transfer(tx.Data()) {
		err = w.ledgerProvideERC20(*tx.To(), chainID)
		if err != nil && err != ledger.ErrCouldNotFindToken {
			return common.Address{}, nil, err
		}

		if tx.FeeCurrency() != nil {
			err = w.ledgerProvideERC20(*tx.FeeCurrency(), chainID)
			if err != nil && err != ledger.ErrCouldNotFindToken {
				return common.Address{}, nil, err
			}
		}
	}

	// Create the transaction RLP based on whether legacy or EIP155 signing was requested
	if chainID == nil {
		//if txrlp, err = rlp.EncodeToBytes([]interface{}{tx.Nonce(), tx.GasPrice(), tx.Gas(), tx.To(), tx.Value(), tx.Data()}); err != nil {
		if txrlp, err = rlp.EncodeToBytes([]interface{}{tx.Nonce(), tx.GasPrice(), tx.Gas(), tx.FeeCurrency(), tx.GatewayFeeRecipient(), tx.GatewayFee(), tx.To(), tx.Value(), tx.Data()}); err != nil {
			return common.Address{}, nil, err
		}
	} else {
		if txrlp, err = rlp.EncodeToBytes([]interface{}{tx.Nonce(), tx.GasPrice(), tx.Gas(), tx.FeeCurrency(), tx.GatewayFeeRecipient(), tx.GatewayFee(), tx.To(), tx.Value(), tx.Data(), chainID, big.NewInt(0), big.NewInt(0)}); err != nil {
			return common.Address{}, nil, err
		}
	}
	payload := append(path, txrlp...)

	// Send the request and wait for the response
	var (
		op    = ledgerP1InitTransactionData
		reply []byte
	)
	for len(payload) > 0 {
		// Calculate the size of the next data chunk
		chunk := 255
		if chunk > len(payload) {
			chunk = len(payload)
		}
		// Send the chunk over, ensuring it's processed correctly
		reply, err = w.ledgerExchange(ledgerOpSignTransaction, op, 0, payload[:chunk])
		if err != nil {
			return common.Address{}, nil, err
		}
		// Shift the payload and ensure subsequent chunks are marked as such
		payload = payload[chunk:]
		op = ledgerP1ContTransactionData
	}
	// Extract the Celo signature and do a sanity validation
	if len(reply) != crypto.SignatureLength {
		return common.Address{}, nil, errors.New("reply lacks signature")
	}
	signature := append(reply[1:], reply[0])

	// Create the correct signer and signature transform based on the chain ID
	var signer types.Signer
	if chainID == nil {
		signer = new(types.HomesteadSigner)
	} else {
		signer = types.NewEIP155Signer(chainID)
		signature[64] -= byte(chainID.Uint64()*2 + 35)
	}
	signed, err := tx.WithSignature(signer, signature)
	if err != nil {
		return common.Address{}, nil, err
	}
	sender, err := types.Sender(signer, signed)
	if err != nil {
		return common.Address{}, nil, err
	}
	return sender, signed, nil
}

// ledgerProvideERC20 provides ERC20 information for tokens.
//
// The data protocol is defined as follows:
//
//	CLA | INS | P1 | P2 | Lc  | Le
//	----+-----+----+----+-----+---
//	 E0 | 04  | 00: first data block
//	            80: subsequent data block
//	               | 00 | variable | variable
//
// Where the input for the data block is:
//
//	Description                                      | Length
//	-------------------------------------------------+----------
//	Ticker length                                    | 1 byte
//	Ticker                                           | abitrary (< 10 bytes)
//	Address                                          | 20 bytes
//	Decimals                                         | 4 bytes
//	ChainID                                          | 4 bytes
//	Signature                                        | arbitrary
//
// And the output data is nothing.
func (w *ledgerDriver) ledgerProvideERC20(contractAddress common.Address, chainID *big.Int) error {
	token, err := w.tokens.ByContractAddressAndChainID(contractAddress, chainID)
	if err != nil {
		return err
	}

	_, err = w.ledgerExchange(ledgerOpProvideERC20, 0, 0, token.Data)
	if err != nil {
		return err
	}

	return nil
}

// ledgerSignData sends the message to the Ledger wallet, and waits for the user
// to confirm or deny the message.
//
// The message signing protocol is defined as follows:
//
//	CLA | INS | P1 | P2 | Lc  | Le
//	----+-----+----+----+-----+---
//	 E0 | 08  | 00: first message data block
//	            80: subsequent message data block
//	               | 00 | variable | variable
//
// Where the input for the first message block (first 255 bytes) is:
//
//	Description                                      | Length
//	-------------------------------------------------+----------
//	Number of BIP 32 derivations to perform (max 10) | 1 byte
//	First derivation index (big endian)              | 4 bytes
//	...                                              | 4 bytes
//	Last derivation index (big endian)               | 4 bytes
//	data chunk                                       | arbitrary
//
// And the input for subsequent message blocks (first 255 bytes) are:
//
//	Description           | Length
//	----------------------+----------
//	data chunk | arbitrary
//
// And the output data is:
//
//	Description | Length
//	------------+---------
//	signature V | 1 byte
//	signature R | 32 bytes
//	signature S | 32 bytes
func (w *ledgerDriver) ledgerSignData(derivationPath []uint32, data []byte) (common.Address, []byte, []byte, error) {
	hashMessage := sha256.Sum256(data)
	w.log.Info("Signing message on Ledger with hash", "message", hex.EncodeToString(data), "hash", hex.EncodeToString(hashMessage[:]))

	// Flatten the derivation path into the Ledger request
	path := make([]byte, 1+4*len(derivationPath))
	path[0] = byte(len(derivationPath))
	for i, component := range derivationPath {
		binary.BigEndian.PutUint32(path[1+4*i:], component)
	}

	dataLen := make([]byte, 4)
	binary.BigEndian.PutUint32(dataLen, uint32(len(data)))
	payload := append(path, dataLen...)
	payload = append(payload, data...)

	// Send the request and wait for the response
	var (
		op    = ledgerP1InitTransactionData
		reply []byte
		err   error
	)
	for len(payload) > 0 {
		// Calculate the size of the next data chunk
		chunk := 255
		if chunk > len(payload) {
			chunk = len(payload)
		}
		// Send the chunk over, ensuring it's processed correctly
		reply, err = w.ledgerExchange(ledgerOpSignMessage, op, 0, payload[:chunk])
		if err != nil {
			return common.Address{}, nil, nil, err
		}
		// Shift the payload and ensure subsequent chunks are marked as such
		payload = payload[chunk:]
		op = ledgerP1ContTransactionData
	}
	// Extract the Celo signature and do a sanity validation
	if len(reply) != crypto.SignatureLength {
		return common.Address{}, nil, nil, errors.New("reply lacks signature")
	}
	signature := append(reply[1:], reply[0])

	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(data), data)
	hash := crypto.Keccak256([]byte(msg))
	hashFixedLength := common.Hash{}
	copy(hashFixedLength[:], hash)

	signer := new(types.HomesteadSigner)
	addr, pubkey, err := signer.SenderData(hashFixedLength, signature)
	if err != nil {
		return common.Address{}, nil, nil, err
	}

	return addr, pubkey, signature, nil
}

/* TODO: add functionality to the Ledger's Celo app
// ledgerSignTypedMessage sends the transaction to the Ledger wallet, and waits for the user
// to confirm or deny the transaction.
//
// The signing protocol is defined as follows:
//
//	CLA | INS | P1 | P2                          | Lc  | Le
//	----+-----+----+-----------------------------+-----+---
//	 E0 | 0C  | 00 | implementation version : 00 | variable | variable
//
// Where the input is:
//
//	Description                                      | Length
//	-------------------------------------------------+----------
//	Number of BIP 32 derivations to perform (max 10) | 1 byte
//	First derivation index (big endian)              | 4 bytes
//	...                                              | 4 bytes
//	Last derivation index (big endian)               | 4 bytes
//	domain hash                                      | 32 bytes
//	message hash                                     | 32 bytes
//
// And the output data is:
//
//	Description | Length
//	------------+---------
//	signature V | 1 byte
//	signature R | 32 bytes
//	signature S | 32 bytes
func (w *ledgerDriver) ledgerSignTypedMessage(derivationPath []uint32, domainHash []byte, messageHash []byte) ([]byte, error) {
	// Flatten the derivation path into the Ledger request
	path := make([]byte, 1+4*len(derivationPath))
	path[0] = byte(len(derivationPath))
	for i, component := range derivationPath {
		binary.BigEndian.PutUint32(path[1+4*i:], component)
	}
	// Create the 712 message
	payload := append(path, domainHash...)
	payload = append(payload, messageHash...)

	// Send the request and wait for the response
	var (
		op    = ledgerP1InitTypedMessageData
		reply []byte
		err   error
	)

	// Send the message over, ensuring it's processed correctly
	reply, err = w.ledgerExchange(ledgerOpSignTypedMessage, op, 0, payload)

	if err != nil {
		return nil, err
	}

	// Extract the Ethereum signature and do a sanity validation
	if len(reply) != crypto.SignatureLength {
		return nil, errors.New("reply lacks signature")
	}
	signature := append(reply[1:], reply[0])
	return signature, nil
}
*/

// ledgerExchange performs a data exchange with the Ledger wallet, sending it a
// message and retrieving the response.
//
// The common transport header is defined as follows:
//
//	Description                           | Length
//	--------------------------------------+----------
//	Communication channel ID (big endian) | 2 bytes
//	Command tag                           | 1 byte
//	Packet sequence index (big endian)    | 2 bytes
//	Payload                               | arbitrary
//
// The Communication channel ID allows commands multiplexing over the same
// physical link. It is not used for the time being, and should be set to 0101
// to avoid compatibility issues with implementations ignoring a leading 00 byte.
//
// The Command tag describes the message content. Use TAG_APDU (0x05) for standard
// APDU payloads, or TAG_PING (0x02) for a simple link test.
//
// The Packet sequence index describes the current sequence for fragmented payloads.
// The first fragment index is 0x00.
//
// APDU Command payloads are encoded as follows:
//
//	Description              | Length
//	-----------------------------------
//	APDU length (big endian) | 2 bytes
//	APDU CLA                 | 1 byte
//	APDU INS                 | 1 byte
//	APDU P1                  | 1 byte
//	APDU P2                  | 1 byte
//	APDU length              | 1 byte
//	Optional APDU data       | arbitrary
func (w *ledgerDriver) ledgerExchange(opcode ledgerOpcode, p1 ledgerParam1, p2 ledgerParam2, data []byte) ([]byte, error) {
	// Construct the message payload, possibly split into multiple chunks
	apdu := make([]byte, 2, 7+len(data))

	binary.BigEndian.PutUint16(apdu, uint16(5+len(data)))
	apdu = append(apdu, []byte{0xe0, byte(opcode), byte(p1), byte(p2), byte(len(data))}...)
	apdu = append(apdu, data...)

	// Stream all the chunks to the device
	header := []byte{0x01, 0x01, 0x05, 0x00, 0x00} // Channel ID and command tag appended
	chunk := make([]byte, 64)
	space := len(chunk) - len(header)

	for i := 0; len(apdu) > 0; i++ {
		// Construct the new message to stream
		chunk = append(chunk[:0], header...)
		binary.BigEndian.PutUint16(chunk[3:], uint16(i))

		if len(apdu) > space {
			chunk = append(chunk, apdu[:space]...)
			apdu = apdu[space:]
		} else {
			chunk = append(chunk, apdu...)
			apdu = nil
		}
		// Send over to the device
		w.log.Trace("Data chunk sent to the Ledger", "chunk", hexutil.Bytes(chunk))
		if _, err := w.device.Write(chunk); err != nil {
			return nil, err
		}
	}
	// Stream the reply back from the wallet in 64 byte chunks
	var reply []byte
	chunk = chunk[:64] // Yeah, we surely have enough space
	for {
		// Read the next chunk from the Ledger wallet
		if _, err := io.ReadFull(w.device, chunk); err != nil {
			return nil, err
		}
		w.log.Trace("Data chunk received from the Ledger", "chunk", hexutil.Bytes(chunk))

		// Make sure the transport header matches
		if chunk[0] != 0x01 || chunk[1] != 0x01 || chunk[2] != 0x05 {
			return nil, errLedgerReplyInvalidHeader
		}
		// If it's the first chunk, retrieve the total message length
		var payload []byte

		if chunk[3] == 0x00 && chunk[4] == 0x00 {
			reply = make([]byte, 0, int(binary.BigEndian.Uint16(chunk[5:7])))
			payload = chunk[7:]
		} else {
			payload = chunk[5:]
		}
		// Append to the reply and stop when filled up
		if left := cap(reply) - len(reply); left > len(payload) {
			reply = append(reply, payload...)
		} else {
			reply = append(reply, payload[:left]...)
			break
		}
	}

	statusCodeBytes := reply[len(reply)-2:]
	statusCode := int(binary.BigEndian.Uint16(statusCodeBytes))
	if statusCode != statusCodeOK {
		return nil, errLedgerBadStatusCode
	}
	return reply[:len(reply)-2], nil
}
