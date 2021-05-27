package backend

import (
	"crypto/ecdsa"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/rlp"
	. "github.com/onsi/gomega"
)

// This test function will test the announce message generator and handler.
// It will also test the gossip query generator and handler.
func TestAnnounceGossipQueryMsg(t *testing.T) {
	withManyEngines(manyValidators(3), true, func(n []testNode, keys []*ecdsa.PrivateKey) {
		g := NewGomegaWithT(t)

		engine0, engine1, engine2 := n[0].engine, n[1].engine, n[2].engine

		// Wait a bit so that the announce versions are generated for the engines
		time.Sleep(6 * time.Second)

		engine0Address := engine0.Address()
		engine1Address := engine1.Address()
		engine2Address := engine2.Address()

		engine0AnnounceVersion := engine0.GetAnnounceVersion()
		engine1AnnounceVersion := engine1.GetAnnounceVersion()
		engine2AnnounceVersion := engine2.GetAnnounceVersion()

		engine0Enode := engine0.SelfNode()

		// Create version certificate messages for engine1 and engine2, so that engine0 will send a queryEnodeMessage to them
		vCert1, err := engine1.generateVersionCertificate(engine1AnnounceVersion)
		g.Expect(err).ToNot(HaveOccurred())

		vCert2, err := engine2.generateVersionCertificate(engine2AnnounceVersion)
		g.Expect(err).ToNot(HaveOccurred())

		// Have engine0 handle vCert messages from engine1 and engine2
		vCert1MsgPayload, err := engine1.encodeVersionCertificatesMsg([]*versionCertificate{vCert1})
		g.Expect(err).ToNot(HaveOccurred())

		err = engine0.handleVersionCertificatesMsg(common.Address{}, nil, vCert1MsgPayload)
		g.Expect(err).ToNot(HaveOccurred())

		vCert2MsgPayload, err := engine2.encodeVersionCertificatesMsg([]*versionCertificate{vCert2})
		g.Expect(err).ToNot(HaveOccurred())

		err = engine0.handleVersionCertificatesMsg(common.Address{}, nil, vCert2MsgPayload)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify that engine0 will query for both engine1 and engine2's enodeURL
		qeEntries, err := engine0.getQueryEnodeValEnodeEntries(false)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(qeEntries).To(HaveLen(2))

		// expctedEntries := []*istanbul.AddressEntry{
		// 	{Address: engine1Address, HighestKnownVersion: engine1AnnounceVersion},
		// 	{Address: engine2Address, HighestKnownVersion: engine2AnnounceVersion},
		// }
		msgFrom := func(expectedAddress common.Address, expectedVersion uint) func(entry *istanbul.AddressEntry) bool {
			return func(entry *istanbul.AddressEntry) bool {
				return entry.Address == expectedAddress && entry.HighestKnownVersion == expectedVersion
			}
		}

		g.Expect(qeEntries).To(ContainElement(Satisfy(msgFrom(engine1Address, engine1AnnounceVersion))))
		g.Expect(qeEntries).To(ContainElement(Satisfy(msgFrom(engine2Address, engine2AnnounceVersion))))

		// for _, expectedEntry := range []*istanbul.AddressEntry{,
		// 	} {

		// 	found := false
		// 	for _, qeEntry := range qeEntries {
		// 		if qeEntry.Address == expectedEntry.Address && qeEntry.HighestKnownVersion == expectedEntry.HighestKnownVersion {
		// 			found = true
		// 			break
		// 		}
		// 	}

		// 	if !found {
		// 		t.Errorf("Didn't find expected entry in qeEntries.  Expected Entry: %v", expectedEntry)
		// 	}
		// }

		// Generate query enode message for engine0
		qeMsg, err := engine0.generateAndGossipQueryEnode(engine0AnnounceVersion, false)
		g.Expect(err).ToNot(HaveOccurred())

		// Convert to payload
		qePayload, err := qeMsg.Payload()
		g.Expect(err).ToNot(HaveOccurred())

		// Handle the qeMsg for both engine1 and engine2
		err = engine1.handleQueryEnodeMsg(engine0.Address(), nil, qePayload)
		g.Expect(err).ToNot(HaveOccurred())

		err = engine2.handleQueryEnodeMsg(engine0.Address(), nil, qePayload)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify that engine1 and engine2 has engine0's entry in their val enode table
		expectedEntry := &istanbul.AddressEntry{Address: engine0Address, Node: engine0Enode, Version: engine0AnnounceVersion}

		entryMap, err := engine1.GetValEnodeTableEntries([]common.Address{engine0Address})
		g.Expect(err).ToNot(HaveOccurred())

		if entry := entryMap[engine0Address]; entry == nil || entry.Address != expectedEntry.Address || entry.Node.URLv4() != expectedEntry.Node.URLv4() || entry.Version != expectedEntry.Version {
			t.Errorf("Incorrect val enode table entry for engine0.  Want: %v, Have: %v", expectedEntry, entry)
		}

		entryMap, err = engine2.GetValEnodeTableEntries([]common.Address{engine0Address})
		g.Expect(err).ToNot(HaveOccurred())

		if entry := entryMap[engine0Address]; entry == nil || entry.Address != expectedEntry.Address || entry.PublicKey != expectedEntry.PublicKey || entry.Node.URLv4() != expectedEntry.Node.URLv4() || entry.Version != expectedEntry.Version || entry.HighestKnownVersion != expectedEntry.HighestKnownVersion {
			t.Errorf("Incorrect val enode table entry for engine0.  Want: %v, Have: %v", expectedEntry, entry)
		}
	})
}

