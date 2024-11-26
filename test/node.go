package test

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"time"

	ethereum "github.com/celo-org/celo-blockchain"
	"github.com/celo-org/celo-blockchain/eth/ethconfig"
	"github.com/celo-org/celo-blockchain/log"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/eth"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/eth/tracers"
	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/miner"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/mycelo/genesis"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
)

var (
	allModules                  = []string{"admin", "debug", "web3", "eth", "txpool", "personal", "istanbul", "miner", "net"}
	baseNodeConfig *node.Config = &node.Config{
		Name:    "celo",
		Version: params.Version,
		P2P: p2p.Config{
			MaxPeers:              100,
			NoDiscovery:           true,
			ListenAddr:            "0.0.0.0:0",
			InboundThrottleTime:   200 * time.Millisecond,
			DialHistoryExpiration: 210 * time.Millisecond,
		},
		NoUSB: true,
		// It is important that HTTPHost and WSHost remain the same. This
		// ensures that only one server is started up on the HTTPHost. That one
		// server can still handle both http and ws connectons.
		HTTPHost:             "0.0.0.0",
		WSHost:               "0.0.0.0",
		UsePlaintextKeystore: true,
		WSModules:            allModules,
		HTTPModules:          allModules,
	}

	BaseEthConfig = &eth.Config{
		SyncMode:              downloader.FullSync, // TODO(Alec) do we need to test different sync modes?
		MinSyncPeers:          1,
		DatabaseCache:         256,
		DatabaseHandles:       256,
		TxPool:                core.DefaultTxPoolConfig,
		RPCEthCompatibility:   true,
		RPCGasPriceMultiplier: big.NewInt(100),
		Istanbul: istanbul.Config{
			Validator: true,
			// Set announce gossip period to 1 minute, if not set this results
			// in a panic when trying to set a timer with a zero value. We
			// don't actually rely on this mechanism in the tests because we
			// pre share all the enode certificates.
			AnnounceQueryEnodeGossipPeriod: 60,

			// 50ms is a really low timeout, we set this here because nodes
			// fail the first round of consensus and so setting this higher
			// makes tests run slower.
			RequestTimeout:              200,
			TimeoutBackoffFactor:        200,
			MinResendRoundChangeTimeout: 200,
			MaxResendRoundChangeTimeout: 10000,
			Epoch:                       20,
			ProposerPolicy:              istanbul.ShuffledRoundRobin,
			DefaultLookbackWindow:       3,
			BlockPeriod:                 0,
		},
		Miner: miner.Config{
			FeeCurrencyDefault: 0.9,
		},
	}
)

// Node provides an enhanced interface to node.Node with useful additions, the
// *node.Node is embedded so that its api is available through Node.
type Node struct {
	*node.Node
	Config        *node.Config
	P2PListenAddr string
	Enode         *enode.Node
	Eth           *eth.Ethereum
	EthConfig     *eth.Config
	WsClient      *ethclient.Client
	Key           *ecdsa.PrivateKey
	Address       common.Address
	Tracker       *Tracker
	// The transactions that this node has sent.
	SentTxs []*types.Transaction
}

// NewNonValidatorNode creates a new running non-validator node with the provided config.
func NewNonValidatorNode(
	nc *node.Config,
	ec *eth.Config,
	genesis *core.Genesis,
	syncmode downloader.SyncMode,
) (*Node, error) {

	// Copy the node config so we can modify it without damaging the original
	ncCopy := *nc

	// p2p key and address, this is not the same as the validator key.
	p2pKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	ncCopy.P2P.PrivateKey = p2pKey

	// Make temp datadir
	datadir, err := ioutil.TempDir("", "celo_datadir")
	if err != nil {
		return nil, err
	}
	ncCopy.DataDir = datadir

	// copy the base eth config, so we can modify it without damaging the
	// original.
	ecCopy := &eth.Config{}
	err = copyObject(ec, ecCopy)
	if err != nil {
		return nil, err
	}
	ecCopy.Genesis = genesis
	ecCopy.NetworkId = genesis.Config.ChainID.Uint64()
	ecCopy.Istanbul.Validator = false
	ecCopy.SyncMode = syncmode

	// We set these values here to avoid a panic in the eth service when these values are unset during fast sync
	if syncmode == downloader.FastSync {
		ecCopy.TrieCleanCache = 5
		ecCopy.TrieDirtyCache = 5
		ecCopy.SnapshotCache = 5
	}

	node := &Node{
		Config:    &ncCopy,
		EthConfig: ecCopy,
		Tracker:   NewTracker(),
	}

	return node, node.Start()
}

