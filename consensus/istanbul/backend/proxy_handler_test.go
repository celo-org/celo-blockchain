package backend

import (
	"crypto/ecdsa"
	"math/rand"
	"log"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestConsistentHashingPolicy(t *testing.T) {
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
	fooProxy := newProxy()
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
	for i := 0; i < nProxies; i++ {
		ch.addProxy(newProxy())
	}
	nVals := 100
	newValidators = make(map[common.Address]bool)
	for i := 0; i < nVals; i++ {
		addrBytes := make([]byte, 32)
		rand.Read(addrBytes)
		addr := common.BytesToAddress(addrBytes)
		newValidators[addr] = true
	}
	ch.addValidators(newValidators)

	expectedValsPerProxy := nVals / nProxies
	tolerance := int(float64(expectedValsPerProxy) * 0.7)
	min := expectedValsPerProxy - tolerance
	max := expectedValsPerProxy + tolerance
	va = ch.getValAssignments()
	sum := 0
	for proxy, vals := range va.proxyToVals {
		sum += len(vals)
		log.Printf("proxy: %v\n vals: %v\n len: %v\n", proxy, vals, len(vals))
		if len(vals) < expectedValsPerProxy - tolerance || len(vals) > expectedValsPerProxy + tolerance {
			t.Errorf("Adding multiple proxies & validators resulted in a proxy being assigned %v validators. Expected min: %v max: %v", len(vals), min, max)
		}
	}

	log.Printf("sum %v", sum)

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
	if val, ok := va.valToProxy[fooVal]; val != nil || ok == false || len(va.proxyToVals[fooProxy]) != 0 {
		t.Errorf("va.unassignValidator() did not correctly unassign a validator from its proxy")
	}
}

func newProxy() *proxy {
	return &proxy{
		internalNode: enode.NewV4(&newKey().PublicKey, nil, 0, 0),
		externalNode: enode.NewV4(&newKey().PublicKey, nil, 0, 0),
	}
}

func newKey() *ecdsa.PrivateKey {
	key, err := crypto.GenerateKey()
	if err != nil {
		panic("couldn't generate key: " + err.Error())
	}
	return key
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
