package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/internal/fileutils"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/p2p/enode"

	"github.com/celo-org/celo-blockchain/accounts/keystore"
	"github.com/celo-org/celo-blockchain/common"
)

// NodeConfig represents the configuration of a celo-blockchain node runner
type NodeConfig struct {
	GethPath              string
	GethParams            *env.GethParamsConfig
	ExtraFlags            string
	ChainID               *big.Int
	Number                int
	Account               env.Account
	TxFeeRecipientAccount env.Account
	OtherAccounts         []env.Account
	Datadir               string
	NodeStartPort         int
	RPCStartPort          int
}

// RPCPort is the rpc port this node will use
func (nc *NodeConfig) RPCPort() int64 {
	return int64(nc.RPCStartPort + nc.Number)
}

// NodePort is the node port this node will use
func (nc *NodeConfig) NodePort() int64 {
	return int64(nc.NodeStartPort + nc.Number)
}

// Node represents a Node runner
type Node struct {
	*NodeConfig
}

// NewNode creates a node runner
func NewNode(cfg *NodeConfig) *Node {
	return &Node{
		NodeConfig: cfg,
	}
}

// SetStaticNodes configures static nodes to be used on the node
func (n *Node) SetStaticNodes(enodeUrls ...string) error {
	var staticNodesRaw []byte
	var err error

	if staticNodesRaw, err = json.Marshal(enodeUrls); err != nil {
		return fmt.Errorf("Can't serialize static nodes: %w", err)
	}
	//nolint:gosec
	if err = ioutil.WriteFile(n.staticNodesFile(), staticNodesRaw, 0644); err != nil {
		return fmt.Errorf("Can't serialize static nodes: %w", err)
	}

	return nil
}

// EnodeURL returns the enode url used by the node
func (n *Node) EnodeURL() (string, error) {
	nodekey, err := crypto.LoadECDSA(n.keyFile())
	if err != nil {
		return "", err
	}
	ip := net.IP{127, 0, 0, 1}
	en := enode.NewV4(&nodekey.PublicKey, ip, int(n.NodePort()), int(n.NodePort()))
	return en.URLv4(), nil
}

// AccountAddresses retrieves the list of accounts currently configured in the node
func (n *Node) AccountAddresses() []common.Address {
	ks := keystore.NewKeyStore(path.Join(n.Datadir, "keystore"), keystore.LightScryptN, keystore.LightScryptP)
	addresses := make([]common.Address, 0)
	for _, acc := range ks.Accounts() {
		addresses = append(addresses, acc.Address)
	}
	return addresses
}

// Init will run `geth init` on the node along other initialization procedures
// that need to happen before we run the node
func (n *Node) Init(GenesisJSON string) error {
	if fileutils.FileExists(n.Datadir) {
		os.RemoveAll(n.Datadir)
	}
	os.MkdirAll(n.Datadir, os.ModePerm)

	// Write password file
	if err := ioutil.WriteFile(n.pwdFile(), []byte{}, os.ModePerm); err != nil {
		return err
	}

	// Run geth init
	if out, err := n.runSync("init", GenesisJSON); err != nil {
		os.Stderr.Write(out)
		return err
	}

	// Generate nodekey file (enode private key)
	if err := n.generateNodeKey(); err != nil {
		return err
	}

	// Add Accounts
	ks := keystore.NewKeyStore(path.Join(n.Datadir, "keystore"), keystore.LightScryptN, keystore.LightScryptP)
	if _, err := ks.ImportECDSA(n.Account.PrivateKey, ""); err != nil {
		return err
	}
	for _, acc := range n.OtherAccounts {
		if _, err := ks.ImportECDSA(acc.PrivateKey, ""); err != nil {
			return err
		}
	}

	return nil
}

func (n *Node) generateNodeKey() error {
	nodeKey, err := crypto.GenerateKey()
	if err != nil {
		return err
	}
	if err = crypto.SaveECDSA(n.keyFile(), nodeKey); err != nil {
		return err
	}
	return nil
}

// Run will run the node
func (n *Node) Run(ctx context.Context) error {

	var addressToUnlock string
	for _, addr := range n.AccountAddresses() {
		addressToUnlock += "," + addr.Hex()
	}

	args := []string{
		"--datadir", n.Datadir,
		"--verbosity", "4",
		"--networkid", n.ChainID.String(),
		"--syncmode", "full",
		"--mine",
		"--allow-insecure-unlock",
		"--nodiscover",
		"--nat", "extip:127.0.0.1",
		"--port", strconv.FormatInt(n.NodePort(), 10),
		"--http",
		"--http.addr", n.GethParams.HTTPAddr,
		"--http.port", strconv.FormatInt(n.RPCPort(), 10),
		"--http.api", n.GethParams.HTTPAPI,
		// "--nodiscover", "--nousb ",
		"--unlock", addressToUnlock,
		"--password", n.pwdFile(),
	}

	// Once we're sure we won't run v1.2.x and older, can get rid of this check
	// and just use the new options
	helpBytes, _ := exec.Command(n.GethPath, "--help").Output() // #nosec G204
	useTxFeeRecipient := strings.Contains(string(helpBytes), "miner.validator")
	if useTxFeeRecipient {
		args = append(args,
			"--miner.validator", n.Account.Address.Hex(),
			"--tx-fee-recipient", n.TxFeeRecipientAccount.Address.Hex(),
		)
	} else {
		args = append(args,
			"--etherbase", n.Account.Address.Hex(),
		)
	}

	if n.ExtraFlags != "" {
		args = append(args, strings.Fields(n.ExtraFlags)...)
	}
	cmd := exec.Command(n.GethPath, args...) // #nosec G204

	log.Println(n.GethPath, strings.Join(args, " "))

	logfile, err := os.OpenFile(n.logFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer logfile.Close()
	cmd.Stderr = logfile
	cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	// rpc, err := rpc.Dial(fmt.Sprintf("http://localhost:%d", n.RPCPort()))
	// if err != nil {
	// 	return err
	// }
	// rpc.CallContext(ctx, nil, "personal_unlock", )

	go func() {
		<-ctx.Done()
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			log.Fatal("Failed to send interrupt signal to geth cmd")
		}
	}()

	return cmd.Wait()
}

func (n *Node) pwdFile() string         { return path.Join(n.Datadir, "password") }
func (n *Node) logFile() string         { return path.Join(n.Datadir, "geth.log") }
func (n *Node) keyFile() string         { return path.Join(n.Datadir, "celo/nodekey") }
func (n *Node) staticNodesFile() string { return path.Join(n.Datadir, "/celo/static-nodes.json") }

func (n *Node) runSync(args ...string) ([]byte, error) {
	args = append([]string{"--datadir", n.Datadir}, args...)
	cmd := exec.Command(n.GethPath, args...) // #nosec G204
	return cmd.CombinedOutput()
}