// NewNode creates a new running validator node with the provided config.
func NewNode(
	validatorAccount *env.Account,
	nc *node.Config,
	ec *eth.Config,
	genesis *core.Genesis,
) (*Node, error) {

	// Copy the node config so we can modify it without damaging the original
	ncCopy := *nc

	// p2p key and address, this is not the same as the validator key.
	p2pKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	ncCopy.P2P.PrivateKey = p2pKey

	// Make temp datadir
	datadir, err := ioutil.TempDir("", "celo_datadir")
	if err != nil {
		return nil, err
	}
	ncCopy.DataDir = datadir

	// copy the base eth config, so we can modify it without damaging the
	// original.
	ecCopy := &eth.Config{}
	err = copyObject(ec, ecCopy)
	if err != nil {
		return nil, err
	}
	ecCopy.Genesis = genesis
	ecCopy.NetworkId = genesis.Config.ChainID.Uint64()
	ecCopy.Miner.Validator = validatorAccount.Address
	ecCopy.TxFeeRecipient = validatorAccount.Address

	node := &Node{
		Config:    &ncCopy,
		EthConfig: ecCopy,
		Key:       validatorAccount.PrivateKey,
		Address:   validatorAccount.Address,
		Tracker:   NewTracker(),
	}

	return node, node.Start()
}

// Start creates the node.Node and eth.Ethereum and starts the node.Node and
// starts eth.Ethereum mining.
func (n *Node) Start() error {
	// Provide a copy of the config to node.New, so that we can rely on
	// Node.Config field not being manipulated by node and hence use our copy
	// for black box testing.
	nodeConfigCopy := &node.Config{}
	err := copyNodeConfig(n.Config, nodeConfigCopy)
	if err != nil {
		return err
	}

	n.Node, err = node.New(nodeConfigCopy)
	if err != nil {
		return err
	}

	// This registers the ethereum service on n.Node, so that calling
	// n.Node.Stop will also close the eth service. Again we provide a copy of
	// the EthConfig so that we can use our copy for black box testing.
	ethConfigCopy := &eth.Config{}
	err = copyObject(n.EthConfig, ethConfigCopy)
	if err != nil {
		return err
	}

	// Register eth service
	n.Eth, err = eth.New(n.Node, ethConfigCopy)
	if err != nil {
		return err
	}
	// This manual step is required to enable tracing, it's messy but this is the
	// approach taken by geth in cmd/utils.RegisterEthService.
	n.Node.RegisterAPIs(tracers.APIs(n.Eth.APIBackend))

	err = n.Node.Start()
	if err != nil {
		return err
	}

	// The ListenAddr is set at p2p server startup, save it here.
	n.P2PListenAddr = n.Node.Server().ListenAddr

	if n.EthConfig.Istanbul.Validator {
		// Import the node key into the keystore and then unlock it, the keystore
		// is the interface used for signing operations so the node key needs to be
		// inside it.
		ks, _, err := n.Config.GetKeyStore()
		if err != nil {
			return err
		}
		n.AccountManager().AddBackend(ks)
		account, err := ks.ImportECDSA(n.Key, "")
		if err != nil {
			return err
		}
		err = ks.TimedUnlock(account, "", 0)
		if err != nil {
			return err
		}
	}

	_, _, err = core.SetupGenesisBlock(n.Eth.ChainDb(), n.EthConfig.Genesis)
	if err != nil {
		return err
	}
	n.WsClient, err = ethclient.Dial(n.WSEndpoint())
	if err != nil {
		return err
	}
	err = n.Tracker.StartTracking(n.WsClient)
	if err != nil {
		return err
	}

	if n.EthConfig.Istanbul.Validator {
		err = n.Eth.StartMining()
		if err != nil {
			return err
		}
	}

	// Note we need to use the LocalNode from the p2p server because that is
	// what is also used by the announce protocol when building enode
	// certificates, so that is what is used by the announce protocol to check
	// if a validator enode has changed. If we constructed the enode ourselves
	// here we could not be sure it matches the enode from the p2p.Server.
	n.Enode = n.Server().LocalNode().Node()

	return nil
}

