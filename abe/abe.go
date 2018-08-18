// Copyright 2017 The Celo Authors
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

package abe

import (
	"bytes"
	"encoding/json"
	"encoding/hex"
	"fmt"
	"net/http"
  "regexp"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
)

func SendVerificationTexts(receipts []*types.Receipt, block *types.Block, coinbase common.Address, accountManager *accounts.Manager) {
	numTransactions := 0
	for range block.Transactions() {
		numTransactions += 1
	}
	log.Debug("\n[Celo] New block: "+block.Hash().Hex()+", "+strconv.Itoa(numTransactions), nil, nil)

	wallet, err := accountManager.Find(accounts.Account{Address: coinbase})
	if err != nil {
		log.Error("[Celo] Failed to get account for sms verification", "err", err)
		return
	}

	for _, receipt := range receipts {
		for _, data := range receipt.SmsQueue {
      phoneHash := data[:32]
      log.Debug("Found phone hash")
      log.Debug(hex.EncodeToString(phoneHash))
      encryptedMsgLength := data[63]
      log.Debug("Encrypted message length")
      log.Debug(fmt.Sprintf("%v", encryptedMsgLength))
      // TODO(asa): Perhaps I need to be able to know the length of the message?
      encryptedPhone := data[64:64 + encryptedMsgLength]
      log.Debug("Found encrypted phone number")
      log.Debug(hex.EncodeToString(encryptedPhone))
      //c, err := hex.DecodeString("04c48aefc487295f2bc8e3dcd7a5b60246b1daeab26aac1af86d341068f6b4134cd934f97b9ac28312d3e130154f5177609db1e5d5a1b8d96550390f1dd58fad8c189d145da5c5c0b2154a2040c3b72acd54c63fd89e0ed2c7fefbd71db8203ed6de50a3203e2d4d8a3a7a0392f99af5647f715785cf1cf7ec2bfb0b9e")
      phone, err := wallet.Decrypt(accounts.Account{Address: coinbase}, encryptedPhone, nil, nil)
      //phone, err := wallet.Decrypt(accounts.Account{Address: coinbase}, c, nil, nil)
      if err != nil {
        log.Error("[Celo] Failed to decrypt phone number", "err", err)
        continue
      }
			log.Debug("[Celo] Decrypted phone: "+string(phone), nil, nil)

      r, _ := regexp.Compile("\\+1[0-9]{10}")
      if bytes.Equal(crypto.Keccak256(phone), phoneHash) {
				log.Error("[Celo] Phone hash doesn't match decrypted phone number", nil, nil)
				continue
			} else if !r.MatchString(string(phone)) {
        log.Error("[Celo] Decrypted phone number invalid: " + string(phone), nil, nil)
				continue
      }

			// Construct the secret code to be sent via SMS.
			unsignedCode := common.BytesToHash([]byte(string(phone) + block.Number().String()))
			code, err := wallet.SignHash(accounts.Account{Address: coinbase}, unsignedCode.Bytes())
			if err != nil {
				log.Error("[Celo] Failed to sign message for sending over SMS", "err", err)
				continue
			}
			hexCode := hexutil.Encode(code[:])
			log.Debug("[Celo] Secret code: "+hexCode+" "+string(len(code)), nil, nil)
			secret := fmt.Sprintf("Gem verification code: %s", hexCode)
			log.Debug("[Celo] New verification request: "+receipt.TxHash.Hex()+" "+string(phone), nil, nil)

			// Send the actual text message using our mining pool.
			// TODO: Make mining pool be configurable via command line arguments.
			url := "https://mining-pool.celo.org/v0.1/sms"
			values := map[string]string{"phoneNumber": string(phone), "message": secret}
			jsonValue, _ := json.Marshal(values)
			_, err = http.Post(url, "application/json", bytes.NewBuffer(jsonValue))
			log.Debug("[Celo] SMS send Url: "+url, nil, nil)

			// Retry 5 times if we fail.
			for i := 0; i < 5; i++ {
				if err == nil {
					break
				}
				log.Debug("[Celo] Got an error when trying to send SMS to: "+url, nil, nil)
				time.Sleep(100 * time.Millisecond)
				_, err = http.Post(url, "application/json", bytes.NewBuffer(jsonValue))
			}

			log.Debug("[Celo] Sent SMS", nil, nil)
		}
	}
}
