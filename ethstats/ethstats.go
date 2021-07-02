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
	"sync"
	"time"

	"github.com/celo-org/celo-blockchain/accounts"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/common/mclock"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	istanbulBackend "github.com/celo-org/celo-blockchain/consensus/istanbul/backend"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/contracts/validators"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/eth"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/event"
	"github.com/celo-org/celo-blockchain/les"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/miner"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/rpc"
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

// backend encompasses the bare-minimum functionality needed for ethstats reporting
type backend interface {
	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription
	SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription
	CurrentHeader() *types.Header
	HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error)
	GetTd(ctx context.Context, hash common.Hash) *big.Int
	Stats() (pending int, queued int)
	Downloader() *downloader.Downloader
	AccountManager() *accounts.Manager
	StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error)
	NewEVMRunner(header *types.Header, state vm.StateDB) vm.EVMRunner
}

// fullNodeBackend encompasses the functionality necessary for a full node
// reporting to ethstats
type fullNodeBackend interface {
	backend
	Miner() *miner.Miner
	BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error)
	CurrentBlock() *types.Block
	SuggestPrice(ctx context.Context, currencyAddress *common.Address) (*big.Int, error)
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
	server          *p2p.Server      // Peer-to-peer server to retrieve networking infos
	engine          consensus.Engine // Consensus engine to retrieve variadic block fields
	backend         backend
	istanbulBackend *istanbulBackend.Backend // Istanbul consensus backend
	nodeName        string                   // Name of the node to display on the monitoring page
	celostatsHost   string                   // Remote address of the monitoring service
	stopFn          context.CancelFunc       // Close ctx Done channel

	pongCh chan struct{} // Pong notifications are fed into this channel
	histCh chan []uint64 // History request block numbers are fed into this channel
}

// connWrapper is a wrapper to prevent concurrent-write or concurrent-read on the
// websocket.
//
// From Gorilla websocket docs:
//   Connections support one concurrent reader and one concurrent writer.
//   Applications are responsible for ensuring that no more than one goroutine calls the write methods
//     - NextWriter, SetWriteDeadline, WriteMessage, WriteJSON, EnableWriteCompression, SetCompressionLevel
//   concurrently and that no more than one goroutine calls the read methods
//     - NextReader, SetReadDeadline, ReadMessage, ReadJSON, SetPongHandler, SetPingHandler
//   concurrently.
//   The Close and WriteControl methods can be called concurrently with all other methods.
type connWrapper struct {
	conn *websocket.Conn

	rlock sync.Mutex
	wlock sync.Mutex
}

func newConnectionWrapper(conn *websocket.Conn) *connWrapper {
	return &connWrapper{conn: conn}
}

// WriteJSON wraps corresponding method on the websocket but is safe for concurrent calling
func (w *connWrapper) WriteJSON(v interface{}) error {
	w.wlock.Lock()
	defer w.wlock.Unlock()

	return w.conn.WriteJSON(v)
}

// ReadJSON wraps corresponding method on the websocket but is safe for concurrent calling
func (w *connWrapper) ReadJSON(v interface{}) error {
	w.rlock.Lock()
	defer w.rlock.Unlock()

	return w.conn.ReadJSON(v)
}

// Close wraps corresponding method on the websocket but is safe for concurrent calling
func (w *connWrapper) Close() error {
	// The Close and WriteControl methods can be called concurrently with all other methods,
	// so the mutex is not used here
	return w.conn.Close()
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
func New(node *node.Node, backend backend, engine consensus.Engine, url string) error {
	// Assemble and return the stats service
	var (
		name          string
		celostatsHost string
	)
	istanbulBackend := engine.(*istanbulBackend.Backend)

	if !istanbulBackend.IsProxiedValidator() {
		// Parse the netstats connection url
		if err := parseStatsConnectionURL(url, &name, &celostatsHost); err != nil {
			return err
		}
	}

	ethstats := &Service{
		engine:          engine,
		backend:         backend,
		istanbulBackend: istanbulBackend,
		server:          node.Server(),
		nodeName:        name,
		celostatsHost:   celostatsHost,
		stopFn:          nil,
		pongCh:          make(chan struct{}),
		histCh:          make(chan []uint64, 1),
	}
	node.RegisterLifecycle(ethstats)
	return nil
}

// Protocols implements node.Service, returning the P2P network protocols used
// by the stats service (nil as it doesn't use the devp2p overlay network).
func (s *Service) Protocols() []p2p.Protocol { return nil }

// Start implements node.Service, starting up the monitoring and reporting daemon.
func (s *Service) Start() error {
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s.stopFn = cancel
		s.loop(ctx)
	}()

	log.Info("Stats daemon started")
	return nil
}

