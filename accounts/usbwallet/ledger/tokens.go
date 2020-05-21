package ledger

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

var ErrCouldNotFindToken = errors.New("could not find token")
var ErrNotAnERC20Transfer = errors.New("not an ERC20 transfer")

type Token struct {
	Ticker    string
	Address   common.Address
	Decimals  uint32
	ChainID   uint32
	Signature []byte
	Data      []byte
}

type Tokens struct {
	tokens []Token
}

func LoadTokens(blob string) (*Tokens, error) {
	buf, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return nil, err
	}

	tokens := make([]Token, 0)

	i := 0
	for i < len(buf) {
		length := int(binary.BigEndian.Uint32(buf[i : i+4]))
		i += 4

		item := buf[i : i+length]
		offset := 0

		tickerLength := int(item[offset])
		offset += 1

		ticker := string(item[offset : offset+tickerLength])
		offset += tickerLength

		var address common.Address
		copy(address[:], item[offset:offset+20])
		offset += 20

		decimals := binary.BigEndian.Uint32(item[offset : offset+4])
		offset += 4

		chainID := binary.BigEndian.Uint32(item[offset : offset+4])
		offset += 4

		signature := item[offset:length]

		tokens = append(tokens, Token{
			Ticker:    ticker,
			Address:   address,
			Decimals:  decimals,
			ChainID:   chainID,
			Signature: signature,
			Data:      item,
		})

		i += length
	}

	return &Tokens{
		tokens,
	}, nil
}

func (t *Tokens) ByContractAddressAndChainID(address common.Address, chainID *big.Int) (*Token, error) {
	for _, token := range t.tokens {
		if token.Address == address && uint64(token.ChainID) == chainID.Uint64() {
			return &token, nil
		}
	}

	return nil, ErrCouldNotFindToken
}

func IsERC20Transfer(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// 0xa9059cbb is the ERC20 transfer function signature
	if !bytes.Equal(data[:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) {
		return false
	}

	return true
}
