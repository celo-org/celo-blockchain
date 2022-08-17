package core

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

func newRoundChangeSetV2(valSet istanbul.ValidatorSet) *roundChangeSetV2 {
	return &roundChangeSetV2{
		validatorSet:      valSet,
		msgsForRound:      make(map[uint64]MessageSet),
		latestRoundForVal: make(map[common.Address]uint64),
		mu:                new(sync.Mutex),
	}
}

type roundChangeSetV2 struct {
	validatorSet      istanbul.ValidatorSet
	msgsForRound      map[uint64]MessageSet
	latestRoundForVal map[common.Address]uint64
	mu                *sync.Mutex
}

// RoundChangeSetSummary holds a print friendly view of a RoundChangeSet.
type RoundChangeSetSummary struct {
	RoundForVal map[common.Address]uint64   `json:"roundForVal"`
	ValsInRound map[uint64][]common.Address `json:"valsInRound"`
}

// Summary returns a print friendly summary of the messages in the set.
func (rcs *roundChangeSetV2) Summary() *RoundChangeSetSummary {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()
	rounds := make(map[common.Address]uint64)
	for v, r := range rcs.latestRoundForVal {
		rounds[v] = r
	}
	vals := make(map[uint64][]common.Address)
	for r, vs := range rcs.msgsForRound {
		vs2 := make([]common.Address, 0, vs.Size())
		for _, v := range vs.Values() {
			vs2 = append(vs2, v.Address)
		}
		vals[r] = vs2
	}
	return &RoundChangeSetSummary{
		RoundForVal: rounds,
		ValsInRound: vals,
	}
}

// Add adds the round and message into round change set
func (rcs *roundChangeSetV2) Add(r *big.Int, msg *istanbul.Message) error {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	src := msg.Address
	round := r.Uint64()

	if prevLatestRound, ok := rcs.latestRoundForVal[src]; ok {
		if prevLatestRound > round {
			// Reject as we have an RC for a later round from this validator.
			return errOldMessage
		} else if prevLatestRound < round {
			// Already got an RC for an earlier round from this validator.
			// Forget that and remember this.
			if rcs.msgsForRound[prevLatestRound] != nil {
				rcs.msgsForRound[prevLatestRound].Remove(src)
				if rcs.msgsForRound[prevLatestRound].Size() == 0 {
					delete(rcs.msgsForRound, prevLatestRound)
				}
			}
		}
	}

	rcs.latestRoundForVal[src] = round

	if rcs.msgsForRound[round] == nil {
		rcs.msgsForRound[round] = newMessageSet(rcs.validatorSet)
	}
	return rcs.msgsForRound[round].Add(msg)
}

// Clear deletes the messages with smaller round
func (rcs *roundChangeSetV2) Clear(round *big.Int) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	for k, rms := range rcs.msgsForRound {
		if rms.Size() == 0 || k < round.Uint64() {
			for _, msg := range rms.Values() {
				delete(rcs.latestRoundForVal, msg.Address) // no need to check if msg.Address is present
			}
			delete(rcs.msgsForRound, k)
		}
	}
}

// MaxRound returns the max round which the number of messages is equal or larger than num
func (rcs *roundChangeSetV2) MaxRound(num int) *big.Int {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	// Sort rounds descending
	var sortedRounds []uint64
	for r := range rcs.msgsForRound {
		sortedRounds = append(sortedRounds, r)
	}
	sort.Slice(sortedRounds, func(i, j int) bool { return sortedRounds[i] > sortedRounds[j] })

	acc := 0
	for _, r := range sortedRounds {
		rms := rcs.msgsForRound[r]
		acc += rms.Size()
		if acc >= num {
			return new(big.Int).SetUint64(r)
		}
	}

	return nil
}

// MaxOnOneRound returns the max round which the number of messages is >= num
func (rcs *roundChangeSetV2) MaxOnOneRound(num int) *big.Int {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	// Sort rounds descending
	var sortedRounds []uint64
	for r := range rcs.msgsForRound {
		sortedRounds = append(sortedRounds, r)
	}
	sort.Slice(sortedRounds, func(i, j int) bool { return sortedRounds[i] > sortedRounds[j] })

	for _, r := range sortedRounds {
		rms := rcs.msgsForRound[r]
		if rms.Size() >= num {
			return new(big.Int).SetUint64(r)
		}
	}
	return nil
}

func (rcs *roundChangeSetV2) String() string {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	// Sort rounds descending
	var sortedRounds []uint64
	for r := range rcs.msgsForRound {
		sortedRounds = append(sortedRounds, r)
	}
	sort.Slice(sortedRounds, func(i, j int) bool { return sortedRounds[i] > sortedRounds[j] })

	modeRound := uint64(0)
	modeRoundSize := 0
	msgsForRoundStr := make([]string, 0, len(sortedRounds))
	for _, r := range sortedRounds {
		rms := rcs.msgsForRound[r]
		if rms.Size() > modeRoundSize {
			modeRound = r
			modeRoundSize = rms.Size()
		}
		msgsForRoundStr = append(msgsForRoundStr, fmt.Sprintf("%v: %v", r, rms.String()))
	}

	return fmt.Sprintf("RCS len=%v mode_round=%v mode_round_len=%v unique_rounds=%v %v",
		len(rcs.latestRoundForVal),
		modeRound,
		modeRoundSize,
		len(rcs.msgsForRound),
		strings.Join(msgsForRoundStr, ", "))
}

// Gets a round change certificate for a specific round. Includes quorumSize messages of that round or later.
// If the total is less than quorumSize, returns an empty cert and errFailedCreateRoundChangeCertificate.
func (rcs *roundChangeSetV2) getCertificate(minRound *big.Int, quorumSize int) (istanbul.RoundChangeCertificateV2, error) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	// Sort rounds descending
	var sortedRounds []uint64
	for r := range rcs.msgsForRound {
		sortedRounds = append(sortedRounds, r)
	}
	sort.Slice(sortedRounds, func(i, j int) bool { return sortedRounds[i] > sortedRounds[j] })

	var requests []istanbul.RoundChangeRequest
	for _, r := range sortedRounds {
		if r < minRound.Uint64() {
			break
		}
		for _, message := range rcs.msgsForRound[r].Values() {
			requests = append(requests, message.RoundChangeV2().Request)

			// Stop when we've added a quorum of the highest-round messages.
			if len(requests) >= quorumSize {
				return istanbul.RoundChangeCertificateV2{
					Requests: requests,
				}, nil
			}
		}
	}

	// Didn't find a quorum of messages. Return an empty certificate with error.
	return istanbul.RoundChangeCertificateV2{}, errFailedCreateRoundChangeCertificate
}
