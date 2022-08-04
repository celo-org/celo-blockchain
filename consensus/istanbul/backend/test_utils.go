package backend

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/celo-org/celo-blockchain/accounts"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/backendtest"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/crypto/ecies"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/celo-org/celo-bls-go/bls"
)

// in this test, we can set n to 1, and it means we can process Istanbul and commit a
// block by one node. Otherwise, if n is larger than 1, we have to generate
// other fake events to process Istanbul.
func newBlockChain(n int, isFullChain bool) (*core.BlockChain, *Backend) {
	genesis, nodeKeys := getGenesisAndKeys(n, isFullChain)

	bc, be, _ := newBlockChainWithKeys(false, common.Address{}, false, genesis, nodeKeys[0])
	return bc, be
}

func newBlockChainWithKeys(isProxy bool, proxiedValAddress common.Address, isProxied bool, genesis *core.Genesis, privateKey *ecdsa.PrivateKey) (*core.BlockChain, *Backend, *istanbul.Config) {
	memDB := rawdb.NewMemoryDatabase()
	config := *istanbul.DefaultConfig
	config.ReplicaStateDBPath = ""
	config.ValidatorEnodeDBPath = ""
	config.VersionCertificateDBPath = ""
	config.RoundStateDBPath = ""
	config.Proxy = isProxy
	config.ProxiedValidatorAddress = proxiedValAddress
	config.Proxied = isProxied
	config.Validator = !isProxy
	istanbul.ApplyParamsChainConfigToConfig(genesis.Config, &config)

	b, _ := New(&config, memDB).(*Backend)

	var publicKey ecdsa.PublicKey
	if !isProxy {
		publicKey = privateKey.PublicKey
		address := crypto.PubkeyToAddress(publicKey)
		decryptFn := DecryptFn(privateKey)
		signerFn := SignFn(privateKey)
		signerBLSFn := SignBLSFn(privateKey)
		signerHashFn := SignHashFn(privateKey)
		b.Authorize(address, address, &publicKey, decryptFn, signerFn, signerBLSFn, signerHashFn)
	} else {
		proxyNodeKey, _ := crypto.GenerateKey()
		publicKey = proxyNodeKey.PublicKey
	}

	genesis.MustCommit(memDB)

	blockchain, err := core.NewBlockChain(memDB, nil, genesis.Config, b, vm.Config{}, nil, nil)
	if err != nil {
		panic(err)
	}

	b.SetChain(
		blockchain,
		blockchain.CurrentBlock,
		func(hash common.Hash) (*state.StateDB, error) {
			stateRoot := blockchain.GetHeaderByHash(hash).Root
			return blockchain.StateAt(stateRoot)
		},
	)
	b.SetBroadcaster(&consensustest.MockBroadcaster{})
	b.SetP2PServer(consensustest.NewMockP2PServer(&publicKey))
	b.StartAnnouncing()

	if !isProxy {
		b.SetCallBacks(blockchain.HasBadBlock,
			func(block *types.Block, state *state.StateDB) (types.Receipts, []*types.Log, uint64, error) {
				return blockchain.Processor().Process(block, state, *blockchain.GetVMConfig())
			},
			blockchain.Validator().ValidateState,
			func(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB) {
				if err := blockchain.InsertPreprocessedBlock(block, receipts, logs, state); err != nil {
					panic(fmt.Sprintf("could not InsertPreprocessedBlock: %v", err))
				}
			})
		if isProxied {
			b.StartProxiedValidatorEngine()
		}
		b.StartValidating()
	}

	return blockchain, b, &config
}

