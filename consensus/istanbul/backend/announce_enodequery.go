package backend

import (
	"crypto/ecdsa"
	"crypto/rand"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/crypto/ecies"
	"github.com/celo-org/celo-blockchain/log"
)

type enodeQuery struct {
	recipientAddress   common.Address
	recipientPublicKey *ecdsa.PublicKey
	enodeURL           string
}

// generateEncryptedEnodeURLs returns the encryptedEnodeURLs to be sent in an enode query.
func generateEncryptedEnodeURLs(plogger log.Logger, enodeQueries []*enodeQuery) ([]*istanbul.EncryptedEnodeURL, error) {
	logger := plogger.New("func", "generateEncryptedEnodeURLs")

	var encryptedEnodeURLs []*istanbul.EncryptedEnodeURL
	for _, param := range enodeQueries {
		logger.Debug("encrypting enodeURL", "externalEnodeURL", param.enodeURL, "publicKey", param.recipientPublicKey)
		publicKey := ecies.ImportECDSAPublic(param.recipientPublicKey)
		encEnodeURL, err := ecies.Encrypt(rand.Reader, publicKey, []byte(param.enodeURL), nil, nil)
		if err != nil {
			logger.Error("Error in encrypting enodeURL", "enodeURL", param.enodeURL, "publicKey", publicKey)
			return nil, err
		}

		encryptedEnodeURLs = append(encryptedEnodeURLs, &istanbul.EncryptedEnodeURL{
			DestAddress:       param.recipientAddress,
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
func generateQueryEnodeMsg(plogger log.Logger, ei *EcdsaInfo, version uint, enodeQueries []*enodeQuery) (*istanbul.Message, error) {
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
		Timestamp:          getTimestamp(),
	}, ei.Address)
	// Sign the announce message
	if err := msg.Sign(ei.Sign); err != nil {
		logger.Error("Error in signing a QueryEnode Message", "QueryEnodeMsg", msg.String(), "err", err)
		return nil, err
	}

	logger.Debug("Generated a queryEnode message", "IstanbulMsg", msg.String(), "QueryEnodeData", msg.QueryEnodeMsg().String())

	return msg, nil
}
