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
	"github.com/celo-org/celo-bls-go/bls"
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

// MustBLSProofOfPossession variant of BLSProofOfPossession that panics on error
func (a *Account) MustBLSProofOfPossession() []byte {
	pop, err := a.BLSProofOfPossession()
	if err != nil {
		panic(err)
	}
	return pop
}

// BLSProofOfPossession generates bls proof of possession
func (a *Account) BLSProofOfPossession() ([]byte, error) {
	privateKeyBytes, err := blscrypto.ECDSAToBLS(a.PrivateKey)
	if err != nil {
		return nil, err
	}

	privateKey, err := bls.DeserializePrivateKey(privateKeyBytes)
	if err != nil {
		return nil, err
	}
	defer privateKey.Destroy()

	signature, err := privateKey.SignPoP(a.Address.Bytes())
	if err != nil {
		return nil, err
	}
	defer signature.Destroy()

	signatureBytes, err := signature.Serialize()
	if err != nil {
		return nil, err
	}
	return signatureBytes, nil
}

// BLSPublicKey returns the bls public key
func (a *Account) BLSPublicKey() (blscrypto.SerializedPublicKey, error) {
	privateKey, err := blscrypto.ECDSAToBLS(a.PrivateKey)
	if err != nil {
		return blscrypto.SerializedPublicKey{}, err
	}

	return blscrypto.PrivateToPublic(privateKey)
}

// PublicKeyHex hex representation of the public key
func (a *Account) PublicKey() []byte {
	return crypto.FromECDSAPub(&a.PrivateKey.PublicKey)
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
	ValidatorAT      AccountType = 0
	DeveloperAT      AccountType = 1 // load test
	TxNodeAT         AccountType = 2
	BootnodeAT       AccountType = 3
	FaucetAT         AccountType = 4
	AttestationAT    AccountType = 5
	PriceOracleAT    AccountType = 6
	ProxyAT          AccountType = 7
	AttestationBotAT AccountType = 8
	VotingBotAT      AccountType = 9
	TxNodePrivateAT  AccountType = 10
	ValidatorGroupAT AccountType = 11 // Not in celotool (yet)
	AdminAT          AccountType = 12 // Not in celotool (yet)
	TxFeeRecipientAT AccountType = 13 // Not in celotool (yet)
)

// String implements the stringer interface.
func (accountType AccountType) String() string {
	switch accountType {
	case ValidatorAT:
		return "validator"
	case DeveloperAT:
		return "developer"
	case TxNodeAT:
		return "txNode"
	case FaucetAT:
		return "faucet"
	case AttestationAT:
		return "attestation"
	case PriceOracleAT:
		return "priceOracle"
	case ProxyAT:
		return "proxy"
	case AttestationBotAT:
		return "attestationBot"
	case VotingBotAT:
		return "votingBot"
	case TxNodePrivateAT:
		return "txNodePrivate"
	case ValidatorGroupAT:
		return "validatorGroup"
	case AdminAT:
		return "admin"
	default:
		return "unknown"
	}
}

// MarshalText marshall account type into text
func (accountType AccountType) MarshalText() ([]byte, error) {
	switch accountType {
	case ValidatorAT:
		return []byte("validator"), nil
	case DeveloperAT:
		return []byte("developer"), nil
	case TxNodeAT:
		return []byte("txNode"), nil
	case FaucetAT:
		return []byte("faucet"), nil
	case AttestationAT:
		return []byte("attestation"), nil
	case PriceOracleAT:
		return []byte("priceOracle"), nil
	case ProxyAT:
		return []byte("proxy"), nil
	case AttestationBotAT:
		return []byte("attestationBot"), nil
	case VotingBotAT:
		return []byte("votingBot"), nil
	case TxNodePrivateAT:
		return []byte("txNodePrivate"), nil
	case ValidatorGroupAT:
		return []byte("validatorGroup"), nil
	case AdminAT:
		return []byte("admin"), nil
	default:
		return nil, fmt.Errorf("unknown account type %d", accountType)
	}
}

// UnmarshalText creates AccountType from string
func (accountType *AccountType) UnmarshalText(text []byte) error {
	switch string(text) {
	case "validator":
		*accountType = ValidatorAT
	case "developer":
		*accountType = DeveloperAT
	case "txNode":
		*accountType = TxNodeAT
	case "faucet":
		*accountType = FaucetAT
	case "attestation":
		*accountType = AttestationAT
	case "priceOracle":
		*accountType = PriceOracleAT
	case "proxy":
		*accountType = ProxyAT
	case "attestationBot":
		*accountType = AttestationBotAT
	case "votingBot":
		*accountType = VotingBotAT
	case "txNodePrivate":
		*accountType = TxNodePrivateAT
	case "validatorGroup":
		*accountType = ValidatorGroupAT
	case "admin":
		*accountType = AdminAT
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

// DeriveAccountList will generate the desired number of accounts using mnemonic & accountType
func DeriveAccountList(mnemonic string, accountType AccountType, qty int) ([]Account, error) {
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
