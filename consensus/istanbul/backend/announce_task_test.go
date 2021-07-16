package backend

import (
	"testing"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/stretchr/testify/assert"
)

func TestShouldStartStopAnnouncing(t *testing.T) {
	st := &announceTaskState{}
	assert.False(t, st.ShouldStartAnnouncing())
	st.shouldAnnounce = true
	assert.True(t, st.ShouldStartAnnouncing())
	st.OnStartAnnouncing()
	assert.False(t, st.ShouldStartAnnouncing())
	st.OnStopAnnouncing()
	assert.True(t, st.ShouldStartAnnouncing())
	st.shouldAnnounce = false
	assert.False(t, st.ShouldStartAnnouncing())
}

func TestShouldStartStopQuerying(t *testing.T) {
	st := &announceTaskState{
		config: &istanbul.Config{
			AggressiveQueryEnodeGossipOnEnablement: true,
		},
	}
	assert.False(t, st.ShouldStartQuerying())
	st.shouldQuery = true
	assert.True(t, st.ShouldStartQuerying())
	st.OnStartQuerying()
	assert.False(t, st.ShouldStartQuerying())
	st.OnStopQuerying()
	assert.True(t, st.ShouldStartQuerying())
	st.shouldQuery = false
	assert.False(t, st.ShouldStartQuerying())
}