func (n *Node) AddPeers(nodes ...*Node) {
	// Add the given nodes as peers. Although this means that nodes can reach
	// each other nodes don't start sending consensus messages to another node
	// until they have received an enode certificate from that node.
	for _, no := range nodes {
		n.Server().AddPeer(no.Enode, p2p.ValidatorPurpose)
	}
}

// GossipEnodeCertificatge gossips this nodes enode certificates to the rest of
// the network.
func (n *Node) GossipEnodeCertificatge() error {
	enodeCertificate := &istanbul.EnodeCertificate{
		EnodeURL: n.Enode.URLv4(),
		Version:  uint(time.Now().Unix()),
	}
	enodeCertificateBytes, err := rlp.EncodeToBytes(enodeCertificate)
	if err != nil {
		return err
	}
	msg := &istanbul.Message{
		Code:    istanbul.EnodeCertificateMsg,
		Address: n.Address,
		Msg:     enodeCertificateBytes,
	}
	b := n.Eth.Engine().(*backend.Backend)
	if err := msg.Sign(b.Sign); err != nil {
		return err
	}
	payload, err := msg.Payload()
	if err != nil {
		return err
	}
	// Share enode certificates to the other nodes, nodes wont consider other
	// nodes valid validators without seeing an enode certificate message from
	// them.
	return b.Gossip(payload, istanbul.EnodeCertificateMsg)
}

// Close shuts down the node and releases all resources and removes the datadir
// unless an error is returned, in which case there is no guarantee that all
// resources are released. It is assumed that this is only called after calling
// Start otherwise it will panic.
func (n *Node) Close() error {
	err := n.Tracker.StopTracking()
	if err != nil {
		return err
	}
	n.WsClient.Close()
	err = n.Node.Close() // This also shuts down the Eth service
	if err != nil {
		return err
	}
	return os.RemoveAll(n.Config.DataDir)
}

// AwaitTransactions awaits all the provided transactions.
func (n *Node) AwaitTransactions(ctx context.Context, txs ...*types.Transaction) error {
	sentHashes := make([]common.Hash, len(txs))
	for i, tx := range txs {
		sentHashes[i] = tx.Hash()
	}
	return n.Tracker.AwaitTransactions(ctx, sentHashes)
}

// ProcessedTxBlock returns the block that the given transaction was processed
// in, nil will be retuned if the transaction has not been processed by this node.
func (n *Node) ProcessedTxBlock(tx *types.Transaction) *types.Block {
	return n.Tracker.GetProcessedBlockForTx(tx.Hash())
}

// TxFee returns the gas fee for the given transaction.
func (n *Node) TxFee(ctx context.Context, tx *types.Transaction) (*big.Int, error) {
	r, err := n.WsClient.TransactionReceipt(ctx, tx.Hash())
	if err != nil {
		return nil, err
	}
	return big.NewInt(0).Mul(new(big.Int).SetUint64(r.GasUsed), tx.GasPrice()), nil
}

// Network represents a network of nodes and provides functionality to easily
// create, start and stop a collection of nodes.
type Network []*Node

func AccountConfig(numValidators, numExternal int) *env.AccountsConfig {
	return &env.AccountsConfig{
		Mnemonic:             env.MustNewMnemonic(),
		NumValidators:        numValidators,
		ValidatorsPerGroup:   1,
		NumDeveloperAccounts: numExternal,
	}
}

