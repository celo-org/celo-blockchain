package genesis

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/fixed"
)

// BaseConfig creates base parameters for celo
// Callers must complete missing pieces
func BaseConfig() *Config {
	bigInt := big.NewInt
	bigIntStr := common.MustBigInt
	fixed := fixed.MustNew

	return &Config{
		SortedOracles: SortedOraclesParameters{
			ReportExpirySeconds: 5 * Minute,
		},
		GasPriceMinimum: GasPriceMinimumParameters{
			MinimunFloor:    bigInt(100000000),
			AdjustmentSpeed: fixed("0.5"),
			TargetDensity:   fixed("0.5"),
		},
		Reserve: ReserveParameters{
			TobinTaxStalenessThreshold: bigInt(3153600000),
			TobinTax:                   common.Big0,
			TobinTaxReserveRatio:       common.Big0,
			DailySpendingRatio:         bigIntStr("50000000000000000000000"),
			FrozenDays:                 nil,
			FrozenGold:                 nil,
			AssetAllocations: AssetAllocationList{
				{"cGLD", fixed("0.5")},
				{"BTC", fixed("0.3")},
				{"ETH", fixed("0.15")},
				{"DAI", fixed("0.05")},
			},
		},
		StableToken: StableTokenParameters{
			Name:                        "Celo Dollar",
			Symbol:                      "cUSD",
			Decimals:                    18,
			Rate:                        fixed("1"),
			InflationFactorUpdatePeriod: bigInt(2 * Year),
			GoldPrice:                   fixed("1"),
		},
		Validators: ValidatorsParameters{
			GroupLockedGoldRequirements: LockedGoldRequirements{
				Value:    bigIntStr("10000000000000000000000"), // 10k CELO per validator
				Duration: bigInt(180 * Day),
			},
			ValidatorLockedGoldRequirements: LockedGoldRequirements{
				Value: bigIntStr("10000000000000000000000"), // 10k CELO
				// MUST BE KEPT IN SYNC WITH MEMBERSHIP HISTORY LENGTH
				Duration: bigInt(60 * Day),
			},
			ValidatorScoreExponent:        bigInt(10),
			ValidatorScoreAdjustmentSpeed: fixed("0.1"),

			// MUST BE KEPT IN SYNC WITH VALIDATOR LOCKED GOLD DURATION
			MembershipHistoryLength: bigInt(60),

			CommissionUpdateDelay: bigInt((3 * Day) / 5), // Approximately 3 days with 5s block times
			MaxGroupSize:          bigInt(5),

			SlashingPenaltyResetPeriod: bigInt(30 * Day),
		},
		Election: ElectionParameters{
			MinElectableValidators: bigInt(1),
			MaxElectableValidators: bigInt(100),
			MaxVotesPerAccount:     bigInt(10),
			ElectabilityThreshold:  fixed("0.001"),
		},
		Exchange: ExchangeParameters{
			Spread:          fixed("0.005"),
			ReserveFraction: fixed("0.01"),
			UpdateFrequency: 5 * Minute,
			MinimumReports:  1,
			Frozen:          false,
		},
		EpochRewards: EpochRewardsParameters{
			TargetVotingYieldInitial:                     fixed("0"),      // Change to (x + 1) ^ 365 = 1.06 once Mainnet activated.
			TargetVotingYieldAdjustmentFactor:            fixed("0"),      // Change to 1 / 3650 once Mainnet activated.,
			TargetVotingYieldMax:                         fixed("0.0005"), // (x + 1) ^ 365 = 1.20
			RewardsMultiplierMax:                         fixed("2"),
			RewardsMultiplierAdjustmentFactorsUnderspend: fixed("0.5"),
			RewardsMultiplierAdjustmentFactorsOverspend:  fixed("5"),

			// Intentionally set lower than the expected value at steady state to account for the fact that
			// users may take some time to start voting with their cGLD.
			TargetVotingGoldFraction: fixed("0.5"),
			MaxValidatorEpochPayment: bigIntStr("205479452054794520547"), // (75,000 / 365) * 10 ^ 18
			CommunityRewardFraction:  fixed("0.25"),
			CarbonOffsettingPartner:  common.Address{},
			CarbonOffsettingFraction: fixed("0.001"),

			Frozen: false,
		},
		LockedGold: LockedGoldParameters{
			UnlockingPeriod: bigInt(259200),
		},
		Random: RandomParameters{
			RandomnessBlockRetentionWindow: bigInt(720),
		},
		TransferWhitelist: TransferWhitelistParameters{},
		GoldToken: GoldTokenParameters{
			Frozen: false,
		},
		Blockchain: BlockchainParameters{
			Version:                 Version{1, 0, 0},
			GasForNonGoldCurrencies: bigInt(50000),
			BlockGasLimit:           bigInt(13000000),
			UptimeLookbackWindow:    12,
		},
		DoubleSigningSlasher: DoubleSigningSlasherParameters{
			Reward:  bigIntStr("1000000000000000000000"), // 1000 cGLD
			Penalty: bigIntStr("9000000000000000000000"), // 9000 cGLD
		},
		DowntimeSlasher: DowntimeSlasherParameters{
			Reward:            bigIntStr("10000000000000000000"),  // 10 cGLD
			Penalty:           bigIntStr("100000000000000000000"), // 100 cGLD
			SlashableDowntime: 60,                                 // Should be overridden on public testnets
		},
	}
}
