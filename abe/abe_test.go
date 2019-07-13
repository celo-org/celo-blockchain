// Copyright 2019 The Celo Authors
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
	"encoding/hex"
	"testing"
)

func testPhone(phone string, hash string, t *testing.T) {
	hashedPhone, err := hashPhoneNumber([]byte(phone))

	if err != nil {
		t.Errorf("Failed")
	}

	hexHashedPhone := hex.EncodeToString(hashedPhone)

	if hexHashedPhone != hash {
		t.Errorf("Failed: %s", hexHashedPhone)
	}
}

func TestHashes(t *testing.T) {
	testPhone("+15551234567", "061aa54230532dcafcd3414717719e188f02814a6346a11bb0bc9324d119ccbc", t)
	testPhone("+15551234567", "061aa54230532dcafcd3414717719e188f02814a6346a11bb0bc9324d119ccbc", t)
	testPhone("+15330208284", "0fea6f884d7b2c294012eb4a6d2044de7726175ced4399121f8b729c2f3f170b", t)
	testPhone("+15072962425", "008776afb06c43891c161038b31650e3bf3c16da28aab2f28f543d58fee5666a", t)
	testPhone("+12244245019", "0b274799fcc8910a9062d9852c951163b9f56c188baf0bec4ab5c6cabc87985d", t)
	testPhone("+10231824169", "07f1a429fd5a45f939a7192f3b212e0c95947dca7d4bb51d4f28c8de1add501e", t)
	testPhone("+11230246230", "0adfce618e9ae272bb822d4701d9b88e432c2d94fb873e4614830189f56d1248", t)
	testPhone("+19850910695", "0832331e9d9e1328797f81008cde489ffd812d1d693f5281353a1c215e86bf1d", t)
	testPhone("+17527953770", "03c52bd33c9cf9c683ae9c8c3f1486d8b373f14066897691d25450f4f6055540", t)
	testPhone("+14458609041", "0afda5cce4b625c1e520751021b50fede8ffb25fb7316aad4eee30456a16902b", t)
	testPhone("+15551834326", "03fb92d9b5272ce3d94c814e68bb68d9f02ba7187c3c3a360943f77be6fa8404", t)
}
