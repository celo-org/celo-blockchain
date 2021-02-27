package genesis

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/fixed"
	"github.com/celo-org/celo-blockchain/mycelo/internal/utils"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/shopspring/decimal"
)

// durations in seconds
const (
	Second = 1
	Minute = 60 * Second
	Hour   = 60 * Minute
	Day    = 24 * Hour
	Week   = 7 * Day
	Year   = 365 * Day
)

// Config represent all celo-blockchain configuration options for the genesis block
type Config struct {
	ChainID          *big.Int              `json:"chainId"` // chainId identifies the current chain and is used for replay protection
	Istanbul         params.IstanbulConfig `json:"istanbul"`
	Hardforks        HardforkConfig        `json:"hardforks"`
	GenesisTimestamp uint64                `json:"genesisTimestamp"`

	SortedOracles              SortedOraclesParameters
	GasPriceMinimum            GasPriceMinimumParameters
	Reserve                    ReserveParameters
	StableToken                StableTokenParameters
	StableTokenEUR             StableTokenParameters
	Exchange                   ExchangeParameters
	ExchangeEUR                ExchangeParameters
	LockedGold                 LockedGoldParameters
	GoldToken                  GoldTokenParameters
	Validators                 ValidatorsParameters
	Election                   ElectionParameters
	EpochRewards               EpochRewardsParameters
	Blockchain                 BlockchainParameters
	Random                     RandomParameters
	Attestations               AttestationsParameters
	TransferWhitelist          TransferWhitelistParameters
	ReserveSpenderMultiSig     MultiSigParameters
	GovernanceApproverMultiSig MultiSigParameters
	DoubleSigningSlasher       DoubleSigningSlasherParameters
	DowntimeSlasher            DowntimeSlasherParameters
	Governance                 GovernanceParameters
}

// Save will write config into a json file
func (cfg *Config) Save(filepath string) error {
	return utils.WriteJson(cfg, filepath)
}

