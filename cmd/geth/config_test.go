package main

import (
	"bytes"
	"testing"

	"github.com/celo-org/celo-blockchain/eth"
	"github.com/celo-org/celo-blockchain/p2p/nat"
	whisper "github.com/celo-org/celo-blockchain/whisper/whisperv6"
)

// testConfigMarshal will test the process of marshal/unmarshal.
func testConfigMarshal(t *testing.T, cfg gethConfig) {
	// 1. Marshal cfg, get cfgMarshal
	cfgMarshal, err := tomlSettings.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Unmarshal cfgMarshal, get outCfg. Then marshal outCfg, get outCfgMarshal.
	var outCfg gethConfig
	err = tomlSettings.NewDecoder(bytes.NewReader(cfgMarshal)).Decode(&outCfg)
	if err != nil {
		t.Fatal(err)
	}
	outCfgMarshal, err := tomlSettings.Marshal(&outCfg)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Compare cfgMarshal & outCfgMarshal
	if string(cfgMarshal) != string(outCfgMarshal) {
		t.Fatal("Not equal")
	}
}

func Test(t *testing.T) {
	testCases := []struct {
		name   string
		config func() gethConfig
	}{
		{
			name: "defaultConfig",
			config: func() gethConfig {
				return gethConfig{
					Eth:  eth.DefaultConfig,
					Shh:  whisper.DefaultConfig,
					Node: defaultNodeConfig(),
				}
			},
		},
		{
			name: "NAT.extip",
			config: func() gethConfig {
				cfg := gethConfig{
					Eth:  eth.DefaultConfig,
					Shh:  whisper.DefaultConfig,
					Node: defaultNodeConfig(),
				}
				cfg.Node.P2P.NAT, _ = nat.Parse("extip:190.168.10.15")
				return cfg
			},
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			testConfigMarshal(t, c.config())
		})
	}
}

//// TestDumpDefaultConfig checks en/decode on default config
//func TestDumpDefaultConfig(t *testing.T) {
//	test := runGeth(t, "dumpconfig")
//	test.Expect(`
//[Eth]
//NetworkId = 42220
//SyncMode = "fast"
//DiscoveryURLs = []
//NoPruning = false
//NoPrefetch = false
//LightPeers = 100
//GatewayFee = 0
//UltraLightFraction = 75
//DatabaseCache = 768
//DatabaseFreezer = ""
//TrieCleanCache = 256
//TrieDirtyCache = 256
//TrieTimeout = 3600000000000
//EnablePreimageRecording = false
//EWASMInterpreter = ""
//EVMInterpreter = ""
//
//[Eth.Miner]
//GasFloor = 8000000
//GasCeil = 8000000
//GasPrice = 1
//Recommit = 3000000000
//Noverify = false
//VerificationService = ""
//
//[Eth.TxPool]
//Locals = []
//NoLocals = false
//Journal = "transactions.rlp"
//Rejournal = 3600000000000
//PriceLimit = 0
//PriceBump = 10
//AccountSlots = 16
//GlobalSlots = 4096
//AccountQueue = 64
//GlobalQueue = 1024
//Lifetime = 10800000000000
//
//[Eth.Istanbul]
//RequestTimeout = 3000
//TimeoutBackoffFactor = 1000
//MinResendRoundChangeTimeout = 15000
//MaxResendRoundChangeTimeout = 120000
//BlockPeriod = 5
//ProposerPolicy = 2
//Epoch = 30000
//DefaultLookbackWindow = 12
//ReplicaStateDBPath = "/var/folders/ng/4zlhpv0x2wj4xjv95ftbxnfm0000gn/T/geth-test827509254/celo/replicastate"
//ValidatorEnodeDBPath = "/var/folders/ng/4zlhpv0x2wj4xjv95ftbxnfm0000gn/T/geth-test827509254/celo/validatorenodes"
//VersionCertificateDBPath = "/var/folders/ng/4zlhpv0x2wj4xjv95ftbxnfm0000gn/T/geth-test827509254/celo/versioncertificates"
//RoundStateDBPath = "/var/folders/ng/4zlhpv0x2wj4xjv95ftbxnfm0000gn/T/geth-test827509254/celo/roundstates"
//AnnounceQueryEnodeGossipPeriod = 300
//AnnounceAggressiveQueryEnodeGossipOnEnablement = true
//AnnounceAdditionalValidatorsToGossip = 10
//
//[Shh]
//MaxMessageSize = 1048576
//MinimumAcceptedPOW = 2e-01
//RestrictConnectionBetweenLightClients = true
//
//[Node]
//DataDir = "/Users/tong/Library/Celo"
//Proxy = false
//IPCPath = "geth.ipc"
//HTTPPort = 8545
//HTTPVirtualHosts = ["localhost"]
//HTTPModules = ["net", "web3", "eth"]
//WSPort = 8546
//WSModules = ["net", "web3", "eth"]
//GraphQLPort = 8547
//GraphQLVirtualHosts = ["localhost"]
//
//[Node.P2P]
//MaxPeers = 175
//NoDiscovery = false
//BootstrapNodes = ["enode://5c9a3afb564b48cc2fa2e06b76d0c5d8f6910e1930ea7d0930213a0cbc20450434cd442f6483688eff436ad14dc29cb90c9592cc5c1d27ca62f28d4d8475d932@34.82.79.155:30301"]
//StaticNodes = []
//TrustedNodes = []
//PingIPFromPacket = false
//UseInMemoryNodeDatabase = false
//ListenAddr = ":30303"
//NetworkId = 42220
//EnableMsgEvents = false
//
//[Node.ProxyP2P]
//MaxPeers = 1
//NoDiscovery = true
//BootstrapNodes = []
//StaticNodes = []
//TrustedNodes = []
//PingIPFromPacket = false
//UseInMemoryNodeDatabase = true
//ListenAddr = ":30503"
//NetworkId = 1
//EnableMsgEvents = false
//
//[Node.HTTPTimeouts]
//ReadTimeout = 30000000000
//WriteTimeout = 30000000000
//IdleTimeout = 120000000000
//`)
//}
//
//// TestDumpCustomConfig checks en/decode on custom flags
//func TestDumpCustomConfig(t *testing.T) {
//	//  Node configs --------------------------------------------------------------
//
//	// datadir
//	datadir := "/tmp/celo/test"
//	runGeth(t, "dumpconfig", "--datadir", datadir).ExpectRegexp(`DataDir = "` + datadir + `"`)
//
//	// Proxy
//	runGeth(t, "dumpconfig").ExpectRegexp(`Proxy = false`) // default
//	runGeth(t, "dumpconfig", "--proxy.proxy",
//		"--proxy.proxiedvalidatoraddress", "0x00998765054761ff5df85154c888b32ad48bd5f2",
//		"--proxy.internalendpoint", ":30503").ExpectRegexp(`Proxy = true`)
//
//	// NAT
//	runGeth(t, "dumpconfig", "--nat", "extip:190.168.10.15").ExpectRegexp(`NAT = \[0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 190, 168, 10, 15\]`)
//}
