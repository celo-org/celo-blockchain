// Copyright 2016 The go-ethereum Authors
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

// Package ethstats implements the network stats reporting service.
package ethstats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	istanbulBackend "github.com/ethereum/go-ethereum/consensus/istanbul/backend"
	"github.com/ethereum/go-ethereum/contract_comm/validators"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/les"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"
)

const (
	// historyUpdateRange is the number of blocks a node should report upon login or
	// history request.
	historyUpdateRange = 50

	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10
	// istDelegateSignChanSize is the size of the channel listening to DelegateSignEvent
	istDelegateSignChanSize = 5

	// connectionTimeout waits for the websocket connection to be established
	connectionTimeout = 10
	// delegateSignTimeout waits for the proxy to sign a message
	delegateSignTimeout = 5
	// wait longer if there are difficulties with login
	loginTimeout = 50
	// statusUpdateInterval is the frequency of sending full node reports
	statusUpdateInterval = 13
	// valSetInterval is the frequency in blocks to send the validator set
	valSetInterval = 11

	actionBlock    = "block"
	actionHello    = "hello"
	actionHistory  = "history"
	actionLatency  = "latency"
	actionNodePing = "node-ping"
	actionNodePong = "node-pong"
	actionPending  = "pending"
	actionStats    = "stats"
)

type txPool interface {
	// SubscribeNewTxsEvent should return an event subscription of
	// NewTxsEvent and send events to the given channel.
	SubscribeNewTxsEvent(chan<- core.NewTxsEvent) event.Subscription
}

type blockChain interface {
	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription
}

// StatsPayload todo: document this
type StatsPayload struct {
	Action string      `json:"action"`
	Stats  interface{} `json:"stats"`
}

// DelegateSignMessage Payload to be signed with the peer that sent it
type DelegateSignMessage struct {
	PeerID  enode.ID
	Payload StatsPayload
}

// Service implements an Ethereum netstats reporting daemon that pushes local
// chain statistics up to a monitoring server.
type Service struct {
	server        *p2p.Server              // Peer-to-peer server to retrieve networking infos
	eth           *eth.Ethereum            // Full Ethereum service if monitoring a full node
	les           *les.LightEthereum       // Light Ethereum service if monitoring a light node
	engine        consensus.Engine         // Consensus engine to retrieve variadic block fields
	backend       *istanbulBackend.Backend // Istanbul consensus backend
	nodeName      string                   // Name of the node to display on the monitoring page
	celostatsHost string                   // Remote address of the monitoring service
	stopFn        context.CancelFunc       // Close ctx Done channel

	pongCh chan struct{} // Pong notifications are fed into this channel
	histCh chan []uint64 // History request block numbers are fed into this channel
}

func parseStatsConnectionURL(url string, name *string, host *string) error {
	re := regexp.MustCompile("([^:@]*)?@(.+)")
	parts := re.FindStringSubmatch(url)
	if len(parts) != 3 {
		return fmt.Errorf("invalid netstats url: \"%s\", should be nodename@host:port", url)
	}
	*name = parts[1]
	*host = parts[2]
	return nil
}

// New returns a monitoring service ready for stats reporting.
func New(url string, ethServ *eth.Ethereum, lesServ *les.LightEthereum) (*Service, error) {
	// Assemble and return the stats service
	var (
		engine        consensus.Engine
		name          string
		celostatsHost string
	)
	if ethServ != nil {
		engine = ethServ.Engine()
	} else {
		engine = lesServ.Engine()
	}

	backend := engine.(*istanbulBackend.Backend)
	if !backend.IsProxiedValidator() {
		// Parse the netstats connection url
		if err := parseStatsConnectionURL(url, &name, &celostatsHost); err != nil {
			return nil, err
		}
	}

	return &Service{
		eth:           ethServ,
		les:           lesServ,
		engine:        engine,
		backend:       backend,
		nodeName:      name,
		celostatsHost: celostatsHost,
		stopFn:        nil,
		pongCh:        make(chan struct{}),
		histCh:        make(chan []uint64, 1),
	}, nil
}

// Protocols implements node.Service, returning the P2P network protocols used
// by the stats service (nil as it doesn't use the devp2p overlay network).
func (s *Service) Protocols() []p2p.Protocol { return nil }

