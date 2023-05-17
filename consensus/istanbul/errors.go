// Copyright 2017 The go-ethereum Authors
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

package istanbul

import "errors"

var (
	// ErrUnauthorizedAddress is returned when given address cannot be found in
	// current validator set.
	ErrUnauthorizedAddress = errors.New("not an elected validator")
	// ErrInvalidSigner is returned if a message's signature does not correspond to the address in msg.Address
	ErrInvalidSigner = errors.New("signed by incorrect validator")
	// ErrStoppedEngine is returned if the engine is stopped
	ErrStoppedEngine = errors.New("stopped engine")
	// ErrStartedEngine is returned if the engine is already started
	ErrStartedEngine = errors.New("started engine")
	// ErrStoppedAnnounce is returned if announce is stopped
	ErrStoppedAnnounce = errors.New("stopped announce")
	// ErrStartedAnnounce is returned if announce is already started
	ErrStartedAnnounce = errors.New("started announce")
	// ErrStoppedVPHThread is returned if validator peer handler thread is stopped
	ErrStoppedVPHThread = errors.New("stopped validator peer handler thread")
	// ErrStartedVPHThread is returned if validator peer handler thread is already started
	ErrStartedVPHThread = errors.New("started validator peer handler thread")
	// ErrInvalidEnodeCertMsgMapOldVersion is returned if a validator sends old enode certificate message
	ErrInvalidEnodeCertMsgMapOldVersion = errors.New("invalid enode certificate message map because of old version")
)
