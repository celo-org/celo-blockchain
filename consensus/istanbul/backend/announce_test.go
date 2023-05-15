package backend

import (
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/announce"
	"github.com/celo-org/celo-blockchain/rlp"
)

// This file is kept in the backend package since, while actually testing the announce protocol code, it requires
// several dependencies from the istanbul code.

// This test function will test the announce message generator and handler.
// It will also test the gossip query generator and handler.
func TestAnnounceGossipQueryMsg(t *testing.T) {
	t.Skip() // Flaky
	// Create three backends
	numValidators := 3
	genesisCfg, nodeKeys := getGenesisAndKeys(numValidators)

	chain0, engine0, _ := newBlockChainWithKeys(false, common.Address{}, false, genesisCfg, nodeKeys[0])
	defer chain0.Stop()
	chain1, engine1, _ := newBlockChainWithKeys(false, common.Address{}, false, genesisCfg, nodeKeys[1])
	defer chain1.Stop()
	chain2, engine2, _ := newBlockChainWithKeys(false, common.Address{}, false, genesisCfg, nodeKeys[2])
	defer chain2.Stop()

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
	vCert1, err := istanbul.NewVersionCertificate(engine1AnnounceVersion, engine1.Sign)
	if err != nil {
		t.Errorf("Error in generating version certificate for engine1.  Error: %v", err)
	}

	vCert2, err := istanbul.NewVersionCertificate(engine1AnnounceVersion, engine2.Sign)
	if err != nil {
		t.Errorf("Error in generating version certificate for engine2.  Error: %v", err)
	}

	// Have engine0 handle vCert messages from engine1 and engine2

	vCert1MsgPayload, err := istanbul.NewVersionCeritifcatesMessage([]*istanbul.VersionCertificate{vCert1}, engine1Address).Payload()
	if err != nil {
		t.Errorf("Error in encoding vCert1.  Error: %v", err)
	}
	err = engine0.announceManager.HandleVersionCertificatesMsg(common.Address{}, nil, vCert1MsgPayload)
	if err != nil {
		t.Errorf("Error in handling vCert1.  Error: %v", err)
	}

	vCert2MsgPayload, err := istanbul.NewVersionCeritifcatesMessage([]*istanbul.VersionCertificate{vCert2}, engine2Address).Payload()
	if err != nil {
		t.Errorf("Error in encoding vCert2.  Error: %v", err)
	}
	err = engine0.announceManager.HandleVersionCertificatesMsg(common.Address{}, nil, vCert2MsgPayload)
	if err != nil {
		t.Errorf("Error in handling vCert2.  Error: %v", err)
	}

	// Verify that engine0 will query for both engine1 and engine2's enodeURL
	qeep := announce.NewQueryEnodeEntryProvider(engine0.valEnodeTable)
	qeEntries, err := qeep.GetQueryEnodeValEnodeEntries(false, engine0.wallets().Ecdsa.Address)
	if err != nil {
		t.Errorf("Error in retrieving entries for queryEnode request")
	}

	if len(qeEntries) != 2 {
		t.Errorf("qeEntries size is incorrect.  Have: %d, Want: 2", len(qeEntries))
	}

	for _, expectedEntry := range []*istanbul.AddressEntry{{Address: engine1Address, HighestKnownVersion: engine1AnnounceVersion},
		{Address: engine2Address, HighestKnownVersion: engine2AnnounceVersion}} {
		found := false
		for _, qeEntry := range qeEntries {
			if qeEntry.Address == expectedEntry.Address && qeEntry.HighestKnownVersion == expectedEntry.HighestKnownVersion {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Didn't find expected entry in qeEntries.  Expected Entry: %v", expectedEntry)
		}
	}

	// Generate query enode message for engine0
	// TODO: refactor this test to remove the hard dependency on the generate&gossip fn
	wk := engine0.announceManager.Worker()
	qeMsg, err := wk.GenerateAndGossipQueryEnode(false)
	if err != nil {
		t.Errorf("Error in generating a query enode message.  Error: %v", err)
	}

	// Convert to payload
	qePayload, err := qeMsg.Payload()
	if err != nil {
		t.Errorf("Error in converting QueryEnode Message to payload.  Error: %v", err)
	}

	// Handle the qeMsg for both engine1 and engine2
	err = engine1.announceManager.HandleQueryEnodeMsg(engine0.Address(), nil, qePayload)
	if err != nil {
		t.Errorf("Error in handling query enode message for engine1.  Error: %v", err)
	}

	err = engine2.announceManager.HandleQueryEnodeMsg(engine0.Address(), nil, qePayload)
	if err != nil {
		t.Errorf("Error in handling query enode message for engine2.  Error: %v", err)
	}

	// Verify that engine1 and engine2 has engine0's entry in their val enode table
	expectedEntry := &istanbul.AddressEntry{Address: engine0Address, Node: engine0Enode, Version: engine0AnnounceVersion}

	entryMap, err := engine1.GetValEnodeTableEntries([]common.Address{engine0Address})
	if err != nil {
		t.Errorf("Error in retrieving val enode table entry from engine1.  Error: %v", err)
	}

	if entry := entryMap[engine0Address]; entry == nil || entry.Address != expectedEntry.Address || entry.Node.URLv4() != expectedEntry.Node.URLv4() || entry.Version != expectedEntry.Version {
		t.Errorf("Incorrect val enode table entry for engine0.  Want: %v, Have: %v", expectedEntry, entry)
	}

	entryMap, err = engine2.GetValEnodeTableEntries([]common.Address{engine0Address})
	if err != nil {
		t.Errorf("Error in retrieving val enode table entry from engine2.  Error: %v", err)
	}

	if entry := entryMap[engine0Address]; entry == nil || entry.Address != expectedEntry.Address || entry.PublicKey != expectedEntry.PublicKey || entry.Node.URLv4() != expectedEntry.Node.URLv4() || entry.Version != expectedEntry.Version || entry.HighestKnownVersion != expectedEntry.HighestKnownVersion {
		t.Errorf("Incorrect val enode table entry for engine0.  Want: %v, Have: %v", expectedEntry, entry)
	}

	engine0.StopAnnouncing()
	engine1.StopAnnouncing()
	engine2.StopAnnouncing()
}

// Test enode certificate generation (via the announce thread), and the handling of an enode certificate msg.
func TestHandleEnodeCertificateMsg(t *testing.T) {
	// Create two backends
	numValidators := 2
	genesisCfg, nodeKeys := getGenesisAndKeys(numValidators)

	chain0, engine0, _ := newBlockChainWithKeys(false, common.Address{}, false, genesisCfg, nodeKeys[0])
	defer chain0.Stop()
	chain1, engine1, _ := newBlockChainWithKeys(false, common.Address{}, false, genesisCfg, nodeKeys[1])
	defer chain1.Stop()

	engine0Node := engine0.SelfNode()

	// Wait for a bit for the enodeCert messages to be generated by engine0.  The announce thread should create that on startup
	time.Sleep(6 * time.Second)

	enodeCerts := engine0.RetrieveEnodeCertificateMsgMap()
	if enodeCerts == nil || enodeCerts[engine0Node.ID()] == nil {
		t.Errorf("No enode certificates generated for engine0")
	}
	enodeCertMsgPayload, _ := enodeCerts[engine0Node.ID()].Msg.Payload()

	// Handle the enodeCertMsg in engine1
	err := engine1.announceManager.HandleEnodeCertificateMsg(nil, enodeCertMsgPayload)
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

	engine0.StopAnnouncing()
	engine1.StopAnnouncing()
}

// This function will test the setAndShareUpdatedAnnounceVersion function.
// It will verify that this function creates correct enode certificates, and that
// the engine's announce version is updated.
func TestSetAndShareUpdatedAnnounceVersion(t *testing.T) {
	// Create one backend
	numValidators := 1
	genesisCfg, nodeKeys := getGenesisAndKeys(numValidators)

	chain, engine, _ := newBlockChainWithKeys(false, common.Address{}, false, genesisCfg, nodeKeys[0])
	defer chain.Stop()

	// Wait a bit so that the announce versions are generated for the engines
	time.Sleep(10 * time.Second)

	announceVersion := engine.GetAnnounceVersion() + 10000
	wrk := engine.announceManager.Worker()
	if err := wrk.UpdateVersionTo(announceVersion); err != nil {
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

	engine.StopAnnouncing()
}
