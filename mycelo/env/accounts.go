package env

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/mycelo/hdwallet"
	"github.com/tyler-smith/go-bip39"
)

// MustNewMnemonic creates a new mnemonic (panics on error)
func MustNewMnemonic() string {
	res, err := NewMnemonic()
	if err != nil {
		panic(err)
	}
	return res
}

// NewMnemonic creates a new mnemonic
func NewMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}

// Account represents a Celo Account
type Account struct {
	Address    common.Address
	PrivateKey *ecdsa.PrivateKey
}

// AccountType represents the different account types for the generator
type AccountType int

// The differente account types for the generator
const (
	Validator AccountType = iota
	Developer             // load test
	TxNode
	Faucet
	Attestation
	PriceOracle
	Proxy
	AttestationBot
	VotingBot
	TxNodePrivate
	ValidatorGroup // Not in celotool
	Admin          // Not in celotool
)

func mustDerivationPath(accountType AccountType, idx int) accounts.DerivationPath {
	return hdwallet.MustParseDerivationPath(fmt.Sprintf("m/%d/%d", int(accountType), idx))
}

// GenerateAccounts will generate the desired number of accounts using mnemonic & accountType
func GenerateAccounts(mnemonic string, accountType AccountType, qty int) ([]Account, error) {
	wallet, err := hdwallet.NewFromMnemonic(mnemonic)
	if err != nil {
		log.Fatal(err)
	}

	accounts := make([]Account, qty)

	for i := 0; i < qty; i++ {
		account, err := wallet.Derive(mustDerivationPath(accountType, i), false)
		if err != nil {
			return nil, err
		}
		pk, err := wallet.PrivateKey(account)
		if err != nil {
			return nil, err
		}
		accounts[i] = Account{
			Address:    account.Address,
			PrivateKey: pk,
		}
	}

	return accounts, nil
}

// MustGenerateAccount creates new account or panics
func MustGenerateAccount() Account {
	acc, err := GenerateAccount()
	if err != nil {
		panic(err)
	}
	return acc
}

// GenerateAccount creates a random new account
func GenerateAccount() (Account, error) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return Account{}, err
	}
	return NewAccount(privateKey), nil
}

// NewAccount creates a new account for the specified private key
func NewAccount(key *ecdsa.PrivateKey) Account {
	return Account{
		PrivateKey: key,
		Address:    crypto.PubkeyToAddress(key.PublicKey),
	}
}

// UnmarshalJSON implements json.Unmarshaler
func (a *Account) UnmarshalJSON(b []byte) error {
	var data struct {
		PrivateKey string
		Address    common.Address
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	a.Address = data.Address
	key, err := crypto.HexToECDSA(data.PrivateKey)
	if err != nil {
		return err
	}
	a.PrivateKey = key
	return nil
}

// MarshalJSON implements json.Marshaler
func (a Account) MarshalJSON() ([]byte, error) {
	var data = struct {
		PrivateKey string
		Address    common.Address
	}{
		PrivateKey: hex.EncodeToString(crypto.FromECDSA(a.PrivateKey)),
		Address:    a.Address,
	}

	return json.Marshal(data)
}

// BLSPublicKey returns the bls public key
func (a *Account) BLSPublicKey() (blscrypto.SerializedPublicKey, error) {
	privateKey, err := blscrypto.ECDSAToBLS(a.PrivateKey)
	if err != nil {
		return blscrypto.SerializedPublicKey{}, err
	}

	return blscrypto.PrivateToPublic(privateKey)
}
