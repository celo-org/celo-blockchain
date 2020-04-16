package ledger

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

const TokensBlob = "AAAAaARjVVNES4TC75SidNv4Pi8ewWCEVsm2LZYAAAASAACcuDBFAiEAqkKjjSN4G+snqF6HY3oxwh5vmslOh/gZVG3MGg5vgfgCIGuGPU8jhvxdXF4bMSQc4vLs2iXgWzzj/1fOLX2kmDoTAAAAaARjR0xERPQ06Doxefzt4olBs6gZU/tXUhcAAAASAACcuDBFAiEAlnEhhgsbClErGxR6SmBTuGEwIE3I+gofDEufqkB6+ewCIFbdxoPuDvl6mb5xzYMM0cCeJwRvaOT7/jTj0RWHIWOZAAAAaARjVVNEpWETGhyKwlkl+4SLykWnSvYeWjgAAAASAACu8jBFAiEAxVAZSsitVIBd7Ov1wGYgZGVI68gA1tlFkPDaTzAppEACIAlPr01KDlmG1pUo7hd4nD4rN2qLdeJaT8sDk23cLuS5AAAAZwRjR0xE8ZSv31CwPmm9fQV8GqnhDJlU5MkAAAASAACu8jBEAiBkJSVHwDoQclm3VNsr1LB0JrXIUy8hQJEBDZJeKS3gsQIgNfXfisKOh/yjw5HM0kv6zYyv3Ea+XI9wVPDQ4OtaPig=" // #nosec

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

	if !bytes.Equal(data[:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) {
		return false
	}

	return true
}
