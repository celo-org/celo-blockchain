package env

import (
	"encoding/json"
	"math/big"
	"testing"

	. "github.com/onsi/gomega"
)

func TestConfigMarhalling(t *testing.T) {
	RegisterTestingT(t)

	cfg := Config{
		ChainID: big.NewInt(1500),
		Accounts: AccountsConfig{
			Mnemonic:           "aloha hawai",
			NumValidators:      6,
			ValidatorsPerGroup: 2,
		},
	}

	raw, err := json.Marshal(cfg)
	Ω(err).ShouldNot(HaveOccurred())

	// raw2, err := json.MarshalIndent(cfg, " ", " ")
	// Ω(err).ShouldNot(HaveOccurred())
	// fmt.Println(string(raw2))

	var resultCfg Config
	err = json.Unmarshal(raw, &resultCfg)
	Ω(err).ShouldNot(HaveOccurred())

	Ω(resultCfg).Should(Equal(cfg))
}
func TestConfigReadJson(t *testing.T) {
	RegisterTestingT(t)

	jsonStr := []byte(`{
		"chainId": 1500,
		"accounts": {
		  "mnemonic": "aloha hawai",
		  "validators": 6,
		  "validatorsPerGroup": 2
		}
	 }`)

	expectedCfg := Config{
		ChainID: big.NewInt(1500),
		Accounts: AccountsConfig{
			Mnemonic:           "aloha hawai",
			NumValidators:      6,
			ValidatorsPerGroup: 2,
		},
	}

	var resultCfg Config
	err := json.Unmarshal(jsonStr, &resultCfg)
	Ω(err).ShouldNot(HaveOccurred())
	Ω(err).ShouldNot(HaveOccurred())

	Ω(resultCfg).Should(Equal(expectedCfg))

}
