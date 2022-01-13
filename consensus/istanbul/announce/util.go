package announce

import (
	"fmt"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

// DecryptEnodeURL decrypts an encrypted enodeURL with the given ecdsa key pair.
func DecryptEnodeURL(ecdsa *istanbul.EcdsaInfo, encEnodeURL []byte) (*enode.Node, error) {
	enodeBytes, err := ecdsa.Decrypt(encEnodeURL)
	if err != nil {
		return nil, fmt.Errorf("Error decrypting endpoint. err=%v. encEnodeURL: '%v'", err, encEnodeURL)
	}
	enodeURL := string(enodeBytes)
	node, err := enode.ParseV4(enodeURL)
	if err != nil {
		return nil, fmt.Errorf("Error parsing enodeURL. err=%v. enodeURL: '%v'", err, enodeURL)
	}
	return node, err
}
