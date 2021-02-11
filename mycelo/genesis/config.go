package genesis

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/mycelo/fixed"
	"github.com/ethereum/go-ethereum/mycelo/internal/utils"
	"github.com/ethereum/go-ethereum/params"
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

type Config struct {
	ChainID          *big.Int              `json:"chainId"` // chainId identifies the current chain and is used for replay protection
	Istanbul         params.IstanbulConfig `json:"istanbul"`
	Hardforks        HardforkConfig        `json:"hardforks"`
	GenesisTimestamp uint64                `json:"genesisTimestamp"`

	SortedOracles              SortedOraclesParameters
	GasPriceMinimum            GasPriceMinimumParameters
	Reserve                    ReserveParameters
	StableToken                StableTokenParameters
	Exchange                   ExchangeParameters
	LockedGold                 LockedGoldParameters
	GoldToken                  GoldTokenParameters
	Validators                 ValidatorsParameters
	Election                   ElectionParameters
	EpochRewards               EpochRewardsParameters
	Blockchain                 BlockchainParameters
	Random                     RandomParameters
	TransferWhitelist          TransferWhitelistParameters
	ReserveSpenderMultiSig     MultiSigParameters
	GovernanceApproverMultiSig MultiSigParameters
	DoubleSigningSlasher       DoubleSigningSlasherParameters
	DowntimeSlasher            DowntimeSlasherParameters
}