// APIs implements node.Service, returning the RPC API endpoints provided by the
// stats service (nil as it doesn't provide any user callable APIs).
func (s *Service) APIs() []rpc.API { return nil }

// Start implements node.Service, starting up the monitoring and reporting daemon.
func (s *Service) Start(server *p2p.Server) error {
	// TODO add lock to avoid starting twice or starting an already started service
	s.server = server
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s.stopFn = cancel
		s.loop(ctx)
	}()

	log.Info("Stats daemon started")
	return nil
}

// Stop implements node.Service, terminating the monitoring and reporting daemon.
func (s *Service) Stop() error {
	// TODO don't stop if already stopped
	// TODO use lock
	if s.stopFn != nil {
		log.Info("Stats daemon stopped")
		s.stopFn()
		s.stopFn = nil
	}
	// TODO use WaitGroup to wait for loop to finish (or use errgroup for it)
	return nil
}

// loop keeps trying to connect to the netstats server, reporting chain events
// until termination.
func (s *Service) loop(ctx context.Context) {
	// Subscribe to chain events to execute updates on
	var blockchain blockChain
	var txpool txPool
	if s.eth != nil {
		blockchain = s.eth.BlockChain()
		txpool = s.eth.TxPool()
	} else {
		blockchain = s.les.BlockChain()
		txpool = s.les.TxPool()
	}

	// Start a goroutine that exhausts the subsciptions to avoid events piling up
	var (
		headCh = make(chan *types.Block, 1)
		txCh   = make(chan struct{}, 1)
		signCh = make(chan *DelegateSignMessage, 1)
		sendCh = make(chan *StatsPayload, 1)
	)

	group, ctxGroup := errgroup.WithContext(ctx)
	group.Go(func() error { return s.handleDelegateSignEvents(ctxGroup, sendCh, signCh) })
	group.Go(func() error { return s.handleNewTransactionEvents(ctxGroup, txCh, txpool) })
	group.Go(func() error { return s.handleChainHeadEvents(ctxGroup, headCh, blockchain) })

	if s.backend.IsProxiedValidator() {
		group.Go(func() error {
			for {
				select {
				case delegateSignMsg := <-signCh:
					if err := s.handleDelegateSign(&delegateSignMsg.Payload, delegateSignMsg.PeerID); err != nil {
						log.Warn("Delegate sign failed", "err", err)
					}
				case <-ctxGroup.Done():
					return ctxGroup.Err()
				}
			}
		})
	} else {
		group.Go(func() error {
			// Resolve the URL, defaulting to TLS, but falling back to none too
			path := fmt.Sprintf("%s/api", s.celostatsHost)
			urls := []string{path}

			// url.Parse and url.IsAbs is unsuitable (https://github.com/golang/go/issues/19779)
			if !strings.Contains(path, "://") {
				urls = []string{"wss://" + path, "ws://" + path}
			}

			// Establish a websocket connection to the server on any supported URL
			var (
				conn *websocket.Conn
				err  error
			)

			// Loop reporting until termination
			for {
				dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
				header := make(http.Header)
				for _, url := range urls {
					conn, _, err = dialer.Dial(url, header)
					if err == nil {
						break
					}
				}

				if err != nil {
					log.Warn("Stats server unreachable", "err", err)
					time.Sleep(connectionTimeout * time.Second)
					continue
				}

				// Authenticate the client with the server
				if err = s.login(conn, sendCh); err != nil {
					log.Warn("Stats login failed", "err", err)
					conn.Close()
					time.Sleep(connectionTimeout * time.Second)
					continue
				}

				// This go routine will close when the connection gets closed/lost
				go s.readLoop(conn)

				// Send the initial stats so our node looks decent from the get go
				if err = s.report(conn, sendCh); err != nil {
					log.Warn("Initial stats report failed", "err", err)
					conn.Close()
					continue
				}

				// Keep sending status updates until the connection breaks
				fullReport := time.NewTicker(statusUpdateInterval * time.Second)

				for err == nil {
					select {
					case <-ctxGroup.Done():
						conn.Close()
						return ctxGroup.Err()

					case <-fullReport.C:
						if err = s.report(conn, sendCh); err != nil {
							log.Warn("Full stats report failed", "err", err)
						}
					case list := <-s.histCh:
						if err = s.reportHistory(conn, list); err != nil {
							log.Warn("Requested history report failed", "err", err)
						}
					case head := <-headCh:
						if err = s.reportBlock(conn, head); err != nil {
							log.Warn("Block stats report failed", "err", err)
						}
					case <-txCh:
						if err = s.reportPending(conn); err != nil {
							log.Warn("Transaction stats report failed", "err", err)
						}
					case signedMessage := <-sendCh:
						if err = s.handleDelegateSend(conn, signedMessage); err != nil {
							log.Warn("Delegate send failed", "err", err)
						}
					}
				}
				// Make sure the connection is closed
				conn.Close()
				// Continues with the for, and tries to login again until the ctx was cancelled
			}
		})
	}

	group.Wait()
}

