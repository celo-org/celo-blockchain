package genesis

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/mycelo/config"
	"github.com/ethereum/go-ethereum/rlp"
)

// Keccak256 of "The Times 09/Apr/2020 With $2.3 Trillion Injection, Fed’s Plan Far Exceeds Its 2008 Rescue"
var genesisMsgHash = common.HexToHash("ecc833a7747eaa8327335e8e0c6b6d8aa3a38d0063591e43ce116ccf5c89753e")

// GenerateGenesis will create a new genesis block with full celo blockchain already configured
func GenerateGenesis(admin config.Account, validators []config.Account, cfg *Config, contractsBuildPath string) (*core.Genesis, error) {

	extraData, err := generateGenesisExtraData(validators)
	if err != nil {
		return nil, err
	}

	genesisAlloc, err := generateGenesisState(admin, cfg, contractsBuildPath)
	if err != nil {
		return nil, err
	}

	return &core.Genesis{
		Config:    cfg.ChainConfig(),
		ExtraData: extraData,
		Coinbase:  admin.Address,
		Timestamp: cfg.GenesisTimestamp,
		Alloc:     genesisAlloc,
	}, nil
}

func generateGenesisExtraData(validatorAccounts []config.Account) ([]byte, error) {
	addresses := make([]common.Address, len(validatorAccounts))
	blsKeys := make([]blscrypto.SerializedPublicKey, len(validatorAccounts))

	for i := 0; i < len(validatorAccounts); i++ {
		var err error
		addresses[i] = validatorAccounts[i].Address
		blsKeys[i], err = validatorAccounts[i].BLSPublicKey()
		if err != nil {
			return nil, err
		}
	}

	istExtra := types.IstanbulExtra{
		AddedValidators:           addresses,
		AddedValidatorsPublicKeys: blsKeys,
		RemovedValidators:         big.NewInt(0),
		Seal:                      []byte{},
		AggregatedSeal:            types.IstanbulAggregatedSeal{},
		ParentAggregatedSeal:      types.IstanbulAggregatedSeal{},
	}

	payload, err := rlp.EncodeToBytes(&istExtra)
	if err != nil {
		return nil, err
	}

	var extraBytes []byte
	extraBytes = append(extraBytes, genesisMsgHash.Bytes()...)
	extraBytes = append(extraBytes, payload...)

	return extraBytes, nil
}
