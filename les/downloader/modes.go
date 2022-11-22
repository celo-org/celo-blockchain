// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package downloader

import "fmt"

// SyncMode represents the synchronisation mode of the downloader.
// It is a uint32 as it is used with atomic operations.
type SyncMode uint32

const (
	FullSync     SyncMode = iota // Synchronise the entire blockchain history from full blocks
	FastSync                     // Quickly download the headers, full sync only at the chain head
	SnapSync                     // Download the chain and the state via compact snapshots
	LightSync                    // Download only the headers and terminate afterwards
	LightestSync                 // Synchronise one block per Epoch (Celo-specific mode)
)

func FromString(syncModeStr string) SyncMode {
	switch syncModeStr {
	case "full":
		return FullSync
	case "fast":
		return FastSync
	// case "snap":
	// 	return SnapSync
	case "light":
		return LightSync
	case "lightest":
		return LightestSync
	default:
		panic("Unknown syncModeStr: " + syncModeStr)
	}
}

func (mode SyncMode) IsValid() bool {
	return mode >= FullSync && mode <= LightestSync
}

// String implements the stringer interface.
func (mode SyncMode) String() string {
	switch mode {
	case FullSync:
		return "full"
	case FastSync:
		return "fast"
	// case SnapSync:
	// 	return "snap"
	case LightSync:
		return "light"
	case LightestSync:
		return "lightest"
	default:
		return "unknown"
	}
}

func (mode SyncMode) MarshalText() ([]byte, error) {
	switch mode {
	case FullSync:
		return []byte("full"), nil
	case FastSync:
		return []byte("fast"), nil
	// case SnapSync:
	// 	return []byte("snap"), nil
	// TODO: Implement snap sync
	case LightSync:
		return []byte("light"), nil
	case LightestSync:
		return []byte("lightest"), nil
	default:
		return nil, fmt.Errorf("unknown sync mode %d", mode)
	}
}

func (mode *SyncMode) UnmarshalText(text []byte) error {
	switch string(text) {
	case "full":
		*mode = FullSync
	case "fast":
		*mode = FastSync
	// case "snap":
	// 	*mode = SnapSync
	// TODO: Implement snap sync
	case "light":
		*mode = LightSync
	case "lightest":
		*mode = LightestSync
	default:
		return fmt.Errorf(`unknown sync mode %q, want "full", "fast", "light", or "lightest"`, text)
	}
	return nil
}

// TODO: Enable snap sync mode here. (https://github.com/celo-org/celo-blockchain/issues/1735)

// Returns true if the all headers and not just some a small, discontinuous, set of headers are fetched.
func (mode SyncMode) SyncFullHeaderChain() bool {
	switch mode {
	case FullSync:
		return true
	case FastSync:
		return true
	case LightSync:
		return true
	case LightestSync:
		return false
	default:
		panic(fmt.Errorf("unknown sync mode %d", mode))
	}
}

// Returns true if the full blocks (and not just headers) are fetched.
// If a mode returns true here then it will return true for `SyncFullHeaderChain` as well.
func (mode SyncMode) SyncFullBlockChain() bool {
	switch mode {
	case FullSync:
		return true
	case FastSync:
		return true
	case LightSync:
		return false
	case LightestSync:
		return false
	default:
		panic(fmt.Errorf("unknown sync mode %d", mode))
	}
}
