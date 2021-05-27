package backend

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/celo-org/celo-blockchain/accounts"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/backendtest"
	istanbulCore "github.com/celo-org/celo-blockchain/consensus/istanbul/core"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/contract_comm"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/core/vm/vmcontext"
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
func newBlockChain(n int) *testNode {
	genesis, nodeKeys := generateGenesisAndKeys(n)

	return testNodeFromGenesis(Validator, common.ZeroAddress, genesis, nodeKeys[0])
}

type nodeType int

const (
	Validator nodeType = iota
	ProxiedValidator
	Proxy
)

type testNode struct {
	chain    *core.BlockChain
	engine   *Backend
	config   istanbul.Config
	nodeType nodeType
}

func (tn *testNode) start() {
	tn.engine.StartAnnouncing()
	if tn.nodeType == ProxiedValidator {
		tn.engine.StartProxiedValidatorEngine()
	}
	if tn.nodeType == ProxiedValidator || tn.nodeType == Validator {
		tn.engine.StartValidating()
	}
}

func (tn *testNode) startAndStop() func() {
	tn.start()
	return tn.stop
}

func (tn *testNode) stop() {
	tn.engine.StopAnnouncing()
	if tn.nodeType == ProxiedValidator {
		tn.engine.StopProxiedValidatorEngine()
	}
	if tn.nodeType == ProxiedValidator || tn.nodeType == Validator {
		tn.engine.StopValidating()
	}
}

// --------------------------------------------------------------------------------------
// Test Config Scenarios
// --------------------------------------------------------------------------------------

type testConfigBuilder func() (*core.Genesis, []*ecdsa.PrivateKey)

func manyValidators(qty int) testConfigBuilder {
	return func() (*core.Genesis, []*ecdsa.PrivateKey) { return generateGenesisAndKeys(qty) }
}

var singleValidator testConfigBuilder = manyValidators(1)

func lightestNode() (*core.Genesis, []*ecdsa.PrivateKey) {
	genesis, keys := singleValidator()
	genesis.Config.FullHeaderChainAvailable = false
	return genesis, keys
}

// --------------------------------------------------------------------------------------

func withEngine(
	testConfigBuilder testConfigBuilder,
	startEngine bool,
	testFn func(n *testNode, keys []*ecdsa.PrivateKey),
) {
	genesis, nodeKeys := testConfigBuilder()

	node := testNodeFromGenesis(Validator, common.ZeroAddress, genesis, nodeKeys[0])

	// if required start engine (announce, validator, proxy) and make sure we stop it
	if startEngine {
		node.start()
		defer node.stop()
	}
	testFn(node, nodeKeys)
}

func withManyEngines(
	testConfigBuilder testConfigBuilder,
	startEngine bool,
	testFn func(n []testNode, keys []*ecdsa.PrivateKey),
) {
	genesis, nodeKeys := testConfigBuilder()

	testNodes := make([]testNode, len(nodeKeys))
	for i, key := range nodeKeys {
		testNodes[i] = *testNodeFromGenesis(Validator, common.ZeroAddress, genesis, key)
	}

	// if required start engine (announce, validator, proxy) and make sure we stop it
	if startEngine {
		for _, node := range testNodes {
			node.start()
			defer node.stop()
		}
	}

	testFn(testNodes, nodeKeys)
}

// testNodeFromGenesis creates a testNode from a genesis & privateKey
func testNodeFromGenesis(nodeType nodeType, proxiedValAddress common.Address, genesis *core.Genesis, privateKey *ecdsa.PrivateKey) *testNode {
	memDB := rawdb.NewMemoryDatabase()
	config := *istanbul.DefaultConfig
	config.ReplicaStateDBPath = ""
	config.ValidatorEnodeDBPath = ""
	config.VersionCertificateDBPath = ""
	config.RoundStateDBPath = ""
	config.Proxy = nodeType == Proxy
	config.ProxiedValidatorAddress = proxiedValAddress
	config.Proxied = nodeType == ProxiedValidator
	config.Validator = nodeType == ProxiedValidator || nodeType == Validator
	istanbul.ApplyParamsChainConfigToConfig(genesis.Config, &config)

	engine, _ := New(&config, memDB).(*Backend)

	var publicKey ecdsa.PublicKey
	if nodeType == ProxiedValidator || nodeType == Validator {
		publicKey = privateKey.PublicKey
		address := crypto.PubkeyToAddress(publicKey)
		engine.Authorize(
			address,
			address,
			&publicKey,
			DecryptFn(privateKey),
			SignFn(privateKey),
			SignBLSFn(privateKey),
			SignHashFn(privateKey),
		)
	} else {
		proxyNodeKey, _ := crypto.GenerateKey()
		publicKey = proxyNodeKey.PublicKey
	}

	genesis.MustCommit(memDB)

	blockchain, err := core.NewBlockChain(memDB, nil, genesis.Config, engine, vm.Config{}, nil, nil)
	if err != nil {
		panic(err)
	}

	engine.SetChain(
		blockchain,
		blockchain.CurrentBlock,
		func(hash common.Hash) (*state.StateDB, error) {
			stateRoot := blockchain.GetHeaderByHash(hash).Root
			return blockchain.StateAt(stateRoot)
		},
	)
	engine.SetBroadcaster(&consensustest.MockBroadcaster{})
	engine.SetP2PServer(consensustest.NewMockP2PServer(&publicKey))

	if nodeType == ProxiedValidator || nodeType == Validator {
		engine.SetCallBacks(blockchain.HasBadBlock,
			func(block *types.Block, state *state.StateDB) (types.Receipts, []*types.Log, uint64, error) {
				return blockchain.Processor().Process(block, state, *blockchain.GetVMConfig())
			},
			blockchain.Validator().ValidateState,
			func(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB) {
				if err := blockchain.InsertPreprocessedBlock(block, receipts, logs, state); err != nil {
					panic(fmt.Sprintf("could not InsertPreprocessedBlock: %v", err))
				}
			})
	}

	contract_comm.SetEVMRunnerFactory(vmcontext.GetSystemEVMRunnerFactory(blockchain))

	return &testNode{
		config:   config,
		chain:    blockchain,
		engine:   engine,
		nodeType: nodeType,
	}
}

