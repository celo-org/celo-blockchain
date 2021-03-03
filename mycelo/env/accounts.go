package env

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	"github.com/celo-org/celo-blockchain/accounts"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/crypto"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/mycelo/hdwallet"
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

// PrivateKeyHex hex representation of the private key
func (a *Account) PrivateKeyHex() string {
	return common.Bytes2Hex(crypto.FromECDSA(a.PrivateKey))
}

func (a *Account) String() string {
	return fmt.Sprintf("{ address: %s\tprivateKey: %s }",
		a.Address.Hex(),
		a.PrivateKeyHex(),
	)
}

// AccountType represents the different account types for the generator
type AccountType int

// The difference account types for the generator
var (
	Validator      AccountType = 0
	Developer      AccountType = 1 // load test
	TxNode         AccountType = 2
	Faucet         AccountType = 3
	Attestation    AccountType = 4
	PriceOracle    AccountType = 5
	Proxy          AccountType = 6
	AttestationBot AccountType = 7
	VotingBot      AccountType = 8
	TxNodePrivate  AccountType = 9
	ValidatorGroup AccountType = 10 // Not in celotool
	Admin          AccountType = 11 // Not in celotool
)

// String implements the stringer interface.
func (accountType AccountType) String() string {
	switch accountType {
	case Validator:
		return "validator"
	case Developer:
		return "developer"
	case TxNode:
		return "txNode"
	case Faucet:
		return "faucet"
	case Attestation:
		return "attestation"
	case PriceOracle:
		return "priceOracle"
	case Proxy:
		return "proxy"
	case AttestationBot:
		return "attestationBot"
	case VotingBot:
		return "votingBot"
	case TxNodePrivate:
		return "txNodePrivate"
	case ValidatorGroup:
		return "validatorGroup"
	case Admin:
		return "admin"
	default:
		return "unknown"
	}
}

// MarshalText marshall account type into text
func (accountType AccountType) MarshalText() ([]byte, error) {
	switch accountType {
	case Validator:
		return []byte("validator"), nil
	case Developer:
		return []byte("developer"), nil
	case TxNode:
		return []byte("txNode"), nil
	case Faucet:
		return []byte("faucet"), nil
	case Attestation:
		return []byte("attestation"), nil
	case PriceOracle:
		return []byte("priceOracle"), nil
	case Proxy:
		return []byte("proxy"), nil
	case AttestationBot:
		return []byte("attestationBot"), nil
	case VotingBot:
		return []byte("votingBot"), nil
	case TxNodePrivate:
		return []byte("txNodePrivate"), nil
	case ValidatorGroup:
		return []byte("validatorGroup"), nil
	case Admin:
		return []byte("admin"), nil
	default:
		return nil, fmt.Errorf("unknown account type %d", accountType)
	}
}

// UnmarshalText creates AccountType from string
func (accountType *AccountType) UnmarshalText(text []byte) error {
	switch string(text) {
	case "validator":
		*accountType = Validator
	case "developer":
		*accountType = Developer
	case "txNode":
		*accountType = TxNode
	case "faucet":
		*accountType = Faucet
	case "attestation":
		*accountType = Attestation
	case "priceOracle":
		*accountType = PriceOracle
	case "proxy":
		*accountType = Proxy
	case "attestationBot":
		*accountType = AttestationBot
	case "votingBot":
		*accountType = VotingBot
	case "txNodePrivate":
		*accountType = TxNodePrivate
	case "validatorGroup":
		*accountType = ValidatorGroup
	case "admin":
		*accountType = Admin
	default:
		return fmt.Errorf(`unknown account type %q, want "validator", "developer", "txNode", "faucet", "attestation", "priceOracle", "proxy", "attestationBot", "votingBot", "txNodePrivate", "validatorGroup", "admin"`, text)
	}
	return nil
}

func mustDerivationPath(accountType AccountType, idx int) accounts.DerivationPath {
	return hdwallet.MustParseDerivationPath(fmt.Sprintf("m/%d/%d", int(accountType), idx))
}

// DeriveAccount will derive the account corresponding to (accountType, idx) using the
// given mnemonic
func DeriveAccount(mnemonic string, accountType AccountType, idx int) (*Account, error) {
	wallet, err := hdwallet.NewFromMnemonic(mnemonic)
	if err != nil {
		return nil, err
	}
	account, err := wallet.Derive(mustDerivationPath(accountType, idx), false)
	if err != nil {
		return nil, err
	}
	pk, err := wallet.PrivateKey(account)
	if err != nil {
		return nil, err
	}

	return &Account{
		Address:    account.Address,
		PrivateKey: pk,
	}, nil
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

// MustGenerateRandomAccount creates new account or panics
func MustGenerateRandomAccount() Account {
	acc, err := GenerateRandomAccount()
	if err != nil {
		panic(err)
	}
	return acc
}

// GenerateRandomAccount creates a random new account
func GenerateRandomAccount() (Account, error) {
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
