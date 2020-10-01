// Copyright 2017 The celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package proxy_test

import (
	"testing"
)

func TestAddProxy(t *testing.T) {
	// Make sure the added proxy is within the proxy set but not assigned anything
}

func TestRemoveProxy(t *testing.T) {
	// Make sure the removed proxy's is not within the proxy set

	// Make sure that this proxy's enode certificate is removed from the enode certificate maps

	// Make sure that the announce version is incremented
}

func TestConnectedPeer(t *testing.T) {
	// Make sure that the connected peer is within the proxy set

	// Make sure that the enode certificates are correctly created for all proxies

	// Make sure that the announce version is incremented
}

func TestDisconnectedPeer(t *testing.T) {
	// Make sure that the peer's disconnect time is just set

	// Make sure that the peer is not immediatedly removed from the proxy set

	// Sleep for a bit and wait for thread to remove it from proxy set

	// Make sure that peer is removed from proxy set

	// Make sure that the enode certificates map is updated corrected

	// Make sure that the announce version is incremented
}

func TestGetValidatorConnSetDiff(t *testing.T) {
}