// BuildConfig generates genesis and eth config instances, that can be modified
// before passing to NewNetwork or NewNode.
//
// NOTE: Do not edit the Istanbul field of the returned genesis config it will
// be overwritten with the corresponding config from the Istanbul field of the
// returned eth config.
func BuildConfig(accounts *env.AccountsConfig, gingerbreadBlock, l2MigrationBlock *big.Int) (*genesis.Config, *ethconfig.Config, error) {
	gc, err := genesis.CreateCommonGenesisConfig(
		big.NewInt(1),
		accounts.AdminAccount().Address,
		params.IstanbulConfig{},
		gingerbreadBlock,
	)
	if err != nil {
		return nil, nil, err
	}

	genesis.FundAccounts(gc, accounts.DeveloperAccounts())

	// copy the base eth config, so we can modify it without damaging the
	// original.
	ec := &eth.Config{}
	err = copyObject(BaseEthConfig, ec)
	ec.L2MigrationBlock = l2MigrationBlock
	return gc, ec, err
}

// GenerateGenesis checks that the contractsBuildPath exists and if so proceeds to generate the genesis.
func GenerateGenesis(accounts *env.AccountsConfig, gc *genesis.Config, contractsBuildPath string) (*core.Genesis, error) {
	// Check for the existence of the compiled-system-contracts dir
	_, err := os.Stat(contractsBuildPath)
	if errors.Is(err, os.ErrNotExist) {
		abs, err := filepath.Abs(contractsBuildPath)
		if err != nil {
			panic(fmt.Sprintf("failed to get abs path for %s, error: %v", contractsBuildPath, err))
		}
		return nil, fmt.Errorf("Could not find dir %s, try running 'make prepare-system-contracts' and then re-running the test", abs)

	}
	return genesis.GenerateGenesis(accounts, gc, contractsBuildPath)
}

// NewNetwork generates a network of nodes that are running and mining. For
// each provided validator account a corresponding node is created and each
// node is also assigned a developer account, there must be at least as many
// developer accounts provided as validator accounts. A shutdown function is
// also returned which will shutdown and clean up the network when called.  In
// the case that an error is returned the shutdown function will be nil and so
// no attempt should be made to call it.
func NewNetwork(accounts *env.AccountsConfig, gc *genesis.Config, ec *eth.Config) (Network, func(), error) {

	// Copy eth istanbul config fields to the genesis istanbul config.
	// There is a ticket to remove this duplication of config.
	// https://github.com/celo-org/celo-blockchain/issues/1693
	gc.Istanbul = params.IstanbulConfig{
		Epoch:          ec.Istanbul.Epoch,
		ProposerPolicy: uint64(ec.Istanbul.ProposerPolicy),
		LookbackWindow: ec.Istanbul.DefaultLookbackWindow,
		BlockPeriod:    ec.Istanbul.BlockPeriod,
		RequestTimeout: ec.Istanbul.RequestTimeout,
	}

	genesis, err := GenerateGenesis(accounts, gc, "../compiled-system-contracts")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate genesis: %v", err)
	}

	va := accounts.ValidatorAccounts()
	var network Network = make([]*Node, len(va))

	for i := range va {
		n, err := NewNode(&va[i], baseNodeConfig, ec, genesis)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build node for network: %v", err)
		}
		network[i] = n
	}

	// Connect nodes to each other, although this means that nodes can reach
	// each other nodes don't start sending consensus messages to another node
	// until they have received an enode certificate from that node.
	for i := range network {
		network[i].AddPeers(network[i+1:]...)
	}

	// Give nodes some time to connect. Also there is a race condition in
	// miner.worker its field snapshotBlock is set only when new transactions
	// are received or commitNewWork is called. But both of these happen in
	// goroutines separate to the call to miner.Start and miner.Start does not
	// wait for snapshotBlock to be set. Therefore there is currently no way to
	// know when it is safe to call estimate gas.  What we do here is sleep a
	// bit and cross our fingers.
	time.Sleep(25 * time.Millisecond)

	for i := range network {
		err := network[i].GossipEnodeCertificatge()
		if err != nil {
			network.Shutdown()
			return nil, nil, err
		}
	}

	shutdown := func() {
		log.Info("Shutting down network from test")
		for _, err := range network.Shutdown() {
			fmt.Println(err.Error())
		}
	}

	return network, shutdown, nil
}

