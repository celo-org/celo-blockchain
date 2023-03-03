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

package core

import (
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/syndtr/goleveldb/leveldb"
)

const ()

type roundStateDBImpl2 struct {
	rs     RoundState
	logger log.Logger
}

// newRoundStateDB2 will only retain the latest roundstate and nothing else
func newRoundStateDB2(path string, opts *RoundStateDBOptions) (RoundStateDB, error) {
	logger := log.New("func", "newRoundStateDB2", "type", "roundStateDB", "rsdb_path", path)

	logger.Info("Open roundstate db 2")

	rsdb := &roundStateDBImpl2{
		logger: logger,
		rs:     nil,
	}

	return rsdb, nil
}

func (rsdb *roundStateDBImpl2) UpdateLastRoundState(rs RoundState) error {
	rsdb.rs = rs
	return nil
}

func (rsdb *roundStateDBImpl2) GetLastView() (*istanbul.View, error) {
	if rsdb.rs == nil {
		return nil, leveldb.ErrNotFound
	}
	return rsdb.rs.View(), nil
}

func (rsdb *roundStateDBImpl2) GetOldestValidView() (*istanbul.View, error) {
	// only used for self GC implemented in RoundStateDb 1
	return nil, nil
}

func (rsdb *roundStateDBImpl2) GetRoundStateFor(view *istanbul.View) (RoundState, error) {
	if rsdb.rs == nil {
		return nil, leveldb.ErrNotFound
	}
	if view.Sequence == rsdb.rs.Sequence() && view.Round == rsdb.rs.Round() {
		return rsdb.rs, nil
	}
	return nil, leveldb.ErrNotFound
}

func (rsdb *roundStateDBImpl2) Close() error {
	rsdb.rs = nil
	return nil
}
