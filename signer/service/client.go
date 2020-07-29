// Package les implements the Signer-only Ethereum subprotocol
package service

import (
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rpc"
)

type SignerEthereum struct {
	netRPCService  *ethapi.PublicNetAPI
	accountManager *accounts.Manager
	config         *eth.Config

	ApiBackend *SignerApiBackend
}

func New(ctx *node.ServiceContext, config *eth.Config) (*SignerEthereum, error) {
	seth := &SignerEthereum{
		config:         config,
		accountManager: ctx.AccountManager,
	}
	seth.ApiBackend = &SignerApiBackend{ctx.ExtRPCEnabled(), seth}
	return seth, nil
}

func (s *SignerEthereum) APIs() []rpc.API {
	return ethapi.GetAPIs(s.ApiBackend)
}


func (s *SignerEthereum) Protocols() []p2p.Protocol {
	return []p2p.Protocol{}
}

// Start implements node.Service, starting all internal goroutines needed by the
// light ethereum protocol implementation.
func (s *SignerEthereum) Start(srvr *p2p.Server) error {
	log.Warn("Signer-only mode is an experimental feature")
	s.netRPCService = ethapi.NewPublicNetAPI(srvr, s.config.NetworkId)
	return nil
}

func (s *SignerEthereum) Stop() error {
	return nil
}