// AddNonValidatorNodes Adds non-validator nodes to the network with the specified sync mode.
func AddNonValidatorNodes(network Network, ec *eth.Config, numNodes uint64, syncmode downloader.SyncMode) (Network, func(), error) {
	var nodes Network = make([]*Node, numNodes)
	genesis := network[0].EthConfig.Genesis

	for i := uint64(0); i < numNodes; i++ {
		n, err := NewNonValidatorNode(baseNodeConfig, ec, genesis, syncmode)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build full node for network: %v", err)
		}
		nodes[i] = n
	}

	// Connect nodes to each other
	for i := range nodes {
		nodes[i].AddPeers(network...)
		nodes[i].AddPeers(nodes[i+1:]...)
	}

	network = append(network, nodes...)

	// Give nodes some time to connect. Also there is a race condition in
	// miner.worker its field snapshotBlock is set only when new transactions
	// are received or commitNewWork is called. But both of these happen in
	// goroutines separate to the call to miner.Start and miner.Start does not
	// wait for snapshotBlock to be set. Therefore there is currently no way to
	// know when it is safe to call estimate gas.  What we do here is sleep a
	// bit and cross our fingers.
	time.Sleep(25 * time.Millisecond)

	shutdown := func() {
		log.Info("Shutting down network from test")
		for _, err := range network.Shutdown() {
			fmt.Println(err.Error())
		}
	}

	return network, shutdown, nil
}

// AwaitTransactions ensures that the entire network has processed the provided transactions.
func (n Network) AwaitTransactions(ctx context.Context, txs ...*types.Transaction) error {
	for _, node := range n {
		err := node.AwaitTransactions(ctx, txs...)
		if err != nil {
			return err
		}
	}
	return nil
}

// AwaitBlock ensures that the entire network has processed a block with the given num.
func (n Network) AwaitBlock(ctx context.Context, num uint64) error {
	for _, node := range n {
		err := node.Tracker.AwaitBlock(ctx, num)
		if err != nil {
			return err
		}
	}
	return nil
}

// Shutdown closes all nodes in the network, any errors encountered when
// shutting down nodes are returned in a slice.
func (n Network) Shutdown() []error {
	var errors []error
	for i, node := range n {
		if node != nil {
			err := node.Close()
			if err != nil {
				errors = append(errors, fmt.Errorf("error shutting down node %v index %d: %w", node.Address.String(), i, err))
			}
		}
	}
	return errors
}

func (n Network) RestartNetworkWithMigrationBlockOffsets(l2MigrationBlockOG *big.Int, offsets []int64) error {
	if len(offsets) != len(n) {
		return fmt.Errorf("number of l2BlockMigration offsets must match number of nodes")
	}

	errors := []error{}
	for i, node := range n {
		node.EthConfig.L2MigrationBlock = new(big.Int).Add(l2MigrationBlockOG, big.NewInt(offsets[i]))
		err := node.Start()
		if err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("failed to restart network: %v", errors)
	}

	for i, node := range n {
		node.AddPeers(n[:i]...)
	}

	// We need to wait here to allow the call to "Backend.RefreshValPeers" to
	// complete before adding peers. This is because "Backend.RefreshValPeers"
	// deletes all peers and then re-adds any peers from the cached
	// connections, but in the case that peers were recently added there may
	// not have been enough time to connect to them and populate the connection
	// cache, and in that case "Backend.RefreshValPeers" simply removes all the
	// peers.
	time.Sleep(25 * time.Millisecond)

	return nil
}

