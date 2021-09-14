package test

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"strconv"
	"time"

	ethereum "github.com/celo-org/celo-blockchain"

	"github.com/celo-org/celo-blockchain/accounts/keystore"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/eth"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/mycelo/genesis"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
)

var (
	baseNodeConfig *node.Config = &node.Config{
		Name:    "celo",
		Version: params.Version,
		P2P: p2p.Config{
			MaxPeers:    100,
			NoDiscovery: true,
			ListenAddr:  "0.0.0.0:0",
		},
		NoUSB: true,
		// It is important that HTTPHost and WSHost remain the same. This
		// ensures that only one server is started up on the HTTPHost. That one
		// server can still handle both http and ws connectons.
		HTTPHost:             "0.0.0.0",
		WSHost:               "0.0.0.0",
		UsePlaintextKeystore: true,
	}

	baseEthConfig = &eth.Config{
		SyncMode:        downloader.FullSync,
		DatabaseCache:   256,
		DatabaseHandles: 256,
		TxPool:          core.DefaultTxPoolConfig,
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
			RequestTimeout:        50,
			Epoch:                 10,
			ProposerPolicy:        istanbul.ShuffledRoundRobin,
			DefaultLookbackWindow: 3,
			BlockPeriod:           0,
		},
	}
)

// Node provides an enhanced interface to node.Node with useful additions, the
// *node.Node is embedded so that its api is available through Node.
type Node struct {
	*node.Node
	Config        *node.Config
	P2PListenAddr string
	Eth           *eth.Ethereum
	EthConfig     *eth.Config
	WsClient      *ethclient.Client
	Nonce         uint64
	Key           *ecdsa.PrivateKey
	Address       common.Address
	DevKey        *ecdsa.PrivateKey
	DevAddress    common.Address
	Tracker       *Tracker
	// The transactions that this node has sent.
	SentTxs []*types.Transaction
}

// NewNode creates a new running node with the provided config.
func NewNode(
	validatorAccount,
	devAccount *env.Account,
	nc *node.Config,
	ec *eth.Config,
	genesis *core.Genesis,
) (*Node, error) {

	// Copy the node config so we can modify it without damaging the original
	ncCopy := *nc
	// p2p key and address
	ncCopy.P2P.PrivateKey = validatorAccount.PrivateKey

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
		Config:     &ncCopy,
		EthConfig:  ecCopy,
		Key:        validatorAccount.PrivateKey,
		Address:    validatorAccount.Address,
		DevAddress: devAccount.Address,
		DevKey:     devAccount.PrivateKey,
		Tracker:    NewTracker(),
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

	// Give this logger context based on the node address so that we can easily
	// trace single node execution in the logs. We set the logger only on the
	// copy, since it is not useful for black box testing and it is also not
	// marshalable since the implementation contains unexported fields.
	//
	// Note unfortunately there are many other loggers created in geth separate
	// from this one, which means we still see a lot of output that is not
	// attributable to a specific node.
	nodeConfigCopy.Logger = log.New("node", n.Address.String()[2:7])
	nodeConfigCopy.Logger.SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))

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

	err = n.Node.Start()
	if err != nil {
		return err
	}

	// The ListenAddr is set at p2p server startup, save it here.
	n.P2PListenAddr = n.Node.Server().ListenAddr

	// Import the node key into the keystore and then unlock it, the keystore
	// is the interface used for signing operations so the node key needs to be
	// inside it.
	ks := n.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	account, err := ks.ImportECDSA(n.Key, "")
	if err != nil {
		return err
	}
	err = ks.TimedUnlock(account, "", 0)
	if err != nil {
		return err
	}
	_, _, err = core.SetupGenesisBlock(n.Eth.ChainDb(), n.EthConfig.Genesis)
	if err != nil {
		return err
	}
	n.WsClient, err = ethclient.Dial(n.WSEndpoint())
	if err != nil {
		return err
	}
	n.Nonce, err = n.WsClient.PendingNonceAt(context.Background(), n.DevAddress)
	if err != nil {
		return err
	}
	err = n.Tracker.StartTracking(n.WsClient)
	if err != nil {
		return err
	}
	return n.Eth.StartMining()
}

