// Copyright 2016 The go-ethereum Authors
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

package geth

import (
	"sync"
)

type Stats struct{
	sync.RWMutex
	m map[string]string
}

func (s *Stats) GetStatsKeys() []string {
	s.RLock()
	keys := make([]string, len(s.m))

	i := 0
	for k := range s.m {
			keys[i] = k
			i++
	}
	s.RUnlock()
	return keys
}

func (s *Stats) GetValue(key string) string {
	s.RLock()
	val := s.m[key]
	s.RUnlock()

	return val
}