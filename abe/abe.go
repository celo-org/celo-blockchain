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
	"github.com/ethereum/go-ethereum/common/math"
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

      unsignedMessage := data[32:64]
      log.Debug("Found unsignedMessage")
      log.Debug(hex.EncodeToString(unsignedMessage))

      // casat with (*big.Int)
      index, parsed := math.ParseBig256(hexutil.Encode(data[64:96]))
      if !parsed {
        log.Error("[Celo] Unable to decode verification index: "+ hexutil.Encode(data[64:96]), "err", err)
      }
      log.Debug("Found index")
      log.Debug(index.String())

      encryptedMsgLength := data[127]
      log.Debug("Encrypted message length")
      log.Debug(fmt.Sprintf("%v", encryptedMsgLength))

      encryptedPhone := data[128:128 + encryptedMsgLength]
      log.Debug("Found encrypted phone number")
      log.Debug(hex.EncodeToString(encryptedPhone))

      phone, err := wallet.Decrypt(accounts.Account{Address: coinbase}, encryptedPhone, nil, nil)
      if err != nil {
        log.Error("[Celo] Failed to decrypt phone number", "err", err)
        continue
      }
			log.Debug("[Celo] Decrypted phone: "+string(phone), nil, nil)

      r, _ := regexp.Compile("\\+1[0-9]{10}")
      log.Debug("Checking for phone hash match")
      log.Debug(hex.EncodeToString(crypto.Keccak256(phone)))
      if !bytes.Equal(crypto.Keccak256(phone), phoneHash) {
				log.Error("[Celo] Phone hash doesn't match decrypted phone number", nil, nil)
				continue
			} else if !r.MatchString(string(phone)) {
        log.Error("[Celo] Decrypted phone number invalid: " + string(phone), nil, nil)
				continue
      }

			// Construct the secret code to be sent via SMS.
			code, err := wallet.SignHash(accounts.Account{Address: coinbase}, unsignedMessage)
			if err != nil {
				log.Error("[Celo] Failed to sign message for sending over SMS", "err", err)
				continue
			}
      secret := fmt.Sprintf("%s:%s", hexutil.Encode(code[:]), index.String())
			log.Debug("[Celo] Secret: " + secret, nil, nil)
      message := fmt.Sprintf("Gem verification code: %s", secret)
			log.Debug("[Celo] New verification request: "+receipt.TxHash.Hex()+" "+string(phone), nil, nil)

			// Send the actual text message using our mining pool.
			// TODO: Make mining pool be configurable via command line arguments.
			url := "https://mining-pool.celo.org/v0.1/sms"
			values := map[string]string{"phoneNumber": string(phone), "message": message}
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
