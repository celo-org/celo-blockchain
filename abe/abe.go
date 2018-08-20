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
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
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
		log.Error("[Celo] Failed to get account", "err", err)
		return
	}

	for _, receipt := range receipts {
		for _, phone := range receipt.SmsQueue {
			log.Debug("[Celo] Sending text to phone: "+phone, nil, nil)

			if len(phone) <= 0 {
				log.Error("[Celo] Invalid phone number: "+phone, nil, nil)
				continue
			}

			// Construct the secret code to be sent via SMS.
			unsignedCode := common.BytesToHash([]byte(phone + block.Number().String()))
			code, err := wallet.SignHash(accounts.Account{Address: coinbase}, unsignedCode.Bytes())
			if err != nil {
				log.Error("[Celo] Failed to sign message for sending over SMS", "err", err)
				continue
			}
			hexCode := hexutil.Encode(code[:])
			log.Debug("[Celo] Secret code: "+hexCode+" "+string(len(code)), nil, nil)
			secret := fmt.Sprintf("Gem verification code: %s", hexCode)
			log.Debug("[Celo] New verification request: "+receipt.TxHash.Hex()+" "+phone, nil, nil)

			// Send the actual text message using our mining pool.
			// TODO: Make mining pool be configurable via command line arguments.
			url := "https://mining-pool.celo.org/v0.1/sms"
			values := map[string]string{"phoneNumber": phone, "message": secret}
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