// login tries to authorize the client at the remote server.
func (s *Service) login(conn *websocket.Conn, sendCh chan *StatsPayload) error {
	// Construct and send the login authentication
	infos := s.server.NodeInfo()

	var (
		network  string
		protocol string
	)
	if info := infos.Protocols[istanbul.ProtocolName]; info != nil {
		ethInfo, ok := info.(*eth.NodeInfo)
		if !ok {
			return errors.New("Could not resolve NodeInfo")
		}
		network = fmt.Sprintf("%d", ethInfo.Network)
		protocol = fmt.Sprintf("%s/%d", istanbul.ProtocolName, istanbul.ProtocolVersions[0])
	} else {
		lesProtocol, ok := infos.Protocols["les"]
		if !ok {
			return errors.New("No less protocol found")
		}
		lesInfo, ok := lesProtocol.(*les.NodeInfo)
		if !ok {
			return errors.New("Could not resolve NodeInfo")
		}
		network = fmt.Sprintf("%d", lesInfo.Network)
		protocol = fmt.Sprintf("les/%d", les.ClientProtocolVersions[0])
	}
	auth := &authMsg{
		ID: s.backend.ValidatorAddress().String(),
		Info: nodeInfo{
			Name:     s.nodeName,
			Node:     infos.Name,
			Port:     infos.Ports.Listener,
			Network:  network,
			Protocol: protocol,
			API:      "No",
			Os:       runtime.GOOS,
			OsVer:    runtime.GOARCH,
			Client:   "0.1.1",
			History:  true,
		},
	}

	if err := s.sendStats(conn, actionHello, auth); err != nil {
		return err
	}

	if s.backend.IsProxy() {
		// Proxy needs a delegate send here to get ACK
		if err := s.waitAndDelegateMessageWithTimeout(conn, sendCh); err != nil {
			return err
		}
	}

	// Retrieve the remote ack or connection termination
	var ack map[string][]string

	signalCh := make(chan error)

	go func() {
		signalCh <- conn.ReadJSON(&ack)
	}()

	select {
	case <-time.After(loginTimeout * time.Second):
		// Login timeout, abort
		return errors.New("delegation of login timed out")
	case err := <-signalCh:
		if err != nil {
			return errors.New("unauthorized, try registering your validator to get whitelisted")
		}
	}

	emit, ok := ack["emit"]

	if !ok {
		return errors.New("emit not in ack")
	}

	if len(emit) != 1 || emit[0] != "ready" {
		return errors.New("unauthorized")
	}

	return nil
}

func (s *Service) waitAndDelegateMessageWithTimeout(conn *websocket.Conn, sendCh chan *StatsPayload) error {
	select {
	case signedMessage := <-sendCh:
		return s.handleDelegateSend(conn, signedMessage)
	case <-time.After(delegateSignTimeout * time.Second):
		return errors.New("delegation sign timeout")
	}
}

func (s *Service) handleDelegateSign(messageToSign *StatsPayload, peerID enode.ID) error {
	signedStats, err := s.signStats(messageToSign.Stats)
	if err != nil {
		return err
	}

	signedMessage := &StatsPayload{
		Action: messageToSign.Action,
		Stats:  signedStats,
	}
	msg, err := json.Marshal(signedMessage)
	if err != nil {
		return err
	}
	return s.backend.SendDelegateSignMsgToProxy(msg, peerID)
}

func (s *Service) handleDelegateSend(conn *websocket.Conn, signedMessage *StatsPayload) error {
	report := map[string][]interface{}{
		"emit": {signedMessage.Action, signedMessage.Stats},
	}
	return conn.WriteJSON(report)
}

