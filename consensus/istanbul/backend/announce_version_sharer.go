package backend

import (
	"sync/atomic"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/announce"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

type AnnounceVersionSharer interface {
	// ShareVersion generates announce data structures and
	// and shares them with relevant nodes.
	// It will:
	//  1) Generate a new enode certificate
	//  2) Multicast the new enode certificate to all peers in the validator conn set
	//	   * Note: If this is a proxied validator, it's multicast message will be wrapped within a forward
	//       message to the proxy, which will in turn send the enode certificate to remote validators.
	//  3) Generate a new version certificate
	//  4) Gossip the new version certificate to all peers
	ShareVersion(version uint) error
}

type OnNewEnodeCertsMsgSentFn func(map[enode.ID]*istanbul.EnodeCertMsg) error

func NewAnnounceVersionSharer(
	aWallets *atomic.Value,
	network AnnounceNetwork,
	state *AnnounceState,
	ovcp OutboundVersionCertificateProcessor,
	ecertGenerator announce.EnodeCertificateMsgGenerator,
	ecertHolder announce.EnodeCertificateMsgHolder,
	onNewEnodeMsgsFn OnNewEnodeCertsMsgSentFn,
) AnnounceVersionSharer {
	// noop func
	var onNewMsgsFn OnNewEnodeCertsMsgSentFn = func(map[enode.ID]*istanbul.EnodeCertMsg) error { return nil }
	if onNewEnodeMsgsFn != nil {
		onNewMsgsFn = onNewEnodeMsgsFn
	}
	return &avs{
		logger:         log.New("module", "AnnounceVersionSharer"),
		aWallets:       aWallets,
		network:        network,
		state:          state,
		ovcp:           ovcp,
		ecertGenerator: ecertGenerator,
		ecertHolder:    ecertHolder,
		onNewMsgsFn:    onNewMsgsFn,
	}
}

type avs struct {
	logger log.Logger

	aWallets *atomic.Value

	network AnnounceNetwork

	state *AnnounceState

	ovcp OutboundVersionCertificateProcessor

	ecertGenerator announce.EnodeCertificateMsgGenerator
	ecertHolder    announce.EnodeCertificateMsgHolder

	onNewMsgsFn OnNewEnodeCertsMsgSentFn
}

func (a *avs) ShareVersion(version uint) error {
	// Send new versioned enode msg to all other registered or elected validators
	validatorConnSet, err := a.network.RetrieveValidatorConnSet()
	if err != nil {
		return err
	}

	w := a.aWallets.Load().(*istanbul.Wallets)

	// Don't send any of the following messages if this node is not in the validator conn set
	if !validatorConnSet[w.Ecdsa.Address] {
		a.logger.Trace("Not in the validator conn set, not updating announce version")
		return nil
	}

	enodeCertificateMsgs, err := a.ecertGenerator.GenerateEnodeCertificateMsgs(&w.Ecdsa, version)
	if err != nil {
		return err
	}

	if len(enodeCertificateMsgs) > 0 {
		if err := a.ecertHolder.Set(enodeCertificateMsgs); err != nil {
			a.logger.Error("Error in SetEnodeCertificateMsgMap", "err", err)
			return err
		}
	}

	valConnArray := make([]common.Address, 0, len(validatorConnSet))
	for address := range validatorConnSet {
		valConnArray = append(valConnArray, address)
	}

	for _, enodeCertMsg := range enodeCertificateMsgs {
		var destAddresses []common.Address
		if enodeCertMsg.DestAddresses != nil {
			destAddresses = enodeCertMsg.DestAddresses
		} else {
			// Send to all of the addresses in the validator connection set
			destAddresses = valConnArray
		}

		payload, err := enodeCertMsg.Msg.Payload()
		if err != nil {
			a.logger.Error("Error getting payload of enode certificate message", "err", err)
			return err
		}

		if err := a.network.Multicast(destAddresses, payload, istanbul.EnodeCertificateMsg, false); err != nil {
			return err
		}
	}

	a.onNewMsgsFn(enodeCertificateMsgs)

	// Generate and gossip a new version certificate
	newVersionCertificate, err := istanbul.NewVersionCertificate(version, w.Ecdsa.Sign)
	if err != nil {
		return err
	}
	return a.ovcp.Process(
		a.state,
		[]*istanbul.VersionCertificate{
			newVersionCertificate,
		},
		w.Ecdsa.Address,
	)
}