// BaseConfig creates base parameters for celo
// Callers must complete missing pieces
func BaseConfig() *Config {

	return &Config{
		SortedOracles: SortedOraclesParameters{
			ReportExpirySeconds: 5 * Minute,
		},
		GasPriceMinimum: GasPriceMinimumParameters{
			MinimunFloor:    new(big.Int).SetUint64(100000000),
			AdjustmentSpeed: fixed.MustNew("0.5"),
			TargetDensity:   fixed.MustNew("0.5"),
		},
		Reserve: ReserveParameters{
			TobinTaxStalenessThreshold: big.NewInt(3153600000),
			TobinTax:                   big.NewInt(0),
			TobinTaxReserveRatio:       big.NewInt(0),
			DailySpendingRatio:         MustBigInt("50000000000000000000000"),
			FrozenDays:                 nil,
			FrozenGold:                 nil,
			AssetAllocations: AssetAllocationList{
				{"cGLD", fixed.MustNew("0.5")},
				{"BTC", fixed.MustNew("0.3")},
				{"ETH", fixed.MustNew("0.15")},
				{"DAI", fixed.MustNew("0.05")},
			},
		},
		StableToken: StableTokenParameters{
			Name:                        "Celo Dollar",
			Symbol:                      "cUSD",
			Decimals:                    18,
			Rate:                        fixed.MustNew("1"),
			InflationFactorUpdatePeriod: big.NewInt(2 * Year),
			GoldPrice:                   fixed.MustNew("1"),
		},
		Validators: ValidatorsParameters{
			GroupLockedGoldRequirements: LockedGoldRequirements{
				Value:    MustBigInt("10000000000000000000000"), // 10k CELO per validator
				Duration: big.NewInt(180 * Day),
			},
			ValidatorLockedGoldRequirements: LockedGoldRequirements{
				Value: MustBigInt("10000000000000000000000"), // 10k CELO
				// MUST BE KEPT IN SYNC WITH MEMBERSHIP HISTORY LENGTH
				Duration: big.NewInt(60 * Day),
			},
			ValidatorScoreExponent:        big.NewInt(10),
			ValidatorScoreAdjustmentSpeed: fixed.MustNew("0.1"),

			// MUST BE KEPT IN SYNC WITH VALIDATOR LOCKED GOLD DURATION
			MembershipHistoryLength: big.NewInt(60),

			CommissionUpdateDelay: big.NewInt((3 * Day) / 5), // Approximately 3 days with 5s block times
			MaxGroupSize:          big.NewInt(5),

			SlashingPenaltyResetPeriod: big.NewInt(30 * Day),
		},
		Election: ElectionParameters{
			MinElectableValidators: big.NewInt(1),
			MaxElectableValidators: big.NewInt(100),
			MaxVotesPerAccount:     big.NewInt(10),
			ElectabilityThreshold:  fixed.MustNew("0.001"),
		},
		Exchange: ExchangeParameters{
			Spread:          fixed.MustNew("0.005"),
			ReserveFraction: fixed.MustNew("0.01"),
			UpdateFrequency: 5 * Minute,
			MinimumReports:  1,
			Frozen:          false,
		},
		EpochRewards: EpochRewardsParameters{
			TargetVotingYieldInitial:                     fixed.MustNew("0"),      // Change to (x + 1) ^ 365 = 1.06 once Mainnet activated.
			TargetVotingYieldAdjustmentFactor:            fixed.MustNew("0"),      // Change to 1 / 3650 once Mainnet activated.,
			TargetVotingYieldMax:                         fixed.MustNew("0.0005"), // (x + 1) ^ 365 = 1.20
			RewardsMultiplierMax:                         fixed.MustNew("2"),
			RewardsMultiplierAdjustmentFactorsUnderspend: fixed.MustNew("0.5"),
			RewardsMultiplierAdjustmentFactorsOverspend:  fixed.MustNew("5"),

			// Intentionally set lower than the expected value at steady state to account for the fact that
			// users may take some time to start voting with their cGLD.
			TargetVotingGoldFraction: fixed.MustNew("0.5"),
			MaxValidatorEpochPayment: MustBigInt("205479452054794520547"), // (75,000 / 365) * 10 ^ 18
			CommunityRewardFraction:  fixed.MustNew("0.25"),
			CarbonOffsettingPartner:  common.Address{},
			CarbonOffsettingFraction: fixed.MustNew("0.001"),

			Frozen: false,
		},
		LockedGold: LockedGoldParameters{
			UnlockingPeriod: big.NewInt(259200),
		},
		Random: RandomParameters{
			RandomnessBlockRetentionWindow: big.NewInt(720),
		},
		TransferWhitelist: TransferWhitelistParameters{},
		GoldToken: GoldTokenParameters{
			Frozen: false,
		},
		Blockchain: BlockchainParameters{
			Version:                 Version{1, 0, 0},
			GasForNonGoldCurrencies: big.NewInt(50000),
			BlockGasLimit:           big.NewInt(13000000),
			UptimeLookbackWindow:    12,
		},
		DoubleSigningSlasher: DoubleSigningSlasherParameters{
			Reward:  MustBigInt("1000000000000000000000"), // 1000 cGLD
			Penalty: MustBigInt("9000000000000000000000"), // 9000 cGLD
		},
		DowntimeSlasher: DowntimeSlasherParameters{
			Reward:            MustBigInt("10000000000000000000"),  // 10 cGLD
			Penalty:           MustBigInt("100000000000000000000"), // 100 cGLD
			SlashableDowntime: 60,                                  // Should be overridden on public testnets
		},
	}
}

func SaveConfig(cfg *Config, filepath string) error {
	return utils.WriteJson(cfg, filepath)
}

func LoadConfig(filepath string) (*Config, error) {
	var cfg Config
	if err := utils.ReadJson(&cfg, filepath); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (cfg *Config) ChainConfig() *params.ChainConfig {
	return &params.ChainConfig{
		ChainID:             cfg.ChainID,
		HomesteadBlock:      big.NewInt(0),
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.Hash{},
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),

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

// SortedOraclesParameters are the initial configuration parameters for SortedOracles
type SortedOraclesParameters struct {
	ReportExpirySeconds int64 `json:"reportExpirySeconds"`
}

// GasPriceMinimumParameters are the initial configuration parameters for GasPriceMinimum
type GasPriceMinimumParameters struct {
	MinimunFloor    *big.Int     `json:"minimunFloor"`
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

func MustBigInt(str string) *big.Int {
	i, ok := new(big.Int).SetString(str, 10)
	if !ok {
		panic(fmt.Errorf("Invalid string for big.Int: %s", str))
	}
	return i
}
