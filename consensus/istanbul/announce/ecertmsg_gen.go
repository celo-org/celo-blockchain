package announce

import (
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

type EnodeCertificateMsgGenerator interface {
	// GenerateEnodeCertificateMsgs generates a map of enode certificate messages.
	// One certificate message is generated for each external enode this node possesses generated for
	// each external enode this node possesses. A unproxied validator will have one enode, while a
	// proxied validator may have one for each proxy.. Each enode is a key in the returned map, and the
	// value is the certificate message.
	GenerateEnodeCertificateMsgs(ei *istanbul.EcdsaInfo, version uint) (map[enode.ID]*istanbul.EnodeCertMsg, error)
}

type ecmg struct {
	logger log.Logger
	efeg   ExternalFacingEnodeGetter
}

func NewEnodeCertificateMsgGenerator(efeg ExternalFacingEnodeGetter) EnodeCertificateMsgGenerator {
	return &ecmg{
		logger: log.New("module", "enodeCertificateMsgGenerator"),
		efeg:   efeg,
	}
}

func (e *ecmg) GenerateEnodeCertificateMsgs(ei *istanbul.EcdsaInfo, version uint) (map[enode.ID]*istanbul.EnodeCertMsg, error) {
	logger := e.logger.New("func", "generateEnodeCertificateMsgs")

	enodeCertificateMsgs := make(map[enode.ID]*istanbul.EnodeCertMsg)
	externalEnodes, valDestinations, err := e.efeg.GetEnodeCertNodesAndDestAddresses()
	if err != nil {
		return nil, err
	}

	for _, externalNode := range externalEnodes {
		msg := istanbul.NewEnodeCeritifcateMessage(
			&istanbul.EnodeCertificate{EnodeURL: externalNode.URLv4(), Version: version},
			ei.Address,
		)
		// Sign the message
		if err := msg.Sign(ei.Sign); err != nil {
			return nil, err
		}

		enodeCertificateMsgs[externalNode.ID()] = &istanbul.EnodeCertMsg{Msg: msg, DestAddresses: valDestinations[externalNode.ID()]}
	}

	logger.Trace("Generated Istanbul Enode Certificate messages", "enodeCertificateMsgs", enodeCertificateMsgs)
	return enodeCertificateMsgs, nil
}
