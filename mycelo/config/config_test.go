package config

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/params"

	. "github.com/onsi/gomega"
)

func TestConfigMarhalling(t *testing.T) {
	RegisterTestingT(t)

	cfg := Config{
		ChainID: big.NewInt(1500),
		Istanbul: params.IstanbulConfig{
			Epoch:          45555,
			ProposerPolicy: 2,
			LookbackWindow: 12,
			BlockPeriod:    45,
			RequestTimeout: 5000,
		},
		Hardforks: HardforkConfig{
			ChurritoBlock: big.NewInt(1555),
			DonutBlock:    big.NewInt(33333),
		},
		GenesisTimestamp:   9999,
		Mnemonic:           "aloha hawai",
		InitialValidators:  6,
		ValidatorsPerGroup: 2,
	}

	raw, err := json.Marshal(cfg)
	Ω(err).ShouldNot(HaveOccurred())

	raw2, err := json.MarshalIndent(cfg, " ", " ")
	fmt.Println(string(raw2))

	var resultCfg Config
	err = json.Unmarshal(raw, &resultCfg)
	Ω(err).ShouldNot(HaveOccurred())

	Ω(resultCfg).Should(Equal(cfg))
}
func TestConfigReadJson(t *testing.T) {
	RegisterTestingT(t)

	jsonStr := []byte(`{
		"chainId": 1500,
		"istanbul": {
		 "epoch": 45555,
		 "policy": 2,
		 "lookbackwindow": 12,
		 "blockperiod": 45,
		 "requesttimeout": 5000
		},
		"hardforks": {
		 "churritoBlock": 1555,
		 "donutBlock": 33333
		},
		"genesisTimestamp": 9999,
		"mnemonic": "aloha hawai",
		"initialValidators": 6,
		"validatorsPerGroup": 2
	 }`)

	expectedCfg := Config{
		ChainID: big.NewInt(1500),
		Istanbul: params.IstanbulConfig{
			Epoch:          45555,
			ProposerPolicy: 2,
			LookbackWindow: 12,
			BlockPeriod:    45,
			RequestTimeout: 5000,
		},
		Hardforks: HardforkConfig{
			ChurritoBlock: big.NewInt(1555),
			DonutBlock:    big.NewInt(33333),
		},
		GenesisTimestamp:   9999,
		Mnemonic:           "aloha hawai",
		InitialValidators:  6,
		ValidatorsPerGroup: 2,
	}

	var resultCfg Config
	err := json.Unmarshal(jsonStr, &resultCfg)
	Ω(err).ShouldNot(HaveOccurred())
	Ω(err).ShouldNot(HaveOccurred())

	Ω(resultCfg).Should(Equal(expectedCfg))

}
