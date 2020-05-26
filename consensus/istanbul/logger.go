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

package istanbul

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

type istLogger struct {
	logger log.Logger
	round  func() *big.Int
}

// NewIstLogger creates an Istanbul Logger with custom logic for exposing logs
func NewIstLogger(fn func() *big.Int, ctx ...interface{}) log.Logger {
	return &istLogger{logger: log.New(ctx...), round: fn}
}

func (l *istLogger) New(ctx ...interface{}) log.Logger {
	childLogger := l.logger.New(ctx...)
	return &istLogger{logger: childLogger, round: l.round}
}

func (l *istLogger) Trace(msg string, ctx ...interface{}) {
	// If the current round > 1, then upgrade this message to Info
	if l.round != nil && l.round() != nil && l.round().Cmp(common.Big1) > 0 {
		l.Info(msg, ctx...)
	} else {
		l.logger.Trace(msg, ctx...)
	}
}

func (l *istLogger) Debug(msg string, ctx ...interface{}) {
	// If the current round > 1, then upgrade this message to Info
	if l.round != nil && l.round() != nil && l.round().Cmp(common.Big1) > 0 {
		l.Info(msg, ctx...)
	} else {
		l.logger.Debug(msg, ctx...)
	}
}

func (l *istLogger) Info(msg string, ctx ...interface{}) {
	l.logger.Info(msg, ctx...)
}

func (l *istLogger) Warn(msg string, ctx ...interface{}) {
	l.logger.Warn(msg, ctx...)
}

func (l *istLogger) Error(msg string, ctx ...interface{}) {
	l.logger.Error(msg, ctx...)
}

func (l *istLogger) Crit(msg string, ctx ...interface{}) {
	l.logger.Crit(msg, ctx...)
}

func (l *istLogger) GetHandler() log.Handler {
	return l.logger.GetHandler()
}

func (l *istLogger) SetHandler(h log.Handler) {
	l.logger.SetHandler(h)
}