// Close shuts down the node and releases all resources and removes the datadir
// unless an error is returned, in which case there is no guarantee that all
// resources are released.
func (n *Node) Close() error {
	err := n.Tracker.StopTracking()
	if err != nil {
		return err
	}
	n.WsClient.Close()
	if n.Node != nil {
		err = n.Node.Close() // This also shuts down the Eth service
	}
	os.RemoveAll(n.Config.DataDir)
	return err
}

// SendCeloTracked functions like SendCelo but also waits for the transaction to be processed.
func (n *Node) SendCeloTracked(ctx context.Context, recipient common.Address, value int64) (*types.Transaction, error) {
	tx, err := n.SendCelo(ctx, recipient, value)
	if err != nil {
		return nil, err
	}
	err = n.AwaitTransactions(ctx, tx)
	if err != nil {
		return nil, err
	}
	return n.Tracker.GetProcessedTx(tx.Hash()), nil
}

// SendCelo submits a value transfer transaction to the network to send celo to
// the recipient. The submitted transaction is returned.
func (n *Node) SendCelo(ctx context.Context, recipient common.Address, value int64) (*types.Transaction, error) {
	signer := types.MakeSigner(n.EthConfig.Genesis.Config, common.Big0)
	tx, err := ValueTransferTransaction(
		n.WsClient,
		n.DevKey,
		n.DevAddress,
		recipient,
		n.Nonce,
		big.NewInt(value),
		signer)

	if err != nil {
		return nil, err
	}
	err = n.WsClient.SendTransaction(ctx, tx)
	if err != nil {
		return nil, err
	}
	n.Nonce++
	n.SentTxs = append(n.SentTxs, tx)
	return tx, nil
}

// AwaitTransactions awaits all the provided transactions.
func (n *Node) AwaitTransactions(ctx context.Context, txs ...*types.Transaction) error {
	sentHashes := make([]common.Hash, len(txs))
	for i, tx := range txs {
		sentHashes[i] = tx.Hash()
	}
	return n.Tracker.AwaitTransactions(ctx, sentHashes)
}