// Uses the client to suggest a gas price and to estimate the gas.
func BuildSignedTransaction(
	client *ethclient.Client,
	senderKey *ecdsa.PrivateKey,
	sender,
	recipient common.Address,
	nonce uint64,
	value *big.Int,
	signer types.Signer,
	data []byte,
) (*types.Transaction, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	// Figure out the gas allowance and gas price values
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to suggest gas price: %v", err)
	}

	msg := ethereum.CallMsg{From: sender, To: &recipient, GasPrice: gasPrice, Value: value, Data: data}
	gasLimit, err := client.EstimateGas(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate gas needed: %v", err)
	}
	// Create the transaction and sign it
	rawTx := types.NewTransaction(nonce, recipient, value, gasLimit, gasPrice, data)
	signed, err := types.SignTx(rawTx, signer, senderKey)
	if err != nil {
		return nil, err
	}
	return signed, nil
}

// ValueTransferTransactionWithDynamicFee builds a signed value transfer transaction
// from the sender to the recipient with the given value, nonce, gasFeeCap and gasTipCap.
func ValueTransferTransactionWithDynamicFee(
	client *ethclient.Client,
	senderKey *ecdsa.PrivateKey,
	sender,
	recipient common.Address,
	nonce uint64,
	value *big.Int,
	feeCurrency *common.Address,
	gasFeeCap *big.Int,
	gasTipCap *big.Int,
	signer types.Signer,
	gasLimit uint64,
) (*types.Transaction, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	if gasLimit == 0 {
		msg := ethereum.CallMsg{From: sender, To: &recipient, Value: value, FeeCurrency: feeCurrency}
		var err error
		gasLimit, err = client.EstimateGas(ctx, msg)
		if err != nil {
			return nil, fmt.Errorf("failed to estimate gas needed: %v", err)
		}
	}
	// Create the transaction and sign it
	rawTx := types.NewTx(&types.CeloDynamicFeeTx{
		Nonce:       nonce,
		To:          &recipient,
		Value:       value,
		Gas:         gasLimit,
		FeeCurrency: feeCurrency,
		GasFeeCap:   gasFeeCap,
		GasTipCap:   gasTipCap,
	})
	signed, err := types.SignTx(rawTx, signer, senderKey)
	if err != nil {
		return nil, err
	}
	return signed, nil
}

// Since the node config is not marshalable by default we construct a
// marshalable struct which we marshal and unmarshal and then unpack into the
// original struct type.
func copyNodeConfig(source, dest *node.Config) error {
	s := &MarshalableNodeConfig{}
	s.Config = *source
	p := MarshalableP2PConfig{}
	p.Config = source.P2P

	p.PrivateKey = (*MarshalableECDSAPrivateKey)(source.P2P.PrivateKey)
	s.P2P = p
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	u := new(MarshalableNodeConfig)
	err = json.Unmarshal(data, u)
	if err != nil {
		return err
	}
	*dest = u.Config
	dest.P2P = u.P2P.Config
	dest.P2P.PrivateKey = (*ecdsa.PrivateKey)(u.P2P.PrivateKey)
	return nil
}

type MarshalableNodeConfig struct {
	node.Config
	P2P MarshalableP2PConfig
}

type MarshalableP2PConfig struct {
	p2p.Config
	PrivateKey *MarshalableECDSAPrivateKey
}

type MarshalableECDSAPrivateKey ecdsa.PrivateKey

func (k *MarshalableECDSAPrivateKey) UnmarshalJSON(b []byte) error {
	key, err := crypto.PrivECDSAFromHex(b[1 : len(b)-1])
	if err != nil {
		return err
	}
	*k = MarshalableECDSAPrivateKey(*key)
	return nil
}

func (k *MarshalableECDSAPrivateKey) MarshalJSON() ([]byte, error) {
	return []byte(`"` + hex.EncodeToString(crypto.FromECDSA((*ecdsa.PrivateKey)(k))) + `"`), nil
}

// copyObject copies an object so that the copy shares no memory with the
// original.
func copyObject(source, dest interface{}) error {
	data, err := json.Marshal(source)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}