// Stop implements node.Lifecycle, terminating the monitoring and reporting daemon.
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
	// Start a goroutine that exhausts the subscriptions to avoid events piling up
	var (
		headCh = make(chan *types.Block, 1)
		txCh   = make(chan struct{}, 1)
		signCh = make(chan *DelegateSignMessage, 1)
		sendCh = make(chan *StatsPayload, 1)
	)

	group, ctxGroup := errgroup.WithContext(ctx)
	group.Go(func() error { return s.handleDelegateSignEvents(ctxGroup, sendCh, signCh) })
	group.Go(func() error { return s.handleNewTransactionEvents(ctxGroup, txCh) })
	group.Go(func() error { return s.handleChainHeadEvents(ctxGroup, headCh) })

	if s.istanbulBackend.IsProxiedValidator() {
		group.Go(func() error {
			for {
				select {
				case delegateSignMsg := <-signCh:
					if s.istanbulBackend.IsValidating() {
						s.fillWithValidatorInfo(&delegateSignMsg.Payload)
						if err := s.handleDelegateSign(&delegateSignMsg.Payload, delegateSignMsg.PeerID); err != nil {
							log.Warn("Delegate sign failed", "err", err)
						}
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

			errTimer := time.NewTimer(0)
			defer errTimer.Stop()
			// Loop reporting until termination
			for {
				select {
				case <-ctxGroup.Done():
					return ctxGroup.Err()
				case <-errTimer.C:
					var (
						conn *connWrapper
						err  error
					)
					// Establish a websocket connection to the server on any supported URL
					dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
					header := make(http.Header)
					header.Set("origin", "http://localhost")
					for _, url := range urls {
						c, _, e := dialer.Dial(url, header)
						err = e
						if err == nil {
							conn = newConnectionWrapper(c)
							break
						}
					}

					if err != nil {
						log.Warn("Stats server unreachable", "err", err)
						errTimer.Reset(connectionTimeout * time.Second)
						continue
					}

					// Authenticate the client with the server
					if err = s.login(conn, sendCh); err != nil {
						log.Warn("Stats login failed", "err", err)
						conn.Close()
						errTimer.Reset(connectionTimeout * time.Second)
						continue
					}

					// This go routine will close when the connection gets closed/lost
					go s.readLoop(conn)

					// Send the initial stats so our node looks decent from the get go
					if err = s.report(conn, sendCh); err != nil {
						log.Warn("Initial stats report failed", "err", err)
						conn.Close()
						errTimer.Reset(0)
						continue
					}

					// Keep sending status updates until the connection breaks
					fullReport := time.NewTicker(statusUpdateInterval * time.Second)

					for err == nil {
						select {
						case <-ctxGroup.Done():
							fullReport.Stop()
							// Make sure the connection is closed
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
							if err = s.reportPending(conn); err != nil {
								log.Warn("Post-block transaction stats report failed", "err", err)
							}
						case <-txCh:
							if err = s.reportPending(conn); err != nil {
								log.Warn("Transaction stats report failed", "err", err)
							}
						case signedMessage := <-sendCh:
							// if it is a ping or hello message, it shouldn't be handled here
							if signedMessage.Action != actionNodePing && signedMessage.Action != actionHello {
								if err = s.handleDelegateSend(conn, signedMessage); err != nil {
									log.Warn("Delegate send failed", "err", err)
								}
							} else {
								// As both discarded messages, if they were required should eventually close the connection
								// we just warn the user to avoid possible unnecessary disconnections (for example, from
								// another backup validator)
								log.Warn("Signed message discarded", "Action", signedMessage.Action)
							}
						}
					}
					fullReport.Stop()

					// Make sure the connection is closed
					conn.Close()
					errTimer.Reset(0)
					// Continues with the for, and tries to login again until the ctx was cancelled
				}
			}
		})
	}

	group.Wait()
}

// login tries to authorize the client at the remote server.
func (s *Service) login(conn *connWrapper, sendCh chan *StatsPayload) error {
	// Construct and send the login authentication
	infos := s.server.NodeInfo()

	var (
		network  string
		protocol string
	)
	if info := infos.Protocols[istanbul.ProtocolName]; info != nil {
		ethInfo, ok := info.(*eth.NodeInfo)
		if !ok {
			return errors.New("could not resolve NodeInfo")
		}
		network = fmt.Sprintf("%d", ethInfo.Network)
		protocol = fmt.Sprintf("%s/%d", istanbul.ProtocolName, istanbul.ProtocolVersions[0])
	} else {
		lesProtocol, ok := infos.Protocols["les"]
		if !ok {
			return errors.New("no LES protocol found")
		}
		lesInfo, ok := lesProtocol.(*les.NodeInfo)
		if !ok {
			return errors.New("could not resolve NodeInfo")
		}
		network = fmt.Sprintf("%d", lesInfo.Network)
		protocol = fmt.Sprintf("les/%d", les.ClientProtocolVersions[0])
	}
	auth := &authMsg{
		ID: s.istanbulBackend.ValidatorAddress().String(),
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

	if s.istanbulBackend.IsProxy() {
		// Proxy needs a delegate send of a hello action here to get ACK
		if err := s.waitAndDelegateMessageWithTimeout(conn, sendCh, actionHello); err != nil {
			return err
		}
	}

	// Retrieve the remote ack or connection termination
	var ack map[string][]string

	signalCh := make(chan error, 1)

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

func (s *Service) waitAndDelegateMessageWithTimeout(conn *connWrapper, sendCh chan *StatsPayload, action string) error {
	for {
		select {
		case signedMessage := <-sendCh:
			err := s.handleDelegateSend(conn, signedMessage)
			// The wait and delegate message, basically requires that some message was already returned.
			// It is possible to receive an old message signed that was queue before.
			// With this, we only continue if that message was present, otherwise we delegate that message
			// and continue
			if signedMessage.Action == action {
				return err
			} else {
				if err != nil {
					log.Warn("Delegate send failed", "err", err)
				}
			}
		case <-time.After(delegateSignTimeout * time.Second):
			return errors.New("delegation sign timeout")
		}
	}
}

var nodeNameRegex = regexp.MustCompile(`(.*)/(.*)/(.*)/(.*)`)

func (s *Service) fillWithValidatorInfo(message *StatsPayload) {
	if message.Action == actionHello {
		msg, ok := message.Stats.(map[string]interface{})
		if ok {
			proxyInfo, ok := msg["info"].(map[string]interface{})
			if ok {
				infos := s.server.NodeInfo()
				proxyNode := proxyInfo["node"].(string)
				proxyNodeParts := nodeNameRegex.FindStringSubmatch(proxyNode)
				validatorNodeParts := nodeNameRegex.FindStringSubmatch(infos.Name)
				// if one of the regex failed, maintain the proxy node name
				if proxyNodeParts != nil && validatorNodeParts != nil {
					proxyInfo["node"] = fmt.Sprintf(
						"%s/%s(val:%s)/%s/%s",
						proxyNodeParts[1],
						proxyNodeParts[2],
						validatorNodeParts[2],
						proxyNodeParts[3],
						proxyNodeParts[4],
					)
				}
			}
		}
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
	return s.istanbulBackend.SendDelegateSignMsgToProxy(msg, peerID)
}

func (s *Service) handleDelegateSend(conn *connWrapper, signedMessage *StatsPayload) error {
	report := map[string][]interface{}{
		"emit": {signedMessage.Action, signedMessage.Stats},
	}
	return conn.WriteJSON(report)
}

func (s *Service) handleDelegateSignEvents(ctx context.Context, sendCh chan *StatsPayload, signCh chan *DelegateSignMessage) error {
	ch := make(chan istanbul.MessageWithPeerIDEvent, istDelegateSignChanSize)
	subscription := s.istanbulBackend.SubscribeNewDelegateSignEvent(ch)
	defer subscription.Unsubscribe()

	for {
		select {
		case msg := <-ch:
			var delegateSignMessage DelegateSignMessage
			delegateSignMessage.PeerID = msg.PeerID
			if err := json.Unmarshal(msg.Payload, &delegateSignMessage.Payload); err != nil {
				continue
			}
			if s.istanbulBackend.IsProxy() {
				// proxy should send to websocket
				select {
				case sendCh <- &delegateSignMessage.Payload:
				default:
				}
			} else if s.istanbulBackend.IsProxiedValidator() {
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

func (s *Service) handleNewTransactionEvents(ctx context.Context, txChan chan struct{}) error {
	var lastTx mclock.AbsTime
	ch := make(chan core.NewTxsEvent, txChanSize)
	subscription := s.backend.SubscribeNewTxsEvent(ch)
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

func (s *Service) handleChainHeadEvents(ctx context.Context, headCh chan *types.Block) error {
	ch := make(chan core.ChainHeadEvent, chainHeadChanSize)
	subscription := s.backend.SubscribeChainHeadEvent(ch)
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
func (s *Service) readLoop(conn *connWrapper) {
	// If the read loop exists, close the connection
	defer conn.Close()

	for {
		// Retrieve the next generic network packet and bail out on error
		var blob json.RawMessage
		if err := conn.ReadJSON(&blob); err != nil {
			log.Warn("Failed to retrieve stats server message", "err", err)
			return
		}
		// If the network packet is a system ping, respond to it directly
		var ping string
		if err := json.Unmarshal(blob, &ping); err == nil && strings.HasPrefix(ping, "primus::ping::") {
			if err := conn.WriteJSON(strings.Replace(ping, "ping", "pong", -1)); err != nil {
				log.Warn("Failed to respond to system ping message", "err", err)
				return
			}
			continue
		}
		// Not a system ping, try to decode an actual state message
		var msg map[string]interface{}
		if err := json.Unmarshal(blob, &msg); err != nil {
			log.Warn("Failed to decode stats server message", "err", err)
			return
		}
		msgEmit, _ := msg["emit"].([]interface{})

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
				select {
				case s.histCh <- nil: // Treat it as an no indexes request
				default:
				}
				continue // Ethstats sometimes sends invalid history requests, ignore those
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
		// Report anything else and continue
		log.Info("Unknown stats message", "msg", msg)
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
func (s *Service) report(conn *connWrapper, sendCh chan *StatsPayload) error {
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
func (s *Service) reportLatency(conn *connWrapper, sendCh chan *StatsPayload) error {
	// Send the current time to the ethstats server
	start := time.Now()

	ping := map[string]interface{}{
		"id":         s.istanbulBackend.ValidatorAddress().String(),
		"clientTime": start.String(),
	}
	if err := s.sendStats(conn, actionNodePing, ping); err != nil {
		return err
	}
	// Proxy needs a delegate send of a node-ping action here to get ACK
	if s.istanbulBackend.IsProxy() {
		if err := s.waitAndDelegateMessageWithTimeout(conn, sendCh, actionNodePing); err != nil {
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
		"id":      s.istanbulBackend.ValidatorAddress().String(),
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
	validator := s.istanbulBackend.ValidatorAddress()

	account := accounts.Account{Address: validator}
	wallet, errWallet := s.backend.AccountManager().Find(account)
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
		"address":   validator,
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

func (s *Service) sendStats(conn *connWrapper, action string, stats interface{}) error {
	if s.istanbulBackend.IsProxy() {
		statsWithAction := map[string]interface{}{
			"stats":  stats,
			"action": action,
		}
		msg, err := json.Marshal(statsWithAction)
		if err != nil {
			return err
		}
		go func() {
			err := s.istanbulBackend.SendDelegateSignMsgToProxiedValidator(msg)
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
func (s *Service) reportBlock(conn *connWrapper, block *types.Block) error {
	// Gather the block details from the header or block chain
	details := s.assembleBlockStats(block)

	// Assemble the block report and send it to the server
	log.Trace("Sending new block to ethstats", "number", details.Number, "hash", details.Hash)

	stats := map[string]interface{}{
		"id":    s.istanbulBackend.ValidatorAddress().String(),
		"block": details,
	}
	return s.sendStats(conn, actionBlock, stats)
}

// assembleBlockStats retrieves any required metadata to report a single block
// and assembles the block stats. If block is nil, the current head is processed.
func (s *Service) assembleBlockStats(block *types.Block) *blockStats {
	// Gather the block infos from the local blockchain
	var (
		header   *types.Header
		stateDB  *state.StateDB
		vmRunner vm.EVMRunner
		td       *big.Int
		txs      []txStats
		valSet   validatorSet
	)

	// check if backend is a full node
	fullBackend, ok := s.backend.(fullNodeBackend)
	if ok {
		if block == nil {
			block = fullBackend.CurrentBlock()
		}
		header = block.Header()
		txs = make([]txStats, len(block.Transactions()))
		for i, tx := range block.Transactions() {
			txs[i].Hash = tx.Hash()
		}
	} else {
		// Light nodes would need on-demand lookups for transactions, skip
		if block != nil {
			header = block.Header()
		} else {
			header = s.backend.CurrentHeader()
		}
		txs = []txStats{}
	}
	td = s.backend.GetTd(context.Background(), header.Hash())
	stateDB, _, _ = s.backend.StateAndHeaderByNumberOrHash(context.Background(), rpc.BlockNumberOrHashWithHash(header.Hash(), true))
	vmRunner = s.backend.NewEVMRunner(header, stateDB)

	// Assemble and return the block stats
	author, _ := s.engine.Author(header)

	// Add epoch info
	epochSize := s.engine.EpochSize()
	blockRemain := epochSize - istanbul.GetNumberWithinEpoch(header.Number.Uint64(), epochSize)

	// only assemble every valSetInterval blocks
	if block != nil && block.Number().Uint64()%valSetInterval == 0 {
		valSet = s.assembleValidatorSet(block, stateDB)
	}

	gasLimit := blockchain_parameters.GetBlockGasLimitOrDefault(vmRunner)

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
		valSet         validatorSet
		valsRegistered []validatorInfo
		valsElected    []common.Address
	)

	vmRunner := s.backend.NewEVMRunner(block.Header(), state)

	// Add set of registered validators
	valsRegisteredMap, _ := validators.RetrieveRegisteredValidators(vmRunner)
	valsRegistered = make([]validatorInfo, 0, len(valsRegisteredMap))
	for _, address := range valsRegisteredMap {
		valData, err := validators.GetValidator(vmRunner, address)

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
	valsElectedList := s.istanbulBackend.GetValidators(block.Number(), block.Hash())

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
func (s *Service) reportHistory(conn *connWrapper, list []uint64) error {
	// Figure out the indexes that need reporting
	indexes := make([]uint64, 0, historyUpdateRange)
	if len(list) > 0 {
		// Specific indexes requested, send them back in particular
		indexes = append(indexes, list...)
	} else {
		// No indexes requested, send back the top ones
		head := s.backend.CurrentHeader().Number.Int64()
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
		fullBackend, ok := s.backend.(fullNodeBackend)
		// Retrieve the next block if it's known to us
		var block *types.Block
		if ok {
			block, _ = fullBackend.BlockByNumber(context.Background(), rpc.BlockNumber(number)) // TODO ignore error here ?
		} else {
			if header, _ := s.backend.HeaderByNumber(context.Background(), rpc.BlockNumber(number)); header != nil {
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
		"id":      s.istanbulBackend.ValidatorAddress().String(),
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
func (s *Service) reportPending(conn *connWrapper) error {
	// Retrieve the pending count from the local blockchain
	pending, _ := s.backend.Stats()
	// Assemble the transaction stats and send it to the server
	log.Trace("Sending pending transactions to ethstats", "count", pending)

	stats := map[string]interface{}{
		"id": s.istanbulBackend.ValidatorAddress().String(),
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
	Peers    int  `json:"peers"`
	GasPrice int  `json:"gasPrice"`
	Uptime   int  `json:"uptime"`
}

// reportStats retrieves various stats about the node at the networking and
// mining layer and reports it to the stats server.
func (s *Service) reportStats(conn *connWrapper) error {
	// Gather the syncing and mining infos from the local miner instance
	var (
		validatorAddress common.Address
		mining           bool
		proxy            bool
		elected          bool
		syncing          bool
		gasprice         int
	)
	// check if backend is a full node
	fullBackend, ok := s.backend.(fullNodeBackend)
	if ok {
		validatorAddress = s.istanbulBackend.ValidatorAddress()
		block := fullBackend.CurrentBlock()

		proxy = s.istanbulBackend.IsProxy()
		mining = fullBackend.Miner().Mining()

		elected = false
		valsElected := s.istanbulBackend.GetValidators(block.Number(), block.Hash())

		for i := range valsElected {
			if valsElected[i].Address() == validatorAddress {
				elected = true
			}
		}

		sync := fullBackend.Downloader().Progress()
		syncing = fullBackend.CurrentHeader().Number.Uint64() >= sync.HighestBlock

		price, _ := fullBackend.SuggestPrice(context.Background(), nil)
		gasprice = int(price.Uint64())
	} else {
		sync := s.backend.Downloader().Progress()
		syncing = s.backend.CurrentHeader().Number.Uint64() >= sync.HighestBlock
	}
	// Assemble the node stats and send it to the server
	log.Trace("Sending node details to ethstats")

	stats := map[string]interface{}{
		"id":      s.istanbulBackend.ValidatorAddress().String(),
		"address": validatorAddress,
		"stats": &nodeStats{
			Active:   true,
			Mining:   mining,
			Elected:  elected,
			Proxy:    proxy,
			Peers:    s.server.PeerCount(),
			GasPrice: gasprice,
			Syncing:  syncing,
			Uptime:   100,
		},
	}

	return s.sendStats(conn, actionStats, stats)
}