func (s *Service) handleDelegateSignEvents(ctx context.Context, sendCh chan *StatsPayload, signCh chan *DelegateSignMessage) error {
	ch := make(chan istanbul.MessageWithPeerIDEvent, istDelegateSignChanSize)
	subscription := s.backend.SubscribeNewDelegateSignEvent(ch)
	defer subscription.Unsubscribe()

	for {
		select {
		case msg := <-ch:
			var delegateSignMessage DelegateSignMessage
			delegateSignMessage.PeerID = msg.PeerID
			if err := json.Unmarshal(msg.Payload, &delegateSignMessage.Payload); err != nil {
				continue
			}
			if s.backend.IsProxy() {
				// proxy should send to websocket
				select {
				case sendCh <- &delegateSignMessage.Payload:
				default:
				}
			} else if s.backend.IsProxiedValidator() {
				// proxied validator should sign
				select {
				case signCh <- &delegateSignMessage:
				default:
				}
			}
		case err := <-subscription.Err():
			log.Error("Subscription for handle signing messages failed", "err", err)
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *Service) handleNewTransactionEvents(ctx context.Context, txChan chan struct{}, txpool txPool) error {
	var lastTx mclock.AbsTime
	ch := make(chan core.NewTxsEvent, txChanSize)
	subscription := txpool.SubscribeNewTxsEvent(ch)
	defer subscription.Unsubscribe()

	for {
		select {
		// Notify of new transaction events, but drop if too frequent
		case <-ch:
			if time.Duration(mclock.Now()-lastTx) < time.Second {
				continue
			}
			lastTx = mclock.Now()

			select {
			case txChan <- struct{}{}:
			default:
			}
		case err := <-subscription.Err():
			log.Error("Subscription for handle new transactions failed", "err", err)
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *Service) handleChainHeadEvents(ctx context.Context, headCh chan *types.Block, blockchain blockChain) error {
	ch := make(chan core.ChainHeadEvent, chainHeadChanSize)
	subscription := blockchain.SubscribeChainHeadEvent(ch)
	defer subscription.Unsubscribe()

	for {
		select {
		// Notify of chain head events, but drop if too frequent
		case head := <-ch:
			select {
			case headCh <- head.Block:
			default:
			}
		case err := <-subscription.Err():
			log.Error("Subscription for handle chain head failed", "err", err)
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// readLoop loops as long as the connection is alive and retrieves data packets
// from the network socket. If any of them match an active request, it forwards
// it, if they themselves are requests it initiates a reply, and lastly it drops
// unknown packets.
func (s *Service) readLoop(conn *websocket.Conn) {
	// If the read loop exists, close the connection
	defer conn.Close()

	for {
		// Retrieve the next generic network packet and bail out on error
		var msg interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			log.Warn("Failed to decode stats server message", "err", err)
			return
		}

		switch packet := msg.(type) {
		case map[string]interface{}:
			msgEmit, _ := packet["emit"].([]interface{})

			log.Trace("Received message from stats server", "msgEmit", msgEmit)
			if len(msgEmit) == 0 {
				log.Warn("Stats server sent non-broadcast", "msgEmit", msgEmit)
				return
			}

			command, ok := msgEmit[0].(string)
			if !ok {
				log.Warn("Invalid stats server message type", "type", msgEmit[0])
				return
			}
			// If the message is a ping reply, deliver (someone must be listening!)
			if len(msgEmit) == 2 && command == actionNodePong {
				select {
				case s.pongCh <- struct{}{}:
					// Pong delivered, continue listening
					continue
				default:
					// Ping routine dead, abort
					log.Warn("Stats server pinger seems to have died")
					return
				}
			}
			// If the message is a history request, forward to the event processor
			if len(msgEmit) == 2 && command == actionHistory {
				// Make sure the request is valid and doesn't crash us
				request, ok := msgEmit[1].(map[string]interface{})
				if !ok {
					log.Warn("Invalid stats history request", "msg", msgEmit[1])
					s.histCh <- nil
					continue // Ethstats sometime sends invalid history requests, ignore those
				}
				list, ok := request["list"].([]interface{})
				if !ok {
					log.Warn("Invalid stats history block list", "list", request["list"])
					return
				}
				// Convert the block number list to an integer list
				numbers := make([]uint64, len(list))
				for i, num := range list {
					n, ok := num.(float64)
					if !ok {
						log.Warn("Invalid stats history block number", "number", num)
						return
					}
					numbers[i] = uint64(n)
				}
				select {
				case s.histCh <- numbers:
					continue
				default:
				}
			}
		default:
			// Report anything else and continue
			log.Info("Ping from websocket server", "msg", msg)
			// Primus server might want to have a pong or it closes the connection
			var serverTime = fmt.Sprintf("primus::pong::%d", time.Now().UnixNano()/int64(time.Millisecond))
			err := conn.WriteMessage(websocket.TextMessage, []byte(serverTime))
			if err != nil {
				log.Error("Sending pong failed!", "err", err)
			}
		}
	}
}

// nodeInfo is the collection of meta information about a node that is displayed
// on the monitoring page.
type nodeInfo struct {
	Name     string `json:"name"`
	Node     string `json:"node"`
	Port     int    `json:"port"`
	Network  string `json:"net"`
	Protocol string `json:"protocol"`
	API      string `json:"api"`
	Os       string `json:"os"`
	OsVer    string `json:"os_v"`
	Client   string `json:"client"`
	History  bool   `json:"canUpdateHistory"`
}

// authMsg is the authentication infos needed to login to a monitoring server.
type authMsg struct {
	ID   string   `json:"id"`
	Info nodeInfo `json:"info"`
}

// report collects all possible data to report and send it to the stats server.
// This should only be used on reconnects or rarely to avoid overloading the
// server. Use the individual methods for reporting subscribed events.
func (s *Service) report(conn *websocket.Conn, sendCh chan *StatsPayload) error {
	if err := s.reportLatency(conn, sendCh); err != nil {
		log.Warn("Latency failed to report", "err", err)
		return err
	}
	if err := s.reportBlock(conn, nil); err != nil {
		return err
	}
	if err := s.reportPending(conn); err != nil {
		return err
	}
	if err := s.reportStats(conn); err != nil {
		return err
	}
	return nil
}

// reportLatency sends a ping request to the server, measures the RTT time and
// finally sends a latency update.
func (s *Service) reportLatency(conn *websocket.Conn, sendCh chan *StatsPayload) error {
	// Send the current time to the ethstats server
	start := time.Now()

	ping := map[string]interface{}{
		"id":         s.backend.ValidatorAddress().String(),
		"clientTime": start.String(),
	}
	if err := s.sendStats(conn, actionNodePing, ping); err != nil {
		return err
	}
	// Proxy needs a delegate send here to get ACK
	if s.backend.IsProxy() {
		if err := s.waitAndDelegateMessageWithTimeout(conn, sendCh); err != nil {
			return err
		}
	}
	// Wait for the pong request to arrive back
	select {
	case <-s.pongCh:
		// Pong delivered, report the latency
	case <-time.After(5 * time.Second):
		// Ping timeout, abort
		return errors.New("ping timed out")
	}
	latency := strconv.Itoa(int((time.Since(start) / time.Duration(2)).Nanoseconds() / 1000000))

	// Send back the measured latency
	log.Trace("Sending measured latency to ethstats", "latency", latency)

	stats := map[string]interface{}{
		"id":      s.backend.ValidatorAddress().String(),
		"latency": latency,
	}
	return s.sendStats(conn, actionLatency, stats)
}

// blockStats is the information to report about individual blocks.
type blockStats struct {
	Number      *big.Int       `json:"number"`
	Hash        common.Hash    `json:"hash"`
	ParentHash  common.Hash    `json:"parentHash"`
	Timestamp   *big.Int       `json:"timestamp"`
	Miner       common.Address `json:"miner"`
	GasUsed     uint64         `json:"gasUsed"`
	GasLimit    uint64         `json:"gasLimit"`
	TotalDiff   string         `json:"totalDifficulty"`
	Txs         []txStats      `json:"transactions"`
	TxHash      common.Hash    `json:"transactionsRoot"`
	Root        common.Hash    `json:"stateRoot"`
	EpochSize   uint64         `json:"epochSize"`
	BlockRemain uint64         `json:"blockRemain"`
	Validators  validatorSet   `json:"validators"`
}

// txStats is the information to report about individual transactions.
type txStats struct {
	Hash common.Hash `json:"hash"`
}

func (s *Service) signStats(stats interface{}) (map[string]interface{}, error) {
	msg, err := json.Marshal(stats)
	if err != nil {
		return nil, err
	}
	msgHash := crypto.Keccak256Hash(msg)

	etherBase, errEtherbase := s.eth.Etherbase()
	if errEtherbase != nil {
		return nil, errEtherbase
	}

	account := accounts.Account{Address: etherBase}
	wallet, errWallet := s.eth.AccountManager().Find(account)
	if errWallet != nil {
		return nil, errWallet
	}

	pubkey, errPubkey := wallet.GetPublicKey(account)
	if errPubkey != nil {
		return nil, errPubkey
	}
	pubkeyBytes := crypto.FromECDSAPub(pubkey)

	signature, errSign := wallet.SignData(account, accounts.MimetypeTypedData, msg)
	if errSign != nil {
		return nil, errSign
	}

	proof := map[string]interface{}{
		"signature": hexutil.Encode(signature),
		"address":   etherBase,
		"publicKey": hexutil.Encode(pubkeyBytes),
		"msgHash":   msgHash.Hex(),
	}

	/* Server-side verification in go: */
	// 	sig := signature[:len(signature)-1]
	// 	verified := crypto.VerifySignature(pubkey, msgHash.Bytes(), sig)
	//				& address == crypto.PubkeyToAddress(*pubkey).Hex()

	/* Client-side verification in JS: */
	//	const { Keccak } = require('sha3');
	// 	const EC = require('elliptic').ec;
	// 	const addressHasher = new Keccak(256)
	// 	addressHasher.update(publicKey.substr(4), 'hex')
	// 	const msgHasher = new Keccak(256)
	// 	msgHasher.update(JSON.stringify(stats))
	// 	const ec = new EC('secp256k1');
	// 	const pubkey = ec.keyFromPublic(publicKey.substr(2), 'hex')
	// 	const signature = {
	//		r : signature.substr(2, 64),
	//		s : signature.substr(66, 64)
	//	}
	//  verified = pubkey.verify(msgHash, signature)
	//				&& address == addressHasher.digest('hex').substr(24)
	//				&& msgHash == msgHasher.digest('hex')

	signedStats := map[string]interface{}{
		"stats": stats,
		"proof": proof,
	}

	return signedStats, nil
}

func (s *Service) sendStats(conn *websocket.Conn, action string, stats interface{}) error {
	if s.backend.IsProxy() {
		statsWithAction := map[string]interface{}{
			"stats":  stats,
			"action": action,
		}
		msg, err := json.Marshal(statsWithAction)
		if err != nil {
			return err
		}
		go func() {
			err := s.backend.SendDelegateSignMsgToProxiedValidator(msg)
			if err != nil {
				log.Warn("Failed to delegate", "err", err)
				conn.Close()
			}
		}()
		return nil
	}
	signedStats, err := s.signStats(stats)
	if err != nil {
		return err
	}

	report := map[string][]interface{}{
		"emit": {action, signedStats},
	}
	return conn.WriteJSON(report)
}

// reportBlock retrieves the current chain head and reports it to the stats server.
func (s *Service) reportBlock(conn *websocket.Conn, block *types.Block) error {
	// Gather the block details from the header or block chain
	details := s.assembleBlockStats(block)

	// Assemble the block report and send it to the server
	log.Trace("Sending new block to ethstats", "number", details.Number, "hash", details.Hash)

	stats := map[string]interface{}{
		"id":    s.backend.ValidatorAddress().String(),
		"block": details,
	}
	return s.sendStats(conn, actionBlock, stats)
}

// assembleBlockStats retrieves any required metadata to report a single block
// and assembles the block stats. If block is nil, the current head is processed.
func (s *Service) assembleBlockStats(block *types.Block) *blockStats {
	// Gather the block infos from the local blockchain
	var (
		header  *types.Header
		stateDB *state.StateDB
		td      *big.Int
		txs     []txStats
		valSet  validatorSet
	)
	if s.eth != nil {
		// Full nodes have all needed information available
		if block == nil {
			block = s.eth.BlockChain().CurrentBlock()
		}
		header = block.Header()
		stateDB, _ = s.eth.BlockChain().State()
		td = s.eth.BlockChain().GetTd(header.Hash(), header.Number.Uint64())

		txs = make([]txStats, len(block.Transactions()))
		for i, tx := range block.Transactions() {
			txs[i].Hash = tx.Hash()
		}
	} else {
		// Light nodes would need on-demand lookups for transactions, skip
		if block != nil {
			header = block.Header()
		} else {
			header = s.les.BlockChain().CurrentHeader()
		}
		stateDB, _ = s.les.BlockChain().State()
		td = s.les.BlockChain().GetTd(header.Hash(), header.Number.Uint64())
		txs = []txStats{}
	}
	// Assemble and return the block stats
	author, _ := s.backend.Author(header)

	// Add epoch info
	epochSize := s.eth.Config().Istanbul.Epoch
	blockRemain := epochSize - istanbul.GetNumberWithinEpoch(header.Number.Uint64(), epochSize)

	// only assemble every valSetInterval blocks
	if block != nil && block.Number().Uint64()%valSetInterval == 0 {
		valSet = s.assembleValidatorSet(block, stateDB)
	}

	gasLimit := core.CalcGasLimit(block, stateDB)

	return &blockStats{
		Number:      header.Number,
		Hash:        header.Hash(),
		ParentHash:  header.ParentHash,
		Timestamp:   new(big.Int).SetUint64(header.Time),
		Miner:       author,
		GasUsed:     header.GasUsed,
		GasLimit:    gasLimit,
		TotalDiff:   td.String(),
		Txs:         txs,
		TxHash:      header.TxHash,
		Root:        header.Root,
		EpochSize:   epochSize,
		BlockRemain: blockRemain,
		Validators:  valSet,
	}
}

type validatorSet struct {
	Registered []validatorInfo  `json:"registered"`
	Elected    []common.Address `json:"elected"`
}

type validatorInfo struct {
	Address        common.Address `json:"address"`
	Score          string         `json:"score"`
	BLSPublicKey   []byte         `json:"blsPublicKey"`
	EcdsaPublicKey []byte         `json:"ecdsaPublicKey"`
	Affiliation    common.Address `json:"affiliation"`
	Signer         common.Address `json:"signer"`
}

func (s *Service) assembleValidatorSet(block *types.Block, state vm.StateDB) validatorSet {
	var (
		err            error
		valSet         validatorSet
		valsRegistered []validatorInfo
		valsElected    []common.Address
	)

	// Add set of registered validators
	valsRegisteredMap, _ := validators.RetrieveRegisteredValidators(s.eth.BlockChain().CurrentHeader(), state)
	valsRegistered = make([]validatorInfo, 0, len(valsRegisteredMap))
	for _, address := range valsRegisteredMap {
		var valData validators.ValidatorContractData
		valData, err = validators.GetValidator(s.eth.BlockChain().CurrentHeader(), state, address)

		if err != nil {
			log.Warn("Validator data not found", "address", address.Hex(), "err", err)
		}

		valsRegistered = append(valsRegistered, validatorInfo{
			Address:        address,
			Score:          fmt.Sprintf("%d", valData.Score),
			BLSPublicKey:   valData.BlsPublicKey,
			EcdsaPublicKey: valData.EcdsaPublicKey,
			Affiliation:    valData.Affiliation,
			Signer:         valData.Signer,
		})
	}

	// Add addresses of elected validators
	valsElectedList := s.backend.GetValidators(block.Number(), block.Hash())

	valsElected = make([]common.Address, 0, len(valsElectedList))
	for i := range valsElectedList {
		valsElected = append(valsElected, valsElectedList[i].Address())
	}

	valSet = validatorSet{
		Elected:    valsElected,
		Registered: valsRegistered,
	}

	return valSet
}

// reportHistory retrieves the most recent batch of blocks and reports it to the
// stats server.
func (s *Service) reportHistory(conn *websocket.Conn, list []uint64) error {
	// Figure out the indexes that need reporting
	indexes := make([]uint64, 0, historyUpdateRange)
	if len(list) > 0 {
		// Specific indexes requested, send them back in particular
		indexes = append(indexes, list...)
	} else {
		// No indexes requested, send back the top ones
		var head int64
		if s.eth != nil {
			head = s.eth.BlockChain().CurrentHeader().Number.Int64()
		} else {
			head = s.les.BlockChain().CurrentHeader().Number.Int64()
		}
		start := head - historyUpdateRange + 1
		if start < 0 {
			start = 0
		}
		for i := uint64(start); i <= uint64(head); i++ {
			indexes = append(indexes, i)
		}
	}
	// Gather the batch of blocks to report
	history := make([]*blockStats, len(indexes))
	for i, number := range indexes {
		// Retrieve the next block if it's known to us
		var block *types.Block
		if s.eth != nil {
			block = s.eth.BlockChain().GetBlockByNumber(number)
		} else {
			if header := s.les.BlockChain().GetHeaderByNumber(number); header != nil {
				block = types.NewBlockWithHeader(header)
			}
		}
		// If we do have the block, add to the history and continue
		if block != nil {
			history[len(history)-1-i] = s.assembleBlockStats(block)
			continue
		}
		// Ran out of blocks, cut the report short and send
		history = history[len(history)-i:]
		break
	}
	// Assemble the history report and send it to the server
	if len(history) > 0 {
		log.Trace("Sending historical blocks to ethstats", "first", history[0].Number, "last", history[len(history)-1].Number)
	} else {
		log.Trace("No history to send to stats server")
	}
	stats := map[string]interface{}{
		"id":      s.backend.ValidatorAddress().String(),
		"history": history,
	}
	return s.sendStats(conn, actionHistory, stats)
}

// pendStats is the information to report about pending transactions.
type pendStats struct {
	Pending int `json:"pending"`
}

// reportPending retrieves the current number of pending transactions and reports
// it to the stats server.
func (s *Service) reportPending(conn *websocket.Conn) error {
	// Retrieve the pending count from the local blockchain
	var pending int
	if s.eth != nil {
		pending, _ = s.eth.TxPool().Stats()
	} else {
		pending = s.les.TxPool().Stats()
	}
	// Assemble the transaction stats and send it to the server
	log.Trace("Sending pending transactions to ethstats", "count", pending)

	stats := map[string]interface{}{
		"id": s.backend.ValidatorAddress().String(),
		"stats": &pendStats{
			Pending: pending,
		},
	}
	return s.sendStats(conn, actionPending, stats)
}

// nodeStats is the information to report about the local node.
type nodeStats struct {
	Active   bool `json:"active"`
	Syncing  bool `json:"syncing"`
	Mining   bool `json:"mining"`
	Proxy    bool `json:"proxy"`
	Elected  bool `json:"elected"`
	Hashrate int  `json:"hashrate"`
	Peers    int  `json:"peers"`
	GasPrice int  `json:"gasPrice"`
	Uptime   int  `json:"uptime"`
}

// reportPending retrieves various stats about the node at the networking and
// mining layer and reports it to the stats server.
func (s *Service) reportStats(conn *websocket.Conn) error {
	// Gather the syncing and mining infos from the local miner instance
	var (
		validatorAddress common.Address
		mining           bool
		proxy            bool
		elected          bool
		hashrate         int
		syncing          bool
		gasprice         int
	)
	// Eth will be nil only if it is a light client
	if s.eth != nil {
		validatorAddress = s.backend.ValidatorAddress()
		block := s.eth.BlockChain().CurrentBlock()

		proxy = s.backend.IsProxy()
		mining = s.eth.Miner().Mining()
		hashrate = int(s.eth.Miner().HashRate())

		elected = false
		valsElected := s.backend.GetValidators(block.Number(), block.Hash())

		for i := range valsElected {
			if valsElected[i].Address() == validatorAddress {
				elected = true
			}
		}

		sync := s.eth.Downloader().Progress()
		syncing = s.eth.BlockChain().CurrentHeader().Number.Uint64() >= sync.HighestBlock

		price, _ := s.eth.APIBackend.SuggestPrice(context.Background())
		gasprice = int(price.Uint64())
	} else {
		sync := s.les.Downloader().Progress()
		syncing = s.les.BlockChain().CurrentHeader().Number.Uint64() >= sync.HighestBlock
	}
	// Assemble the node stats and send it to the server
	log.Trace("Sending node details to ethstats")

	stats := map[string]interface{}{
		"id":      s.backend.ValidatorAddress().String(),
		"address": validatorAddress,
		"stats": &nodeStats{
			Active:   true,
			Mining:   mining,
			Elected:  elected,
			Proxy:    proxy,
			Hashrate: hashrate,
			Peers:    s.server.PeerCount(),
			GasPrice: gasprice,
			Syncing:  syncing,
			Uptime:   100,
		},
	}

	return s.sendStats(conn, actionStats, stats)
}
