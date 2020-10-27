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
	"github.com/ethereum/go-ethereum/p2p/enode"
)

// SendDelegateSignMsgToProxy sends an istanbulDelegateSign message to a proxy
// if one exists
func (pv *proxiedValidatorEngine) SendDelegateSignMsgToProxy(msg []byte, peerID enode.ID) error {
	proxy, err := pv.getProxy(peerID)
	if err != nil {
		return err
	}

	if proxy == nil {
		// If we got here, then the proxy that sent the message to be signed is not up anymore
		return ErrNoCelostatsProxy
	}

	pv.backend.Unicast(proxy.peer, msg, istanbul.DelegateSignMsg)
	return nil
}

// SendDelegateSignMsgToProxiedValidator sends an istanbulDelegateSign message to a
// proxied validator if one exists
func (p *proxyEngine) SendDelegateSignMsgToProxiedValidator(msg []byte) error {
	p.proxiedValidatorsMu.RLock()
	defer p.proxiedValidatorsMu.RUnlock()

	if len(p.proxiedValidators) != 0 {
		for proxiedValidator := range p.proxiedValidators {
			p.backend.Unicast(proxiedValidator, msg, istanbul.DelegateSignMsg)
		}
		return nil
	}

	return ErrNoProxiedValidator
}