// LoadConfig will read config from a json file
func LoadConfig(filepath string) (*Config, error) {
	var cfg Config
	if err := utils.ReadJson(&cfg, filepath); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ChainConfig returns the chain config objt for the blockchain
func (cfg *Config) ChainConfig() *params.ChainConfig {
	return &params.ChainConfig{
		ChainID:             cfg.ChainID,
		HomesteadBlock:      common.Big0,
		EIP150Block:         common.Big0,
		EIP150Hash:          common.Hash{},
		EIP155Block:         common.Big0,
		EIP158Block:         common.Big0,
		ByzantiumBlock:      common.Big0,
		ConstantinopleBlock: common.Big0,
		PetersburgBlock:     common.Big0,
		IstanbulBlock:       common.Big0,

		ChurritoBlock: cfg.Hardforks.ChurritoBlock,
		DonutBlock:    cfg.Hardforks.DonutBlock,

		Istanbul: &params.IstanbulConfig{
			Epoch:          cfg.Istanbul.Epoch,
			ProposerPolicy: cfg.Istanbul.ProposerPolicy,
			LookbackWindow: cfg.Istanbul.LookbackWindow,
			BlockPeriod:    cfg.Istanbul.BlockPeriod,
			RequestTimeout: cfg.Istanbul.RequestTimeout,
		},
	}
}

// HardforkConfig contains celo hardforks activation blocks
type HardforkConfig struct {
	ChurritoBlock *big.Int `json:"churritoBlock"`
	DonutBlock    *big.Int `json:"donutBlock"`
}

// MultiSigParameters are the initial configuration parameters for a MultiSig contract
type MultiSigParameters struct {
	Signatories                      []common.Address `json:"signatories"`
	NumRequiredConfirmations         uint64           `json:"numRequiredConfirmations"`
	NumInternalRequiredConfirmations uint64           `json:"numInternalRequiredConfirmations"`
}

// LockedGoldRequirements represents value/duration requirments on locked gold
type LockedGoldRequirements struct {
	Value    *big.Int `json:"value"`
	Duration *big.Int `json:"duration"`
}

// ElectionParameters are the initial configuration parameters for Elections
type ElectionParameters struct {
	MinElectableValidators *big.Int     `json:"minElectableValidators"`
	MaxElectableValidators *big.Int     `json:"maxElectableValidators"`
	MaxVotesPerAccount     *big.Int     `json:"maxVotesPerAccount"`
	ElectabilityThreshold  *fixed.Fixed `json:"electabilityThreshold"`
}

// Version represents an artifact version number
type Version struct {
	Major int64 `json:"major"`
	Minor int64 `json:"minor"`
	Patch int64 `json:"patch"`
}

// BlockchainParameters are the initial configuration parameters for Blockchain
type BlockchainParameters struct {
	Version                 Version  `json:"version"`
	GasForNonGoldCurrencies *big.Int `json:"gasForNonGoldCurrencies"`
	BlockGasLimit           *big.Int `json:"blockGasLimit"`
	UptimeLookbackWindow    int64    `json:"uptimeLookbackWindow"`
}

// DoubleSigningSlasherParameters are the initial configuration parameters for DoubleSigningSlasher
type DoubleSigningSlasherParameters struct {
	Penalty *big.Int `json:"penalty"`
	Reward  *big.Int `json:"reward"`
}

// DowntimeSlasherParameters are the initial configuration parameters for DowntimeSlasher
type DowntimeSlasherParameters struct {
	Penalty           *big.Int `json:"penalty"`
	Reward            *big.Int `json:"reward"`
	SlashableDowntime uint64   `json:"slashableDowntime"`
}

// GovernanceParameters are the initial configuration parameters for Governance
type GovernanceParameters struct {
	UseMultiSig             bool         `json:"useMultiSig"` // whether the approver should be the multisig (otherwise it's the admin)
	ConcurrentProposals     uint64       `json:"concurrentProposals"`
	MinDeposit              *big.Int     `json:"MinDeposit"`
	QueueExpiry             uint64       `json:"QueueExpiry"`
	DequeueFrequency        uint64       `json:"DequeueFrequency"`
	ApprovalStageDuration   uint64       `json:"ApprovalStageDuration"`
	ReferendumStageDuration uint64       `json:"ReferendumStageDuration"`
	ExecutionStageDuration  uint64       `json:"ExecutionStageDuration"`
	ParticipationBaseline   *fixed.Fixed `json:"participationBaseline"`
	ParticipationFloor      *fixed.Fixed `json:"participationFloor"`
	BaselineUpdateFactor    *fixed.Fixed `json:"BaselineUpdateFactor"`
	BaselineQuorumFactor    *fixed.Fixed `json:"BaselineQuorumFactor"`
}

// ValidatorsParameters are the initial configuration parameters for Validators
type ValidatorsParameters struct {
	GroupLockedGoldRequirements     LockedGoldRequirements `json:"groupLockedGoldRequirements"`
	ValidatorLockedGoldRequirements LockedGoldRequirements `json:"validatorLockedGoldRequirements"`
	ValidatorScoreExponent          *big.Int               `json:"validatorScoreExponent"`
	ValidatorScoreAdjustmentSpeed   *fixed.Fixed           `json:"validatorScoreAdjustmentSpeed"`
	MembershipHistoryLength         *big.Int               `json:"membershipHistoryLength"`
	SlashingPenaltyResetPeriod      *big.Int               `json:"slashingPenaltyResetPeriod"`
	MaxGroupSize                    *big.Int               `json:"maxGroupSize"`
	CommissionUpdateDelay           *big.Int               `json:"commissionUpdateDelay"`
	DowntimeGracePeriod             *big.Int               `json:"downtimeGracePeriod"`

	Commission *fixed.Fixed `json:"commission"` // commision for genesis registered validator groups
}

// EpochRewardsParameters are the initial configuration parameters for EpochRewards
type EpochRewardsParameters struct {
	TargetVotingYieldInitial                     *fixed.Fixed   `json:"targetVotingYieldInitial"`
	TargetVotingYieldMax                         *fixed.Fixed   `json:"targetVotingYieldMax"`
	TargetVotingYieldAdjustmentFactor            *fixed.Fixed   `json:"targetVotingYieldAdjustmentFactor"`
	RewardsMultiplierMax                         *fixed.Fixed   `json:"rewardsMultiplierMax"`
	RewardsMultiplierAdjustmentFactorsUnderspend *fixed.Fixed   `json:"rewardsMultiplierAdjustmentFactorsUnderspend"`
	RewardsMultiplierAdjustmentFactorsOverspend  *fixed.Fixed   `json:"rewardsMultiplierAdjustmentFactorsOverspend"`
	TargetVotingGoldFraction                     *fixed.Fixed   `json:"targetVotingGoldFraction"`
	MaxValidatorEpochPayment                     *big.Int       `json:"maxValidatorEpochPayment"`
	CommunityRewardFraction                      *fixed.Fixed   `json:"communityRewardFraction"`
	CarbonOffsettingPartner                      common.Address `json:"carbonOffsettingPartner"`
	CarbonOffsettingFraction                     *fixed.Fixed   `json:"carbonOffsettingFraction"`
	Frozen                                       bool           `json:"frozen"`
}

// TransferWhitelistParameters are the initial configuration parameters for TransferWhitelist
type TransferWhitelistParameters struct {
	Addresses   []common.Address `json:"addresses"`
	RegistryIDs []common.Hash    `json:"registryIds"`
}

// GoldTokenParameters are the initial configuration parameters for GoldToken
type GoldTokenParameters struct {
	Frozen          bool        `json:"frozen"`
	InitialBalances BalanceList `json:"initialBalances"`
}

// RandomParameters are the initial configuration parameters for Random
type RandomParameters struct {
	RandomnessBlockRetentionWindow *big.Int `json:"randomnessBlockRetentionWindow"`
}

// AttestationsParameters are the initial configuration parameters for Attestations
type AttestationsParameters struct {
	AttestationExpiryBlocks        *big.Int        `json:"attestationExpiryBlocks"`
	SelectIssuersWaitBlocks        *big.Int        `json:"selectIssuersWaitBlocks"`
	MaxAttestations                *big.Int        `json:"maxAttestations"`
	AttestationRequestFeeInDollars decimal.Decimal `json:"AttestationRequestFeeInDollars"`
}

// SortedOraclesParameters are the initial configuration parameters for SortedOracles
type SortedOraclesParameters struct {
	ReportExpirySeconds int64 `json:"reportExpirySeconds"`
}

// GasPriceMinimumParameters are the initial configuration parameters for GasPriceMinimum
type GasPriceMinimumParameters struct {
	MinimumFloor    *big.Int     `json:"minimumFloor"`
	TargetDensity   *fixed.Fixed `json:"targetDensity"`
	AdjustmentSpeed *fixed.Fixed `json:"adjustmentSpeed"`
}

// ReserveParameters are the initial configuration parameters for Reserve
type ReserveParameters struct {
	TobinTaxStalenessThreshold *big.Int            `json:"tobinTaxStalenessThreshold"`
	DailySpendingRatio         *big.Int            `json:"dailySpendingRatio"`
	FrozenGold                 *big.Int            `json:"frozenGold"`
	FrozenDays                 *big.Int            `json:"frozenDays"`
	AssetAllocations           AssetAllocationList `json:"assetAllocations"`
	TobinTax                   *big.Int            `json:"tobinTax"`
	TobinTaxReserveRatio       *big.Int            `json:"tobinTaxReserveRatio"`

	// Other parameters
	Spenders                 []common.Address `json:"spenders"`
	OtherAddresses           []common.Address `json:"otherAddresses"`
	InitialBalance           *big.Int         `json:"initialBalance"`
	FrozenAssetsStartBalance *big.Int         `json:"frozenAssetsStartBalance"`
	FrozenAssetsDays         *big.Int         `json:"frozenAssetsDays"`
}

// StableTokenParameters are the initial configuration parameters for StableToken
type StableTokenParameters struct {
	Name                        string           `json:"name"`
	Symbol                      string           `json:"symbol"`
	Decimals                    uint8            `json:"decimals"`
	Rate                        *fixed.Fixed     `json:"rate"`
	InflationFactorUpdatePeriod *big.Int         `json:"inflationFactorUpdatePeriod"` // How often the inflation factor is updated.
	InitialBalances             BalanceList      `json:"initialBalances"`
	Frozen                      bool             `json:"frozen"`
	Oracles                     []common.Address `json:"oracles"`
	GoldPrice                   *fixed.Fixed     `json:"goldPrice"`
	ExchangeIdentifier          string           `json:"exchangeIdentifier"`
}

// ExchangeParameters are the initial configuration parameters for Exchange
type ExchangeParameters struct {
	Frozen          bool         `json:"frozen"`
	Spread          *fixed.Fixed `json:"spread"`
	ReserveFraction *fixed.Fixed `json:"reserveFraction"`
	UpdateFrequency uint64       `json:"updateFrequency"`
	MinimumReports  uint64       `json:"minimumReports"`
}

// LockedGoldParameters are the initial configuration parameters for LockedGold
type LockedGoldParameters struct {
	UnlockingPeriod *big.Int `json:"unlockingPeriod"`
}

// Balance represents an account and it's initial balance in wei
type Balance struct {
	Account common.Address `json:"account"`
	Amount  *big.Int       `json:"amount"`
}

// BalanceList list of balances
type BalanceList []Balance

// Accounts returns all the addresses
func (bl BalanceList) Accounts() []common.Address {
	res := make([]common.Address, len(bl))
	for i, x := range bl {
		res[i] = x.Account
	}
	return res
}

// Amounts returns all the amounts
func (bl BalanceList) Amounts() []*big.Int {
	res := make([]*big.Int, len(bl))
	for i, x := range bl {
		res[i] = x.Amount
	}
	return res
}

// AssetAllocation config for Reserve
type AssetAllocation struct {
	Symbol string       `json:"symbol"`
	Weight *fixed.Fixed `json:"weight"`
}

// AssetAllocationList list of AssetAllocation
type AssetAllocationList []AssetAllocation

// SymbolsABI returns symbols in ABI format for assets in list
func (aa AssetAllocationList) SymbolsABI() []common.Hash {
	res := make([]common.Hash, len(aa))
	for i, x := range aa {

		res[i] = common.BytesToHash(common.RightPadBytes([]byte(x.Symbol), 32))
	}
	return res
}

// Weights returns weights for assets in list
func (aa AssetAllocationList) Weights() []*big.Int {
	res := make([]*big.Int, len(aa))
	for i, x := range aa {
		res[i] = x.Weight.BigInt()
	}
	return res
}
