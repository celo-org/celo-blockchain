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
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// A CSVRecorder enables easy writing of CSV data a specified writer.
// The header is written on creation. Writing is thread safe.
type CSVRecorder struct {
	header  []string
	writer  io.Writer
	format  string
	writeMu sync.Mutex
}

// NewStdoutCSVRecorder creates a CSV recorder to standard out
// The header is immediately written upon construction.
func NewStdoutCSVRecorder(fields ...string) *CSVRecorder {
	return NewCSVRecorder(os.Stdout, fields...)
}

// NewStdoutCSVRecorder creates a CSV recorder that writes to the supplied writer.
// The header is immediately written upon construction.
func NewCSVRecorder(w io.Writer, fields ...string) *CSVRecorder {
	fmt.Fprintln(w, strings.Join(fields, ","))
	return &CSVRecorder{
		header: fields,
		writer: w,
		format: rowFormatString(len(fields))}
}

// rowFormatString creates the string "%v,%v[,%v]...\n" where "%v" is repeated fieldCount times.
func rowFormatString(fieldCount int) string {
	var b strings.Builder
	b.WriteString("%v")
	for i := 1; i < fieldCount; i++ {
		b.WriteString(",%v")
	}
	b.WriteString("\n")
	return b.String()
}

// WriteRow writes out as csv row. Each value is printed with "%v"
// The length of the values must match the length of the header.
func (c *CSVRecorder) WriteRow(values ...interface{}) {
	if len(values) != len(c.header) {
		panic("Dev error: Value length does not match the header length")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	fmt.Fprintf(c.writer, c.format, values...)
}
