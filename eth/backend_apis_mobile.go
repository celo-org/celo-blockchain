// +build mobile

package eth

import "github.com/ethereum/go-ethereum/rpc"

func (s *Ethereum) APIs() []rpc.API {
	return []rpc.API{}
}
