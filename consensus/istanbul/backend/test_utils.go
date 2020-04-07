package backend

import (
	"crypto/ecdsa"
	"fmt"
	"strings"

	"github.com/celo-org/bls-zexe/go/bls"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/consensustest"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/params"
)

// in this test, we can set n to 1, and it means we can process Istanbul and commit a
// block by one node. Otherwise, if n is larger than 1, we have to generate
// other fake events to process Istanbul.
func newBlockChain(n int, isFullChain bool) (*core.BlockChain, *Backend) {
	genesis, nodeKeys := getGenesisAndKeys(n, isFullChain)
	memDB := rawdb.NewMemoryDatabase()
	config := istanbul.DefaultConfig
	config.ValidatorEnodeDBPath = ""
	config.VersionCertificateDBPath = ""
	config.RoundStateDBPath = ""
	// Use the first key as private key
	publicKey := nodeKeys[0].PublicKey
	address := crypto.PubkeyToAddress(publicKey)
	signerFn := func(_ accounts.Account, mimeType string, data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), nodeKeys[0])
	}

	signerBLSFn := signBLSFn(nodeKeys[0])

	b, _ := New(config, memDB).(*Backend)
	b.Authorize(address, &publicKey, decryptFn, signerFn, signerBLSFn)

	genesis.MustCommit(memDB)

	blockchain, err := core.NewBlockChain(memDB, nil, genesis.Config, b, vm.Config{}, nil)
	if err != nil {
		panic(err)
	}

	b.SetChain(blockchain, blockchain.CurrentBlock,
		func(hash common.Hash) (*state.StateDB, error) {
			stateRoot := blockchain.GetHeaderByHash(hash).Root
			return blockchain.StateAt(stateRoot)
		})
	b.SetBroadcaster(&consensustest.MockBroadcaster{})
	b.SetP2PServer(consensustest.NewMockP2PServer())
	b.StartAnnouncing()
	b.StartValidating(blockchain.HasBadBlock,
		func(block *types.Block, state *state.StateDB) (types.Receipts, []*types.Log, uint64, error) {
			return blockchain.Processor().Process(block, state, *blockchain.GetVMConfig())
		},
		func(block *types.Block, state *state.StateDB, receipts types.Receipts, usedGas uint64) error {
			return blockchain.Validator().ValidateState(block, state, receipts, usedGas)
		})
	snap, err := b.snapshot(blockchain, 0, common.Hash{}, nil)
	if err != nil {
		panic(err)
	}
	if snap == nil {
		panic("failed to get snapshot")
	}
	proposerAddr := b.AuthorForBlock(snap.Number)
	// proposerAddr := snap.ValSet.GetProposer().Address()

	// find proposer key
	for _, key := range nodeKeys {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if addr.String() == proposerAddr.String() {
			signerFn := func(_ accounts.Account, mimeType string, data []byte) ([]byte, error) {
				return crypto.Sign(data, key)
			}
			signerBLSFn := signBLSFn(key)
			b.Authorize(address, &publicKey, decryptFn, signerFn, signerBLSFn)
			break
		}
	}

	contract_comm.SetInternalEVMHandler(blockchain)

	return blockchain, b
}

func getGenesisAndKeys(n int, isFullChain bool) (*core.Genesis, []*ecdsa.PrivateKey) {
	// Setup validators
	var nodeKeys = make([]*ecdsa.PrivateKey, n)
	validators := make([]istanbul.ValidatorData, n)
	for i := 0; i < n; i++ {
		var addr common.Address
		if i == 1 {
			nodeKeys[i], _ = generatePrivateKey()
			addr = getAddress()
		} else {
			nodeKeys[i], _ = crypto.GenerateKey()
			addr = crypto.PubkeyToAddress(nodeKeys[i].PublicKey)
		}
		blsPrivateKey, _ := blscrypto.ECDSAToBLS(nodeKeys[i])
		blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
		validators[i] = istanbul.ValidatorData{
			Address:      addr,
			BLSPublicKey: blsPublicKey,
		}

	}

	// generate genesis block
	genesis := core.DefaultGenesisBlock()
	genesis.Config = params.DefaultChainConfig
	if !isFullChain {
		genesis.Config.FullHeaderChainAvailable = false
	}
	// force enable Istanbul engine
	genesis.Config.Istanbul = &params.IstanbulConfig{
		Epoch:          10,
		LookbackWindow: 2,
	}

	AppendValidatorsToGenesisBlock(genesis, validators)
	return genesis, nodeKeys
}

func makeHeader(parent *types.Block, config *istanbul.Config) *types.Header {
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     parent.Number().Add(parent.Number(), common.Big1),
		GasUsed:    0,
		Extra:      parent.Extra(),
		Time:       parent.Time() + config.BlockPeriod,
	}
	return header
}

