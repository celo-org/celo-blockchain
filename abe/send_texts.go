// Copyright 2017 The Celo Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package abe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

func SendTexts(block *types.Block, coinbase common.Address, accountManager *accounts.Manager) {
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

	for _, tx := range block.Transactions() {
		data := string(tx.Data())

		log.Debug("[Celo] PM - TX: "+tx.Hash().Hex()+" "+data, nil, nil)

		if len(data) > 0 && (strings.HasPrefix(data, "reqVerify") || strings.HasPrefix(data, "reqAndVerify")) {
			dataArray := strings.Split(data, "-")
			phone := dataArray[len(dataArray)-1]
			log.Debug("[Celo] PM - phone: "+phone, nil, nil)

			nonce := tx.Nonce()
			unsignedCode := common.BytesToHash([]byte(string(phone) + string(nonce)))
			code, err := wallet.SignHash(accounts.Account{Address: coinbase}, unsignedCode.Bytes())
			if err != nil {
				log.Error("[Celo] Failed to sign phone number for sending over SMS", "err", err)
				continue
			}

			hexCode := hexutil.Encode(code[:])
			log.Debug("[Celo] PM - New code: "+hexCode+" "+string(len(code)), nil, nil)

			secret := fmt.Sprintf("Gem verification code: %s", hexCode)
			log.Debug("[Celo] PM - New Verification request: "+tx.Hash().Hex()+" "+phone, nil, nil)

			if len(phone) > 0 {
				url := "https://mining-pool.celo.org/send-text"
				values := map[string]string{"phoneNumber": phone, "message": secret}
				jsonValue, _ := json.Marshal(values)
				_, err = http.Post(url, "application/json", bytes.NewBuffer(jsonValue))
				log.Debug("[Celo] PM - SMS Url: "+url, nil, nil)

				for i := 0; i < 5; i++ {
					if err == nil {
						break
					}
					log.Debug("[Celo] PM - Got an error when trying to send SMS to: "+url, nil, nil)
					time.Sleep(100 * time.Millisecond)
					_, err = http.Post(url, "application/json", bytes.NewBuffer(jsonValue))
				}

				log.Debug("[Celo] PM - Sent SMS", nil, nil)
			}
		}
	}
}
