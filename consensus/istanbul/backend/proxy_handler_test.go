package backend

import (
	"bytes"
	"crypto/ecdsa"
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestProxySet(t *testing.T) {
	// Using math/rand to keep rng deterministic for tests
	rng := rand.New(rand.NewSource(0))

	// Testing with consistentHashingPolicy but keeping all tests as
	// implementation-agnostic as possible
	ps := newProxySet(newConsistentHashingPolicy())
	proxyNodes := newProxyNodes(rng)
	ps.addProxy(proxyNodes)

	// Testing that adding a single proxy results in an entry in proxiesByID
	if len(ps.proxiesByID) != 1 {
		t.Errorf("ps.addProxy() should first result in a single entry in proxiesByID")
	}

	// Testing that getting a proxy works as intended
	proxy := ps.getProxy(proxyNodes.InternalNode.ID())
	if proxy == nil || proxy.internalNode != proxyNodes.InternalNode || proxy.externalNode != proxyNodes.ExternalNode {
		t.Errorf("ps.getProxy() did not get the correct proxy")
	}

	// Testing that adding a validator while there are no proxies that are peered with
	// will not result in an assignment but will add the validator to the valAssignments
	fooVal := common.BytesToAddress([]byte("foo"))
	validators := make(map[common.Address]bool)
	validators[fooVal] = true
	ps.addValidators(validators)
	va := ps.valAssigner.getValAssignments()
	if value, ok := va.valToProxy[fooVal]; value != nil || !ok {
		t.Errorf("ps.addValidators() without any proxy peers did not result in validators added but no assignments")
	}

	// Testing that adding a proxy peer works as intended
	peer := &testPeer{}
	ps.addProxyPeer(proxy.ID(), peer)
	proxy = ps.getProxy(proxyNodes.InternalNode.ID())
	if proxy.peer == nil {
		t.Errorf("ps.addProxyPeer() did not set the peer")
	}

	// Testing that there will now be an assignment as a result of the proxy peer
	// being added
	va = ps.valAssigner.getValAssignments()
	if va.valToProxy[fooVal] == nil || !bytes.Equal(va.valToProxy[fooVal].Bytes(), proxy.ID().Bytes()) {
		t.Errorf("ps.addProxyPeer() did not result in an assignment")
	}

	// Adding another validator for more testing
	barVal := common.BytesToAddress([]byte("bar"))
	validators[barVal] = true
	ps.addValidators(validators)

	// Testing that getValidatorProxies will return all validator assignments if validators is nil
	proxies := ps.getValidatorProxies(nil)
	// Should be two entries, one for fooVal and barVal
	if len(proxies) != 2 {
		t.Errorf("ps.getValidatorProxies() did not return all assignments when validators is nil")
	}
	// Both validators should be assigned to the same proxy
	for _, p := range proxies {
		if p.ID() != proxy.ID() {
			t.Errorf("ps.getValidatorProxies() did not return the correct assignments with nil validators")
		}
	}

	// Testing that getValidatorProxies will return the proxy for a specified validator
	fooValMap := make(map[common.Address]bool)
	fooValMap[fooVal] = true
	proxies = ps.getValidatorProxies(fooValMap)
	if len(proxies) != 1 || !bytes.Equal(proxies[fooVal].ID().Bytes(), proxy.ID().Bytes()) {
		t.Errorf("ps.getValidatorProxies did not return the correct assignments with a specified validator")
	}

	// Adding another proxy for more testing
	proxyNodes1 := newProxyNodes(rng)
	ps.addProxy(proxyNodes1)
	proxy1 := ps.getProxy(proxyNodes1.InternalNode.ID())
	peer1 := &testPeer{}
	ps.addProxyPeer(proxy1.ID(), peer1)

	// Testing that getValidatorProxyPeers will return all proxy peers if validators is nil
	peers := ps.getValidatorProxyPeers(nil)
	// Should be two entries, one for fooVal and barVal
	if len(peers) != 2 {
		t.Errorf("ps.getValidatorProxyPeers() did not return all peers when validators is nil")
	}
	// Both validators should be assigned to the same proxy
	for _, p := range peers {
		if p == nil {
			t.Errorf("ps.getValidatorProxyPeers() did not return the correct peers with nil validators")
		}
	}

	// Testing that getValidatorProxyPeers will return all  the peer for a specified validator
	peers = ps.getValidatorProxyPeers([]common.Address{fooVal})
	if len(peers) != 1 {
		t.Errorf("ps.getValidatorProxyPeers did not return the correct peer with a specified validator")
	}

	oldDcTimestamp := proxy.dcTimestamp
	// Testing that removing a proxy peer works as intended
	ps.removeProxyPeer(proxy.ID())
	proxy = ps.getProxy(proxyNodes.InternalNode.ID())
	if proxy.peer != nil || !proxy.dcTimestamp.After(oldDcTimestamp) {
		t.Errorf("ps.removeProxyPeer() did not remove the peer or the dcTimestamp was not updated. oldDcTimestamp: %v proxy.dcTimestamp: %v", oldDcTimestamp, proxy.dcTimestamp)
	}

	// Testing that removing proxies works as intended
	ps.removeProxy(proxy.ID())
	ps.removeProxy(proxy1.ID())
	va = ps.valAssigner.getValAssignments()
	proxy = ps.getProxy(proxyNodes.InternalNode.ID())
	if len(ps.proxiesByID) != 0 || proxy != nil || va.valToProxy[fooVal] != nil {
		t.Errorf("ps.removeProxy did not remove the proxy or did not unassign the validator %v %v %v", len(ps.proxiesByID), proxy != nil, va.valToProxy[fooVal])
	}
}

func TestConsistentHashingPolicy(t *testing.T) {
	// Using math/rand to keep rng deterministic for tests
	rng := rand.New(rand.NewSource(0))

	ch := newConsistentHashingPolicy()

	// Testing that the valAssignments are empty at first
	va := ch.getValAssignments()
	if !valAssignmentsIsEmpty(va) {
		t.Errorf("ch.getValAssignments() should initially be empty")
	}

	// Testing that reassignValidators should do nothing when valAssignments is empty
	ch.reassignValidators()
	va = ch.getValAssignments()
	if !valAssignmentsIsEmpty(va) {
		t.Errorf("ch.reassignValidators() should still result in an empty valAssignments")
	}

	// Testing that adding validators without any proxies will not result in any assignments
	fooVal := common.BytesToAddress([]byte("foo"))
	validators := make(map[common.Address]bool)
	validators[fooVal] = true

	ch.addValidators(validators)
	va = ch.getValAssignments()
	if len(va.getValidators()) != 1 || len(va.proxyToVals) != 0 {
		t.Errorf("ch.addValidators() with no proxies should not result in assignments")
	}

	// Testing that adding a proxy will now result in an assignment to the previously added
	// validator
	fooProxy := newProxy(rng)
	ch.addProxy(fooProxy)
	if len(va.proxyToVals) != 1 || !eqValidators(va.proxyToVals[fooProxy.ID()], validators) {
		t.Errorf("ch.addProxy() should assign any previously added validators")
	}

	// Testing that adding another validator will result in it being assigned to fooProxy
	barVal := common.BytesToAddress([]byte("bar"))
	newValidators := make(map[common.Address]bool)
	newValidators[barVal] = true

	ch.addValidators(newValidators)
	validators[barVal] = true
	if len(va.proxyToVals) != 1 || !eqValidators(va.proxyToVals[fooProxy.ID()], validators) {
		t.Errorf("ch.addProxy() should assign new validators to the only available proxy")
	}

	// Testing that multiple proxies will have validators distributed amongst them
	nProxies := 10
	proxies := make([]*proxy, nProxies)
	for i := 0; i < nProxies; i++ {
		proxies[i] = newProxy(rng)
		ch.addProxy(proxies[i])
	}
	nVals := 100
	newValidators = make(map[common.Address]bool)
	for i := 0; i < nVals; i++ {
		addrBytes := make([]byte, 32)
		rng.Read(addrBytes)
		addr := common.BytesToAddress(addrBytes)
		newValidators[addr] = true
	}
	ch.addValidators(newValidators)

	testValAssignmentsDistribution(t, "Adding multiple proxies and validators", ch.getValAssignments(), nVals/nProxies, 0.7)

	// Testing that removing half the proxies will reassign validators as expected
	nProxiesRemoved := nProxies / 2
	for i := 0; i < nProxiesRemoved; i++ {
		ch.removeProxy(proxies[i])
	}
	nProxies -= nProxiesRemoved
	testValAssignmentsDistribution(t, "Removing half the proxies", ch.getValAssignments(), nVals/nProxies, 0.7)

	// Testing that removing half the validators will reassign validators as expected
	nValsRemoved := nVals / 2
	i := 0
	for val := range newValidators {
		if i >= nValsRemoved {
			break
		}
		delete(newValidators, val)
		i++
	}
	ch.removeValidators(newValidators)
	nVals -= nValsRemoved
	testValAssignmentsDistribution(t, "Removing half the validators", ch.getValAssignments(), nVals/nProxies, 0.7)
}

func TestValAssignments(t *testing.T) {
	va := newValAssignments()

	// Testing that adding a single validator works correctly

	validators := make(map[common.Address]bool)
	// add one address to validators
	fooVal := common.BytesToAddress([]byte("foo"))
	validators[fooVal] = true

	va.addValidators(validators)

	// va.getValidators() should return a map identical to validators
	if !eqValidators(validators, va.getValidators()) {
		t.Errorf("va.getValidators() did not return the correct validators when adding a single validator")
	}

	// Testing that adding multiple validators works correctly, and that any
	// existing validators cannot be added as duplicates
	barVal := common.BytesToAddress([]byte("bar"))
	bazVal := common.BytesToAddress([]byte("baz"))
	validators[barVal] = true
	validators[bazVal] = true

	va.addValidators(validators)

	// va.getValidators() should return a map still identical to validators
	if !eqValidators(validators, va.getValidators()) {
		t.Errorf("va.getValidators() did not return the correct validators when adding multiple validator")
	}

	// Testing that removing a single validator works correctly
	validatorsToDelete := make(map[common.Address]bool)
	validatorsToDelete[fooVal] = true

	va.removeValidators(validatorsToDelete)

	delete(validators, fooVal)
	if !eqValidators(validators, va.getValidators()) {
		t.Errorf("va.getValidators() did not return the correct validators when removing a single validator")
	}

	// Testing that removing a multiple validators works correctly
	validatorsToDelete[barVal] = true
	validatorsToDelete[bazVal] = true

	va.removeValidators(validatorsToDelete)

	delete(validators, barVal)
	delete(validators, bazVal)
	if !eqValidators(validators, va.getValidators()) {
		t.Errorf("va.getValidators() did not return the correct validators when removing multiple validators")
	}

	// Testing that assigning a validator works

	validators = make(map[common.Address]bool)

	fooProxy := enode.HexID("f000000000000000000000000000000000000000000000000000000000000000")
	va.assignValidator(fooVal, fooProxy)
	validators[fooVal] = true
	// check the assignment is reflected in both valToProxy and proxyToVals
	if *(va.valToProxy[fooVal]) != fooProxy || !eqValidators(va.proxyToVals[fooProxy], validators) {
		t.Errorf("va.assignValidator() did not correctly assign a validator to a proxy")
	}

	// Testing that unassignValidator a validator works
	va.unassignValidator(fooVal)
	if val, ok := va.valToProxy[fooVal]; val != nil || !ok || len(va.proxyToVals[fooProxy]) != 0 {
		t.Errorf("va.unassignValidator() did not correctly unassign a validator from its proxy")
	}
}

type testPeer struct{}

func (tp *testPeer) Send(msgcode uint64, data interface{}) error { return nil }
func (tp *testPeer) Node() *enode.Node                           { return nil }

func newProxy(rng *rand.Rand) *proxy {
	pNodes := newProxyNodes(rng)
	return &proxy{
		internalNode: pNodes.InternalNode,
		externalNode: pNodes.ExternalNode,
	}
}

func newProxyNodes(rng *rand.Rand) *istanbul.ProxyNodes {
	return &istanbul.ProxyNodes{
		InternalNode: enode.NewV4(&newKey(rng).PublicKey, nil, 0, 0),
		ExternalNode: enode.NewV4(&newKey(rng).PublicKey, nil, 0, 0),
	}
}

func newKey(rng *rand.Rand) *ecdsa.PrivateKey {
	key, err := ecdsa.GenerateKey(crypto.S256(), rng)
	if err != nil {
		panic("couldn't generate key: " + err.Error())
	}
	return key
}

func testValAssignmentsDistribution(t *testing.T, testMsg string, va *valAssignments, expectedValsPerProxy int, toleranceMultiplier float64) {
	// Allows some variance-- setting this relatively high to prevent flaky tests
	tolerance := int(float64(expectedValsPerProxy) * toleranceMultiplier)
	min := expectedValsPerProxy - tolerance
	max := expectedValsPerProxy + tolerance
	valCount := 0
	for _, vals := range va.proxyToVals {
		valCount += len(vals)
		if len(vals) < expectedValsPerProxy-tolerance || len(vals) > expectedValsPerProxy+tolerance {
			t.Errorf("%s: adding multiple proxies & validators resulted in a proxy being assigned %v validators. Expected min: %v max: %v", testMsg, len(vals), min, max)
		}
	}
	// Should be nVals + the two we added earlier
	if len(va.valToProxy) != valCount {
		t.Errorf("%s: adding multiple proxies & validators did not result in the correct amount of validators assigned: %v != %v", testMsg, len(va.valToProxy), valCount)
	}
}

func valAssignmentsIsEmpty(va *valAssignments) bool {
	return len(va.valToProxy) == 0 && len(va.proxyToVals) == 0
}

func eqValidators(a map[common.Address]bool, b map[common.Address]bool) bool {
	if len(a) != len(b) {
		return false
	}

	for k, v := range a {
		if v != b[k] {
			return false
		}
	}

	return true
}