// AwaitSentTransactions awaits all the transactions that this node has sent
// via SendCelo.
func (n *Node) AwaitSentTransactions(ctx context.Context) error {
	return n.AwaitTransactions(ctx, n.SentTxs...)
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

func Accounts(numValidators int) *env.AccountsConfig {
	return &env.AccountsConfig{
		Mnemonic:             env.MustNewMnemonic(),
		NumValidators:        numValidators,
		ValidatorsPerGroup:   1,
		NumDeveloperAccounts: numValidators,
	}
}

// BuildConfig generates genesis and eth config instances, that can be modified
// before passing to NewNetwork or NewNode.
//
// NOTE: Do not edit the Istanbul field of the returned genesis config it will
// be overwritten with the corresponding config from the Istanbul field of the
// returned eth config.
func BuildConfig(accounts *env.AccountsConfig) (*genesis.Config, *eth.Config, error) {
	gc := genesis.CreateCommonGenesisConfig(
		big.NewInt(1),
		accounts.AdminAccount().Address,
		params.IstanbulConfig{},
	)
	genesis.FundAccounts(gc, accounts.DeveloperAccounts())

	// copy the base eth config, so we can modify it without damaging the
	// original.
	ec := &eth.Config{}
	err := copyObject(baseEthConfig, ec)
	return gc, ec, err
}

// NewNetwork generates a network of nodes that are running and mining. For
// each provided validator account a corresponding node is created and each
// node is also assigned a developer account, there must be at least as many
// developer accounts provided as validator accounts. If there is an error it
// will be returned immediately, meaning that some nodes may be running and
// others not.
func NewNetwork(accounts *env.AccountsConfig, gc *genesis.Config, ec *eth.Config) (Network, error) {

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

	genesis, err := genesis.GenerateGenesis(accounts, gc, "../compiled-system-contracts")
	if err != nil {
		return nil, fmt.Errorf("failed to generate genesis: %v", err)
	}

	va := accounts.ValidatorAccounts()
	da := accounts.DeveloperAccounts()
	network := make([]*Node, len(va))
	for i := range va {

		n, err := NewNode(&va[i], &da[i], baseNodeConfig, ec, genesis)
		if err != nil {
			return nil, fmt.Errorf("failed to build node for network: %v", err)
		}
		network[i] = n
	}

	enodes := make([]*enode.Node, len(network))
	for i, n := range network {
		host, port, err := net.SplitHostPort(n.P2PListenAddr)
		if err != nil {
			return nil, err
		}
		portNum, err := strconv.Atoi(port)
		if err != nil {
			return nil, err
		}
		en := enode.NewV4(&n.Key.PublicKey, net.ParseIP(host), portNum, portNum)
		enodes[i] = en
	}
	// Connect nodes to each other, although this means that nodes can reach
	// each other nodes don't start sending consensus messages to another node
	// until they have received an enode certificate from that node.
	for i, en := range enodes {
		for j, n := range network {
			if j == i {
				continue
			}
			n.Server().AddPeer(en, p2p.ValidatorPurpose)
			n.Server().AddTrustedPeer(en, p2p.ValidatorPurpose)
		}
	}

	// Give nodes some time to connect. Also there is a race condition in
	// miner.worker its field snapshotBlock is set only when new transactions
	// are received or commitNewWork is called. But both of these happen in
	// goroutines separate to the call to miner.Start and miner.Start does not
	// wait for snapshotBlock to be set. Therefore there is currently no way to
	// know when it is safe to call estimate gas.  What we do here is sleep a
	// bit and cross our fingers.
	time.Sleep(25 * time.Millisecond)

	version := uint(time.Now().Unix())

	// Share enode certificates between nodes, nodes wont consider other nodes
	// valid validators without seeing an enode certificate message from them.
	for i := range network {
		enodeCertificate := &istanbul.EnodeCertificate{
			EnodeURL: enodes[i].URLv4(),
			Version:  version,
		}
		enodeCertificateBytes, err := rlp.EncodeToBytes(enodeCertificate)
		if err != nil {
			return nil, err
		}

		b := network[i].Eth.Engine().(*backend.Backend)
		msg := &istanbul.Message{
			Code:    istanbul.EnodeCertificateMsg,
			Address: b.Address(),
			Msg:     enodeCertificateBytes,
		}
		// Sign the message
		if err := msg.Sign(b.Sign); err != nil {
			return nil, err
		}
		p, err := msg.Payload()
		if err != nil {
			return nil, err
		}

		err = b.Gossip(p, istanbul.EnodeCertificateMsg)
		if err != nil {
			return nil, err
		}
	}

	return network, nil
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

// Shutdown closes all nodes in the network, any errors that are encountered are
// printed to stdout.
func (n Network) Shutdown() {
	for _, node := range n {
		if node != nil {
			err := node.Close()
			if err != nil {
				fmt.Printf("error shutting down node %v: %v", node.Address.String(), err)
			}
		}
	}
}

// ValueTransferTransaction builds a signed value transfer transaction from the
// sender to the recipient with the given value and nonce, it uses the client
// to suggest a gas price and to estimate the gas.
func ValueTransferTransaction(
	client *ethclient.Client,
	senderKey *ecdsa.PrivateKey,
	sender,
	recipient common.Address,
	nonce uint64,
	value *big.Int,
	signer types.Signer,
) (*types.Transaction, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	// Figure out the gas allowance and gas price values
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to suggest gas price: %v", err)
	}

	msg := ethereum.CallMsg{From: sender, To: &recipient, GasPrice: gasPrice, Value: value}
	gasLimit, err := client.EstimateGas(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate gas needed: %v", err)
	}

	// Create the transaction and sign it
	rawTx := types.NewTransactionEthCompatible(nonce, recipient, value, gasLimit, gasPrice, nil)
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