// Test enode certificate generation (via the announce thread), and the handling of an enode certificate msg.
func TestHandleEnodeCertificateMsg(t *testing.T) {
	// Create two backends
	numValidators := 2
	genesisCfg, nodeKeys := generateGenesisAndKeys(numValidators)

	node0 := testNodeFromGenesis(Validator, common.Address{}, genesisCfg, nodeKeys[0])
	defer node0.startAndStop()()
	node1 := testNodeFromGenesis(Validator, common.Address{}, genesisCfg, nodeKeys[1])
	defer node1.startAndStop()()

	engine0, engine1 := node0.engine, node1.engine

	engine0Node := engine0.SelfNode()

	// Wait for a bit for the enodeCert messages to be generated by engine0.  The announce thread should create that on startup
	time.Sleep(6 * time.Second)

	enodeCerts := engine0.RetrieveEnodeCertificateMsgMap()
	if enodeCerts == nil || enodeCerts[engine0Node.ID()] == nil {
		t.Errorf("No enode certificates generated for engine0")
	}
	enodeCertMsgPayload, _ := enodeCerts[engine0Node.ID()].Msg.Payload()

	// Handle the enodeCertMsg in engine1
	err := engine1.handleEnodeCertificateMsg(nil, enodeCertMsgPayload)
	if err != nil {
		t.Errorf("Error in handling an enode certificate message. Error: %v", err)
	}

	// Verify that the enode certificate is saved in the val enode table via GetValEnodeTableEntries
	vetEntryMap, err := engine1.GetValEnodeTableEntries([]common.Address{engine0.Address()})
	if err != nil {
		t.Errorf("Error in retrieving val enode table entires.  Error: %v", err)
	}

	if vetEntryMap == nil || vetEntryMap[engine0.Address()] == nil {
		t.Errorf("Missing val enode table entry for engine0")
	}

	engine0VetEntry := vetEntryMap[engine0.Address()]
	if engine0VetEntry.Address != engine0.Address() {
		t.Errorf("Engine0's val enode table entry's address is incorrect.  Want: %v, Have: %v", engine0.Address(), engine0VetEntry.Address)
	}

	if engine0VetEntry.Version != engine0.GetAnnounceVersion() {
		t.Errorf("Engine0's val enode table entry's version is incorrect.  Want: %d, Have: %d", engine0.GetAnnounceVersion(), engine0VetEntry.Version)
	}
}

// This function will test the setAndShareUpdatedAnnounceVersion function.
// It will verify that this function creates correct enode certificates, and that
// the engine's announce version is updated.
func TestSetAndShareUpdatedAnnounceVersion(t *testing.T) {
	// Create one backend
	numValidators := 1
	genesisCfg, nodeKeys := generateGenesisAndKeys(numValidators)

	node := testNodeFromGenesis(Validator, common.Address{}, genesisCfg, nodeKeys[0])
	defer node.startAndStop()()
	engine := node.engine

	// Wait a bit so that the announce versions are generated for the engines
	time.Sleep(10 * time.Second)

	announceVersion := engine.GetAnnounceVersion() + 10000
	if err := engine.setAndShareUpdatedAnnounceVersion(announceVersion); err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// Verify that enode certificate map is set via RetrieveEnodeCertificateMsgMap
	enodeCertMsgs := engine.RetrieveEnodeCertificateMsgMap()

	// For a standalone validator, there should be only one entry in the enodeCert Map.
	// TODO.  Add test case for when the engine has proxies
	selfEnode := engine.SelfNode()
	enodeCertMsg := enodeCertMsgs[selfEnode.ID()]

	if enodeCertMsg == nil {
		t.Errorf("unassigned enode certificate")
	}

	msgPayload, _ := enodeCertMsg.Msg.Payload()

	// Verify the actual message
	var msg istanbul.Message
	// Decode payload into msg
	err := msg.FromPayload(msgPayload, istanbul.GetSignatureAddress)
	if err != nil {
		t.Errorf("Error in decoding received Istanbul Enode Certificate message. Error: %v", err)
	}

	// Verify the msg sender
	if msg.Address != engine.Address() {
		t.Errorf("Incorrect msg sender for enode cert msg. Want: %v, Have: %v", engine.Address(), msg.Address)
	}

	var enodeCertificate istanbul.EnodeCertificate
	if err := rlp.DecodeBytes(msg.Msg, &enodeCertificate); err != nil {
		t.Errorf("Error in decoding received Istanbul Enode Certificate message content. Error: %v", err)
	}

	if enodeCertificate.EnodeURL != selfEnode.URLv4() {
		t.Errorf("Incorrect enodeURL in the enode certificate.  Want: %s, Have: %s", selfEnode.URLv4(), enodeCertificate.EnodeURL)
	}

	if enodeCertificate.Version != announceVersion {
		t.Errorf("Incorrect version in the enode certificate.  Want: %d, Have %d", announceVersion, enodeCertificate.Version)
	}

	// Verify that the dest address for the enode cert is nil.
	if enodeCertMsg.DestAddresses != nil {
		t.Errorf("Enode cert dest addresses is not nil")
	}
}
