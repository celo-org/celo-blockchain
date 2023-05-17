package announce

import (
	"errors"
	"sync"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

var (
	errInvalidEnodeCertMsgMapInconsistentVersion = errors.New("invalid enode certificate message map because of inconsistent version")
)

type EnodeCertificateMsgHolder interface {
	// Get gets the most recent enode certificate messages.
	// May be nil if no message was generated as a result of the core not being
	// started
	Get() map[enode.ID]*istanbul.EnodeCertMsg
	Set(enodeCertMsgMap map[enode.ID]*istanbul.EnodeCertMsg) error
}

type lockedHolder struct {
	logger log.Logger
	// The enode certificate message map contains the most recently generated
	// enode certificates for each external node ID (e.g. will have one entry if it's a standalone validator).
	// Used for proving itself as a validator in the handshake for externally exposed nodes
	enodeCertificateMsgMap     map[enode.ID]*istanbul.EnodeCertMsg
	enodeCertificateMsgVersion uint
	enodeCertificateMsgMapMu   sync.RWMutex // This protects both enodeCertificateMsgMap and enodeCertificateMsgVersion
}

func NewLockedHolder() EnodeCertificateMsgHolder {
	return &lockedHolder{
		logger: log.New("module", "lockedHolder"),
	}
}

// RetrieveEnodeCertificateMsgMap gets the most recent enode certificate messages.
// May be nil if no message was generated as a result of the core not being
// started
func (h *lockedHolder) Get() map[enode.ID]*istanbul.EnodeCertMsg {
	h.enodeCertificateMsgMapMu.Lock()
	defer h.enodeCertificateMsgMapMu.Unlock()
	return h.enodeCertificateMsgMap
}

func (h *lockedHolder) Set(enodeCertMsgMap map[enode.ID]*istanbul.EnodeCertMsg) error {
	logger := h.logger.New("func", "Set")
	var enodeCertVersion *uint

	// Verify that all of the certificates have the same version
	for _, enodeCertMsg := range enodeCertMsgMap {
		enodeCert := enodeCertMsg.Msg.EnodeCertificate()

		if enodeCertVersion == nil {
			enodeCertVersion = &enodeCert.Version
		} else {
			if enodeCert.Version != *enodeCertVersion {
				logger.Error("enode certificate messages within enode certificate msgs array don't all have the same version")
				return errInvalidEnodeCertMsgMapInconsistentVersion
			}
		}
	}

	h.enodeCertificateMsgMapMu.Lock()
	defer h.enodeCertificateMsgMapMu.Unlock()

	// Already have a more recent enodeCertificate
	if *enodeCertVersion < h.enodeCertificateMsgVersion {
		logger.Error("Ignoring enode certificate msgs since it's an older version", "enodeCertVersion", *enodeCertVersion, "sb.enodeCertificateMsgVersion", h.enodeCertificateMsgVersion)
		return istanbul.ErrInvalidEnodeCertMsgMapOldVersion
	} else if *enodeCertVersion == h.enodeCertificateMsgVersion {
		// This function may be called with the same enode certificate.
		logger.Trace("Attempting to set an enode certificate with the same version as the previous set enode certificate's")
	} else {
		logger.Debug("Setting enode certificate", "version", *enodeCertVersion)
		h.enodeCertificateMsgMap = enodeCertMsgMap
		h.enodeCertificateMsgVersion = *enodeCertVersion
	}

	return nil
}