func makeBlock(chain *core.BlockChain, engine *Backend, parent *types.Block) *types.Block {
	block := makeBlockWithoutSeal(chain, engine, parent)
	results := make(chan *types.Block)
	go func() { engine.Seal(chain, block, results, nil) }()
	block = <-results
	return block
}

func makeBlockWithoutSeal(chain *core.BlockChain, engine *Backend, parent *types.Block) *types.Block {
	header := makeHeader(parent, engine.config)
	engine.Prepare(chain, header)
	state, err := chain.StateAt(parent.Root())
	if err != nil {
		fmt.Printf("Error!! %v\n", err)
	}
	engine.Finalize(chain, header, state, nil)
	block, err := engine.FinalizeAndAssemble(chain, header, state, nil, nil, nil)
	if err != nil {
		fmt.Printf("Error!! %v\n", err)
	}
	return block
}

/**
 * SimpleBackend
 * Private key: bb047e5940b6d83354d9432db7c449ac8fca2248008aaa7271369880f9f11cc1
 * Public key: 04a2bfb0f7da9e1b9c0c64e14f87e8fb82eb0144e97c25fe3a977a921041a50976984d18257d2495e7bfd3d4b280220217f429287d25ecdf2b0d7c0f7aae9aa624
 * Address: 0x70524d664ffe731100208a0154e556f9bb679ae6
 */
func getAddress() common.Address {
	return common.HexToAddress("0x70524d664ffe731100208a0154e556f9bb679ae6")
}

func getInvalidAddress() common.Address {
	return common.HexToAddress("0xc63597005f0da07a9ea85b5052a77c3b0261bdca")
}

func generatePrivateKey() (*ecdsa.PrivateKey, error) {
	key := "bb047e5940b6d83354d9432db7c449ac8fca2248008aaa7271369880f9f11cc1"
	return crypto.HexToECDSA(key)
}

func newTestValidatorSet(n int) (istanbul.ValidatorSet, []*ecdsa.PrivateKey) {
	// generate validators
	keys := make(Keys, n)
	validators := make([]istanbul.ValidatorData, n)
	for i := 0; i < n; i++ {
		privateKey, _ := crypto.GenerateKey()
		blsPrivateKey, _ := blscrypto.ECDSAToBLS(privateKey)
		blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
		keys[i] = privateKey
		validators[i] = istanbul.ValidatorData{
			Address:      crypto.PubkeyToAddress(privateKey.PublicKey),
			BLSPublicKey: blsPublicKey,
		}
	}
	vset := validator.NewSet(validators)
	return vset, keys
}

type Keys []*ecdsa.PrivateKey

func (slice Keys) Len() int {
	return len(slice)
}

func (slice Keys) Less(i, j int) bool {
	return strings.Compare(crypto.PubkeyToAddress(slice[i].PublicKey).String(), crypto.PubkeyToAddress(slice[j].PublicKey).String()) < 0
}

func (slice Keys) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func decryptFn(_ accounts.Account, c, s1, s2 []byte) ([]byte, error) {
	ecdsaKey, _ := generatePrivateKey()
	eciesKey := ecies.ImportECDSA(ecdsaKey)
	return eciesKey.Decrypt(c, s1, s2)
}

func signFn(key *ecdsa.PrivateKey) istanbul.SignerFn {
	if key == nil {
		key, _ = generatePrivateKey()
	}

	return func(_ accounts.Account, mimeType string, data []byte) ([]byte, error) {
		key, _ := generatePrivateKey()
		return crypto.Sign(crypto.Keccak256(data), key)
	}
}

func signBLSFn(key *ecdsa.PrivateKey) istanbul.BLSSignerFn {
	if key == nil {
		key, _ = generatePrivateKey()
	}

	return func(_ accounts.Account, data []byte, extraData []byte, useComposite bool) (blscrypto.SerializedSignature, error) {
		privateKeyBytes, err := blscrypto.ECDSAToBLS(key)
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}

		privateKey, err := bls.DeserializePrivateKey(privateKeyBytes)
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}
		defer privateKey.Destroy()

		signature, err := privateKey.SignMessage(data, extraData, useComposite)
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}
		defer signature.Destroy()
		signatureBytes, err := signature.Serialize()
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}

		return blscrypto.SerializedSignatureFromBytes(signatureBytes)
	}
}

func newBackend() (b *Backend) {
	_, b = newBlockChain(4, true)

	key, _ := generatePrivateKey()
	b.Authorize(crypto.PubkeyToAddress(key.PublicKey), &key.PublicKey, decryptFn, signFn(key), signBLSFn(key))
	return
}