func generateGenesisAndKeys(n int) (*core.Genesis, []*ecdsa.PrivateKey) {
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

	// start the sealing procedure
	results := make(chan *types.Block)

	// start seal request (this is non blocking)
	err := engine.Seal(chain, block, results, nil)
	if err != nil {
		return nil, err
	}

	// create the sig and call Commit so that the result is pushed to the channel
	block, err = engine.signBlock(block)
	if err != nil {
		return nil, err
	}
	aggregatedSeal := signBlock(keys, block)
	aggregatedEpochSnarkDataSeal := signEpochSnarkData(keys, []byte("message"), []byte("extra data"))
	err = engine.Commit(block, aggregatedSeal, aggregatedEpochSnarkDataSeal, nil)
	if err != nil {
		return nil, err
	}

	// wait for seal job to finish
	block = <-results

	// insert the block to the chain so that we can make multiple calls to this function
	_, err = chain.InsertChain(types.Blocks{block})
	if err != nil {
		return nil, err
	}

	// Notify the core engine to stop working on current Seal.
	go engine.istanbulEventMux.Post(istanbul.FinalCommittedEvent{})

	return block, nil
}

func makeBlockWithoutSeal(chain *core.BlockChain, engine *Backend, parent *types.Block) *types.Block {
	header := makeHeader(parent, engine.config)
	// The worker that calls Prepare is the one filling the Coinbase
	header.Coinbase = engine.address
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

func SignHashFn(key *ecdsa.PrivateKey) istanbul.HashSignerFn {
	if key == nil {
		key, _ = generatePrivateKey()
	}

	return func(_ accounts.Account, data []byte) ([]byte, error) {
		return crypto.Sign(data, key)
	}
}

// this will return an aggregate sig by the BLS keys corresponding to the `keys` array over the
// block's hash, on consensus round 0, without a composite hasher
func signBlock(keys []*ecdsa.PrivateKey, block *types.Block) types.IstanbulAggregatedSeal {
	extraData := []byte{}
	useComposite := false
	cip22 := false
	round := big.NewInt(0)
	headerHash := block.Header().Hash()
	signatures := make([][]byte, len(keys))

	msg := istanbulCore.PrepareCommittedSeal(headerHash, round)

	for i, key := range keys {
		signFn := SignBLSFn(key)
		sig, err := signFn(accounts.Account{}, msg, extraData, useComposite, cip22)
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
	useComposite := true
	cip22 := true
	signatures := make([][]byte, len(keys))

	for i, key := range keys {
		signFn := SignBLSFn(key)
		sig, err := signFn(accounts.Account{}, message, extraData, useComposite, cip22)
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

func newBackend() *Backend {
	testNode := newBlockChain(4)

	key, _ := generatePrivateKey()
	address := crypto.PubkeyToAddress(key.PublicKey)
	testNode.engine.Authorize(address, address, &key.PublicKey, DecryptFn(key), SignFn(key), SignBLSFn(key), SignHashFn(key))
	return testNode.engine
}

type testBackendFactoryImpl struct{}

// TestBackendFactory can be passed to backendtest.InitTestBackendFactory
var TestBackendFactory backendtest.TestBackendFactory = testBackendFactoryImpl{}

// New is part of TestBackendInterface.
func (testBackendFactoryImpl) New(isProxy bool, proxiedValAddress common.Address, isProxied bool, genesisCfg *core.Genesis, privateKey *ecdsa.PrivateKey) (backendtest.TestBackendInterface, *istanbul.Config) {
	var nodeType nodeType
	if isProxy {
		nodeType = Proxy
	} else if isProxied {
		nodeType = ProxiedValidator
	} else {
		nodeType = Validator
	}
	node := testNodeFromGenesis(nodeType, proxiedValAddress, genesisCfg, privateKey)
	node.start()
	return node.engine, &node.config
}

// GetGenesisAndKeys is part of TestBackendInterface
func (testBackendFactoryImpl) GetGenesisAndKeys(numValidators int) (*core.Genesis, []*ecdsa.PrivateKey) {
	return generateGenesisAndKeys(numValidators)
}
