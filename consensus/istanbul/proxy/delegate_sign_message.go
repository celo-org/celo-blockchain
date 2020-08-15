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
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

// SendDelegateSignMsgToProxy sends an istanbulDelegateSign message to a proxy
// if one exists
func (pv *proxiedValidatorEngine) SendDelegateSignMsgToProxy(msg []byte) error {
	proxies, _, err := pv.GetProxiesAndValAssignments()
	if err != nil {
		return err
	}

	for _, proxy := range proxies {
		if proxy.peer != nil {
			isStatsProxy, err := pv.IsStatsProxy(proxy.peer.Node().ID())
			if err != nil {
				return err
			}

			if isStatsProxy {
				pv.backend.Unicast(proxy.peer, msg, istanbul.DelegateSignMsg)
			}
			return nil
		}
	}

	// If we got here, then there is no designated proxy for the ethstats messages
	return ErrNoEthstatsProxy
}

// SendDelegateSignMsgToProxiedValidator sends an istanbulDelegateSign message to a
// proxied validator if one exists
func (p *proxyEngine) SendDelegateSignMsgToProxiedValidator(msg []byte) error {
	p.proxiedValidatorMu.RLock()
	defer p.proxiedValidatorMu.RUnlock()

	if p.proxiedValidator != nil {
		p.backend.Unicast(p.proxiedValidator, msg, istanbul.DelegateSignMsg)
		return nil
	} else {
		return ErrNoProxiedValidator
	}
}
