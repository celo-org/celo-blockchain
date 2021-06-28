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
	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/log"
)

// SetVerbosity sets the global verbosity level (between 0 and 6 - see logger/verbosity.go).
func SetVerbosity(level int) {
	handler := debug.CreateStreamHandler("term", "split")
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(level), handler))
}

// Note: A call to SetVerbosity after a call to SendLogsToFile will disable file logging.
// That's just how currently the code is structured. It is not an issue since we are going
// to make the calls sequentially at the time of initialization and can choose a particular
// order.
// Logs will be sent to this file along with the logcat.
// format has to be term or json. Anything else will cause the app to panic.
func SendLogsToFile(filename string, level int, format string) bool {
	var consoleFormat log.Format
	if format == "term" {
		consoleFormat = log.TerminalFormat(false)
	} else if format == "json" {
		consoleFormat = log.JSONFormat()
	} else {
		panic("Unexpected format: " + format)
	}
	currentHandler := log.Root().GetHandler()
	fileHandler, err := log.FileHandler(filename, consoleFormat)
	levelFilterFileHandler := log.LvlFilterHandler(log.Lvl(level), fileHandler)
	if err != nil {
		log.Error("SendLogsToFile/Failed to open file " + filename + " for logging")
		return false
	} else {
		if currentHandler == nil {
			log.Root().SetHandler(levelFilterFileHandler)
		} else {
			multiHandler := log.MultiHandler(currentHandler, levelFilterFileHandler)
			log.Root().SetHandler(multiHandler)
		}
		return true
	}
}
