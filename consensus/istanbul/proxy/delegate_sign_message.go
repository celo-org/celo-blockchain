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

package proxy

import (
	"errors"

	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
)

// SendDelegateSignMsgToProxy sends an istanbulDelegateSign message to a proxy
// if one exists
func (p *proxyEngine) SendDelegateSignMsgToProxy(msg []byte) error {
	i := 0
	for _, proxy := range p.ph.ps.proxiesByID {
		if proxy.peer != nil {
			err := proxy.peer.Send(istanbul.DelegateSignMsg, msg)
			if err != nil {
				log.Warn("Error sending to proxy", err)
			} else {
				i = i+1
			}
		}
	}
	if i == 0 {
		return errors.New("Not connected to proxy")
	} else {
		return nil
	}
}

// SendDelegateSignMsgToProxiedValidator sends an istanbulDelegateSign message to a
// proxied validator if one exists
func (p *proxyEngine) SendDelegateSignMsgToProxiedValidator(msg []byte) error {
	if p.proxiedValidator != nil {
		return p.proxiedValidator.Send(istanbul.DelegateSignMsg, msg)
	} else {
		return errors.New("Not connected to proxied validator")
	}
}
