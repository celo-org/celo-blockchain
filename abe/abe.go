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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
)

func decryptPhoneNumber(request types.VerificationRequest, account accounts.Account, wallet accounts.Wallet) (string, error) {
	phoneNumber, err := wallet.Decrypt(account, request.EncryptedPhone, nil, nil)
	if err != nil {
		return "", err
	}
	// TODO(asa): Better validation of phone numbers
	r, _ := regexp.Compile(`^\+[0-9]{8,15}$`)
	if !bytes.Equal(crypto.Keccak256(phoneNumber), request.PhoneHash.Bytes()) {
		return string(phoneNumber), errors.New("Phone hash doesn't match decrypted phone number")
	} else if !r.MatchString(string(phoneNumber)) {
		return string(phoneNumber), fmt.Errorf("Decrypted phone number invalid: %s", string(phoneNumber))
	}
	return string(phoneNumber), nil
}

func createVerificationMessage(request types.VerificationRequest, verificationRewardsAddress common.Address, account accounts.Account, wallet accounts.Wallet) (string, error) {
	unsignedMessage := crypto.Keccak256(append(request.CodeHash.Bytes(), verificationRewardsAddress.Bytes()...))
	signature, err := wallet.SignHash(account, unsignedMessage)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d:%d:%s", base64.URLEncoding.EncodeToString(signature), request.RequestIndex, request.VerificationIndex, base64.URLEncoding.EncodeToString(verificationRewardsAddress.Bytes())), nil
}

func sendSms(phoneNumber string, message string, account common.Address, verificationServiceURL string) error {
	values := map[string]string{"phoneNumber": phoneNumber, "message": message, "account": base64.URLEncoding.EncodeToString(account.Bytes())}
	jsonValue, _ := json.Marshal(values)
	var err error

	// Retry 5 times if we fail.
	for i := 0; i < 5; i++ {
		_, err := http.Post(verificationServiceURL, "application/json", bytes.NewBuffer(jsonValue))
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

func SendVerificationMessages(receipts []*types.Receipt, block *types.Block, coinbase common.Address, accountManager *accounts.Manager, verificationServiceURL string, verificationRewardsAddress common.Address) {
	account := accounts.Account{Address: coinbase}
	wallet, err := accountManager.Find(account)
	if err != nil {
		log.Error("[Celo] Failed to get account for sms verification", "err", err)
		return
	}

	for _, receipt := range receipts {
		for _, request := range receipt.VerificationRequests {
			if !bytes.Equal(coinbase.Bytes(), request.Verifier.Bytes()) {
				continue
			}
			phoneNumber, err := decryptPhoneNumber(request, account, wallet)
			if err != nil {
				log.Error("[Celo] Failed to decrypt phone number", "err", err)
				continue
			}

			message, err := createVerificationMessage(request, verificationRewardsAddress, account, wallet)
			if err != nil {
				log.Error("[Celo] Failed to create verification message", "err", err)
				continue
			}

			log.Debug(fmt.Sprintf("[Celo] Sending verification message: \"%s\"", message), nil, nil)
			err = sendSms(phoneNumber, message, request.Account, verificationServiceURL)
			if err != nil {
				log.Error("[Celo] Failed to send SMS", "err", err)
			}
		}
	}
}
