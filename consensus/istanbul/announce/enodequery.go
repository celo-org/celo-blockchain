package announce

import (
	"crypto/ecdsa"
	"crypto/rand"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/crypto/ecies"
	"github.com/celo-org/celo-blockchain/log"
)

type EnodeQuery struct {
	RecipientAddress   common.Address
	RecipientPublicKey *ecdsa.PublicKey
	EnodeURL           string
}

// generateEncryptedEnodeURLs returns the encryptedEnodeURLs to be sent in an enode query.
func generateEncryptedEnodeURLs(plogger log.Logger, enodeQueries []*EnodeQuery) ([]*istanbul.EncryptedEnodeURL, error) {
	logger := plogger.New("func", "generateEncryptedEnodeURLs")

	var encryptedEnodeURLs []*istanbul.EncryptedEnodeURL
	for _, param := range enodeQueries {
		logger.Debug("encrypting enodeURL", "externalEnodeURL", param.EnodeURL, "publicKey", param.RecipientPublicKey)
		publicKey := ecies.ImportECDSAPublic(param.RecipientPublicKey)
		encEnodeURL, err := ecies.Encrypt(rand.Reader, publicKey, []byte(param.EnodeURL), nil, nil)
		if err != nil {
			logger.Error("Error in encrypting enodeURL", "enodeURL", param.EnodeURL, "publicKey", publicKey)
			return nil, err
		}

		encryptedEnodeURLs = append(encryptedEnodeURLs, &istanbul.EncryptedEnodeURL{
			DestAddress:       param.RecipientAddress,
			EncryptedEnodeURL: encEnodeURL,
		})
	}

	return encryptedEnodeURLs, nil
}

// generateQueryEnodeMsg returns a queryEnode message from this node with a given version.
// A query enode message contains a number of individual enode queries, each of which is intended
// for a single recipient validator. A query contains of this nodes external enode URL, to which
// the recipient validator is intended to connect, and is ECIES encrypted with the recipient's
// public key, from which their validator signer address is derived.
// Note: It is referred to as a "query" because the sender does not know the recipients enode.
// The recipient is expected to respond by opening a direct connection with an enode certificate.
func generateQueryEnodeMsg(plogger log.Logger, ei *istanbul.EcdsaInfo, version uint, enodeQueries []*EnodeQuery) (*istanbul.Message, error) {
	logger := plogger.New("func", "generateQueryEnodeMsg")

	encryptedEnodeURLs, err := generateEncryptedEnodeURLs(logger, enodeQueries)
	if err != nil {
		logger.Warn("Error generating encrypted enodeURLs", "err", err)
		return nil, err
	}
	if len(encryptedEnodeURLs) == 0 {
		logger.Trace("No encrypted enodeURLs were generated, will not generate encryptedEnodeMsg")
		return nil, nil
	}

	msg := istanbul.NewQueryEnodeMessage(&istanbul.QueryEnodeData{
		EncryptedEnodeURLs: encryptedEnodeURLs,
		Version:            version,
		Timestamp:          istanbul.GetTimestamp(),
	}, ei.Address)
	// Sign the announce message
	if err := msg.Sign(ei.Sign); err != nil {
		logger.Error("Error in signing a QueryEnode Message", "QueryEnodeMsg", msg.String(), "err", err)
		return nil, err
	}

	logger.Debug("Generated a queryEnode message", "IstanbulMsg", msg.String(), "QueryEnodeData", msg.QueryEnodeMsg().String())

	return msg, nil
}

type EnodeQueryGossiper interface {
	// GossipEnodeQueries will generate, encrypt, and gossip through the p2p network a new
	// QueryEnodeMsg with the enodeQueries given.
	GossipEnodeQueries(*istanbul.EcdsaInfo, []*EnodeQuery) (*istanbul.Message, error)
}

type eqg struct {
	logger          log.Logger
	announceVersion VersionReader
	gossip          func([]byte) error
}

func NewEnodeQueryGossiper(announceVersion VersionReader, gossipFn func([]byte) error) EnodeQueryGossiper {
	return &eqg{
		logger:          log.New("module", "enodeQueryGossiper"),
		announceVersion: announceVersion,
		gossip:          gossipFn,
	}
}

func (e *eqg) GossipEnodeQueries(ei *istanbul.EcdsaInfo, enodeQueries []*EnodeQuery) (*istanbul.Message, error) {
	version := e.announceVersion.Get()
	var err error
	qeMsg, err := generateQueryEnodeMsg(e.logger, ei, version, enodeQueries)
	if err != nil {
		return nil, err
	}

	if qeMsg == nil {
		return nil, nil
	}

	// Convert to payload
	payload, err := qeMsg.Payload()
	if err != nil {
		e.logger.Error("Error in converting Istanbul QueryEnode Message to payload", "QueryEnodeMsg", qeMsg.String(), "err", err)
		return nil, err
	}

	if err = e.gossip(payload); err != nil {
		return nil, err
	}
	return qeMsg, nil
}
