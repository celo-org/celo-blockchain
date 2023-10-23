package rlp

import (
	"encoding/binary"
	"math"
)

// for payloads longer than 55 bytes for both strings and lists additional prefix signalizing
// the total length of the payload is needed
const payloadLengthThreshold = 0x37 // 55 in dec

// indicates that the payload is a list
const shortListEncodingByte = 0xC0

// listEncodingByte (0xC0) + payloadLengthThreshold (0x37) = 0xF7
const longListEncodingByte = 0xF7

// Combine takes two RLP-encoded values one and two and produces a combined one
// as if it was an RLP encoding of the [one, two] list
// based on https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/
func Combine(one []byte, two []byte) []byte {
	payloadLen := len(one) + len(two)
	result := make([]byte, 0, ListSize(uint64(payloadLen)))

	if payloadLen <= payloadLengthThreshold {
		// If the total payload of a list (i.e. the combined length of all its items being RLP
		// encoded) is 0-55 bytes long, the RLP encoding consists of a single byte with value
		// 0xc0 plus the length of the payload followed by the concatenation of the RLP encodings
		// of the items.
		result = append(result, byte(shortListEncodingByte+payloadLen))
	} else {
		encodedPayloadLength := binaryEncode(uint64(payloadLen))

		// If the total payload of a list is more than 55 bytes long, the RLP encoding consists
		// of a single byte with value 0xf7 plus the length in bytes of the length of the payload
		// in binary form, followed by the length of the payload, followed by the concatenation
		// of the RLP encodings of the items.
		result = append(result, byte(longListEncodingByte+len(encodedPayloadLength)))
		result = append(result, encodedPayloadLength...)
	}

	result = append(result, one...)
	result = append(result, two...)

	return result
}

// binaryEncode returns a binary-encoded number without leading zeroes
func binaryEncode(number uint64) []byte {
	binaryEncoded := make([]byte, 8)

	binary.BigEndian.PutUint64(binaryEncoded, number)

	// we need to +1 as logarithm works for positive numbers
	lengthInBytes := uint(math.Ceil(math.Log2(float64(number+1)) / 8))

	return binaryEncoded[8-lengthInBytes:]
}