func getGenesisAndKeys(n int, isFullChain bool) (*core.Genesis, []*ecdsa.PrivateKey) {
	// Setup validators
	var nodeKeys = make([]*ecdsa.PrivateKey, n)
	validators := make([]istanbul.ValidatorData, n)
	for i := 0; i < n; i++ {
		var addr common.Address
		if i == 0 {
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
	genesis := core.MainnetGenesisBlock()
	genesis.Config = params.IstanbulTestChainConfig
	if !isFullChain {
		genesis.Config.FullHeaderChainAvailable = false
	}
	// force enable Istanbul engine
	genesis.Config.Istanbul = &params.IstanbulConfig{
		Epoch:          10,
		LookbackWindow: 3,
	}

	AppendValidatorsToGenesisBlock(genesis, validators)
	return genesis, nodeKeys
}

func AppendValidatorsToGenesisBlock(genesis *core.Genesis, validators []istanbul.ValidatorData) {
	if len(genesis.ExtraData) < types.IstanbulExtraVanity {
		genesis.ExtraData = append(genesis.ExtraData, bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity)...)
	}
	genesis.ExtraData = genesis.ExtraData[:types.IstanbulExtraVanity]

	var addrs []common.Address
	var publicKeys []blscrypto.SerializedPublicKey

	for i := range validators {
		if (validators[i].BLSPublicKey == blscrypto.SerializedPublicKey{}) {
			panic("BLSPublicKey is nil")
		}
		addrs = append(addrs, validators[i].Address)
		publicKeys = append(publicKeys, validators[i].BLSPublicKey)
	}

	ist := &types.IstanbulExtra{
		AddedValidators:           addrs,
		AddedValidatorsPublicKeys: publicKeys,
		Seal:                      []byte{},
		AggregatedSeal:            types.IstanbulAggregatedSeal{},
		ParentAggregatedSeal:      types.IstanbulAggregatedSeal{},
	}

	istPayload, err := rlp.EncodeToBytes(&ist)
	if err != nil {
		panic("failed to encode istanbul extra")
	}
	genesis.ExtraData = append(genesis.ExtraData, istPayload...)
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

func makeBlock(keys []*ecdsa.PrivateKey, chain *core.BlockChain, engine *Backend, parent *types.Block) (*types.Block, error) {
	block := makeBlockWithoutSeal(chain, engine, parent)

	// Set up block subscription
	chainHeadCh := make(chan core.ChainHeadEvent, 10)
	sub := chain.SubscribeChainHeadEvent(chainHeadCh)
	defer sub.Unsubscribe()

	// start seal request (this is non-blocking)
	err := engine.Seal(chain, block)
	if err != nil {
		return nil, err
	}

	// Wait for and then save the mined block.
	select {
	case ev := <-chainHeadCh:
		block = ev.Block
	case <-time.After(6 * time.Second):
		return nil, errors.New("Timed out when making a block")
	}

	// Notify the core engine to stop working on current Seal.
	go engine.istanbulEventMux.Post(istanbul.FinalCommittedEvent{})

	return block, nil
}

func makeBlockWithoutSeal(chain *core.BlockChain, engine *Backend, parent *types.Block) *types.Block {
	header := makeHeader(parent, engine.config)
	// The worker that calls Prepare is the one filling the Coinbase
	header.Coinbase = engine.wallets().Ecdsa.Address
	engine.Prepare(chain, header)
	time.Sleep(time.Until(time.Unix(int64(header.Time), 0)))

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

func DecryptFn(key *ecdsa.PrivateKey) istanbul.DecryptFn {
	if key == nil {
		key, _ = generatePrivateKey()
	}

	return func(_ accounts.Account, c, s1, s2 []byte) ([]byte, error) {
		eciesKey := ecies.ImportECDSA(key)
		return eciesKey.Decrypt(c, s1, s2)
	}
}

func SignFn(key *ecdsa.PrivateKey) istanbul.SignerFn {
	if key == nil {
		key, _ = generatePrivateKey()
	}

	return func(_ accounts.Account, mimeType string, data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), key)
	}
}

func SignBLSFn(key *ecdsa.PrivateKey) istanbul.BLSSignerFn {
	if key == nil {
		key, _ = generatePrivateKey()
	}

	return func(_ accounts.Account, data []byte, extraData []byte, useComposite, cip22 bool) (blscrypto.SerializedSignature, error) {
		privateKeyBytes, err := blscrypto.ECDSAToBLS(key)
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}

		privateKey, err := bls.DeserializePrivateKey(privateKeyBytes)
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}
		defer privateKey.Destroy()

		signature, err := privateKey.SignMessage(data, extraData, useComposite, cip22)
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

func signerFn(key *ecdsa.PrivateKey) func(data, extraData []byte, compositeHasher, cip22 bool) ([]byte, error) {
	return func(data []byte, extraData []byte, useComposite, cip22 bool) ([]byte, error) {
		sig, err := SignBLSFn(key)(accounts.Account{}, data, extraData, useComposite, cip22)
		return sig[:], err
	}
}

func SignHashFn(key *ecdsa.PrivateKey) istanbul.HashSignerFn {
	if key == nil {
		key, _ = generatePrivateKey()
	}

	return func(_ accounts.Account, data []byte) ([]byte, error) {
		return crypto.Sign(data, key)
	}
}

/*<<<<<<< HEAD
// this will return an aggregate sig by the BLS keys corresponding to the `keys` array over the
// block's hash, on consensus round 0, without a composite hasher
func signBlock(keys []*ecdsa.PrivateKey, block *types.Block) types.IstanbulAggregatedSeal {
	round := big.NewInt(0)
	headerHash := block.Header().Hash()
	signatures := make([][]byte, len(keys))

	msg := istanbulCore.NewCommitSeal(headerHash, round)

	for i, key := range keys {
		sig, err := msg.Sign(signerFn(key))
		if err != nil {
			panic("could not sign msg")
		}
		signatures[i] = sig
	}

	asigBytes, err := blscrypto.AggregateSignatures(signatures)
	if err != nil {
		panic("could not aggregate sigs")
	}

	// create the bitmap from the validators
	bitmap := big.NewInt(0)
	for i := 0; i < len(keys); i++ {
		bitmap.SetBit(bitmap, i, 1)
	}

	asig := types.IstanbulAggregatedSeal{
		Bitmap:    bitmap,
		Round:     round,
		Signature: asigBytes,
	}

	return asig
}

// this will return an aggregate sig by the BLS keys corresponding to the `keys` array over
// an abtirary message
func signEpochSnarkData(keys []*ecdsa.PrivateKey, message, extraData []byte) types.IstanbulEpochValidatorSetSeal {

	msg := &istanbulCore.BLSSeal{
		Seal:            message,
		ExtraData:       extraData,
		CompositeHasher: true,
		Cip22:           true,
	}

	signatures := make([][]byte, len(keys))
	for i, key := range keys {
		sig, err := msg.Sign(signerFn(key))
		if err != nil {
			panic("could not sign msg")
		}
		signatures[i] = sig[:]
	}

	asigBytes, err := blscrypto.AggregateSignatures(signatures)
	if err != nil {
		panic("could not aggregate sigs")
	}

	// create the bitmap from the validators
	bitmap := big.NewInt(0)
	for i := 0; i < len(keys); i++ {
		bitmap.SetBit(bitmap, i, 1)
	}

	asig := types.IstanbulEpochValidatorSetSeal{
		Bitmap:    bitmap,
		Signature: asigBytes,
	}

	return asig
}

=======
>>>>>>> master*/
func newBackend() (b *Backend) {
	_, b = newBlockChain(4, true)

	key, _ := generatePrivateKey()
	address := crypto.PubkeyToAddress(key.PublicKey)
	b.Authorize(address, address, &key.PublicKey, DecryptFn(key), SignFn(key), SignBLSFn(key), SignHashFn(key))
	return
}

type testBackendFactoryImpl struct{}

// TestBackendFactory can be passed to backendtest.InitTestBackendFactory
var TestBackendFactory backendtest.TestBackendFactory = testBackendFactoryImpl{}

// New is part of TestBackendInterface.
func (testBackendFactoryImpl) New(isProxy bool, proxiedValAddress common.Address, isProxied bool, genesisCfg *core.Genesis, privateKey *ecdsa.PrivateKey) (backendtest.TestBackendInterface, *istanbul.Config) {
	_, be, config := newBlockChainWithKeys(isProxy, proxiedValAddress, isProxied, genesisCfg, privateKey)
	return be, config
}

// GetGenesisAndKeys is part of TestBackendInterface
func (testBackendFactoryImpl) GetGenesisAndKeys(numValidators int, isFullChain bool) (*core.Genesis, []*ecdsa.PrivateKey) {
	return getGenesisAndKeys(numValidators, isFullChain)
}
