// Copyright 2021 The Celo Authors
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

package metrics

import (
	"encoding/csv"
	"fmt"
	"io"
	"sync"
)

type WriterCloser interface {
	io.Writer
	io.Closer
}

// A CSVRecorder enables easy writing of CSV data a specified writer.
// The header is written on creation. Writing is thread safe.
type CSVRecorder struct {
	writer    csv.Writer
	backingWC WriterCloser
	writeMu   sync.Mutex
}

// NewCSVRecorder creates a CSV recorder that writes to the supplied writer.
// The writer is retained and can be closed by calling CSVRecorder.Close()
// The header is immediately written upon construction.
func NewCSVRecorder(wc WriterCloser, fields ...string) *CSVRecorder {
	c := &CSVRecorder{
		writer:    *csv.NewWriter(wc),
		backingWC: wc,
	}
	c.writer.Write(fields)
	return c
}

// WriteRow writes out as csv row. Will convert the values to a string using "%v".
func (c *CSVRecorder) Write(values ...interface{}) {
	if c == nil {
		return
	}
	strs := make([]string, 0, len(values))
	for _, v := range values {
		strs = append(strs, fmt.Sprintf("%v", v))
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.writer.Write(strs)
}

// Close closes the writer. This is a no-op for a nil receiver.
func (c *CSVRecorder) Close() error {
	if c == nil {
		return nil
	}
	c.writer.Flush()
	return c.backingWC.Close()
}
