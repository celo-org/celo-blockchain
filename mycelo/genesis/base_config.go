package genesis

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/decimal/fixed"
	"github.com/shopspring/decimal"
)

// BaseConfig creates base parameters for celo
// Callers must complete missing pieces
func BaseConfig(gingerbreadBlock *big.Int) *Config {
	bigInt := big.NewInt
	bigIntStr := common.MustBigInt
	fixed := fixed.MustNew
	decimal := decimal.RequireFromString

	return &Config{
		SortedOracles: SortedOraclesParameters{
			ReportExpirySeconds: 5 * Minute,
		},
		GasPriceMinimum: GasPriceMinimumParameters{
			MinimumFloor:                 bigInt(100000000),
			AdjustmentSpeed:              fixed("0.5"),
			TargetDensity:                fixed("0.5"),
			BaseFeeOpCodeActivationBlock: gingerbreadBlock,
		},
		Reserve: ReserveParameters{
			TobinTaxStalenessThreshold: 3153600000,
			TobinTax:                   fixed("0"),
			TobinTaxReserveRatio:       fixed("0"),
			DailySpendingRatio:         fixed("0.05"),
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
			InflationFactorUpdatePeriod: 2 * Year,
			GoldPrice:                   fixed("1"),
			ExchangeIdentifier:          "Exchange",
		},
		StableTokenEUR: StableTokenParameters{
			Name:                        "Celo Euro",
			Symbol:                      "cEUR",
			Decimals:                    18,
			Rate:                        fixed("1"),
			InflationFactorUpdatePeriod: 2 * Year,
			GoldPrice:                   fixed("1"),
			ExchangeIdentifier:          "ExchangeEUR",
		},
		StableTokenBRL: StableTokenParameters{
			Name:                        "Celo Brazilian Real",
			Symbol:                      "cREAL",
			Decimals:                    18,
			Rate:                        fixed("1"),
			InflationFactorUpdatePeriod: 2 * Year,
			GoldPrice:                   fixed("1"),
			ExchangeIdentifier:          "ExchangeBRL",
		},
		Validators: ValidatorsParameters{
			GroupLockedGoldRequirements: LockedGoldRequirements{
				Value:    bigIntStr("10000000000000000000000"), // 10k CELO per validator
				Duration: 180 * Day,
			},
			ValidatorLockedGoldRequirements: LockedGoldRequirements{
				Value: bigIntStr("10000000000000000000000"), // 10k CELO
				// MUST BE KEPT IN SYNC WITH MEMBERSHIP HISTORY LENGTH
				Duration: 60 * Day,
			},
			ValidatorScoreExponent:        10,
			ValidatorScoreAdjustmentSpeed: fixed("0.1"),

			// MUST BE KEPT IN SYNC WITH VALIDATOR LOCKED GOLD DURATION
			MembershipHistoryLength: 60,

			CommissionUpdateDelay: (3 * Day) / 5, // Approximately 3 days with 5s block times
			MaxGroupSize:          5,

			SlashingPenaltyResetPeriod: 30 * Day,

			DowntimeGracePeriod: 0,

			Commission: fixed("0.1"),
		},
		Election: ElectionParameters{
			MinElectableValidators: 1,
			MaxElectableValidators: 100,
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
		ExchangeEUR: ExchangeParameters{
			Spread:          fixed("0.005"),
			ReserveFraction: fixed("0.01"),
			UpdateFrequency: 5 * Minute,
			MinimumReports:  1,
			Frozen:          false,
		},
		ExchangeBRL: ExchangeParameters{
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
			UnlockingPeriod: 259200,
		},
		Random: RandomParameters{
			RandomnessBlockRetentionWindow: 720,
		},
		Attestations: AttestationsParameters{
			AttestationExpiryBlocks:        Hour / 5, // 1 hour assuming 5 second blocks, but ok anyway
			SelectIssuersWaitBlocks:        4,
			MaxAttestations:                100,
			AttestationRequestFeeInDollars: decimal("0.05"), // use decimal rather than fixed, since we use this to multiply by
		},
		GoldToken: GoldTokenParameters{
			Frozen: false,
		},
		Blockchain: BlockchainParameters{
			Version:                 Version{1, 0, 0},
			GasForNonGoldCurrencies: 50000,
			BlockGasLimit:           13000000,
		},
		DoubleSigningSlasher: DoubleSigningSlasherParameters{
			Reward:  bigIntStr("1000000000000000000000"), // 1000 cGLD
			Penalty: bigIntStr("9000000000000000000000"), // 9000 cGLD
		},
		DowntimeSlasher: DowntimeSlasherParameters{
			Reward:            bigIntStr("10000000000000000000"),  // 10 cGLD
			Penalty:           bigIntStr("100000000000000000000"), // 100 cGLD
			SlashableDowntime: 4,                                  // make it small so it works with small epoch sizes, e.g. 10
		},
		Governance: GovernanceParameters{
			UseMultiSig:             true,
			ConcurrentProposals:     3,
			MinDeposit:              bigIntStr("100000000000000000000"), // 100 cGLD
			QueueExpiry:             4 * Week,
			DequeueFrequency:        30 * Minute,
			ReferendumStageDuration: Hour,
			ExecutionStageDuration:  Day,
			ParticipationBaseline:   fixed("0.005"),
			ParticipationFloor:      fixed("0.01"),
			BaselineUpdateFactor:    fixed("0.2"),
			BaselineQuorumFactor:    fixed("1"),
		},
		GrandaMento: GrandaMentoParameters{
			MaxApprovalExchangeRateChange: fixed("0.3"),
			Spread:                        fixed("0.005"),
			VetoPeriodSeconds:             10,
			StableTokenExchangeLimits: []StableTokenExchangeLimit{
				{
					StableToken:       "StableToken",
					MinExchangeAmount: bigIntStr("50000000000000000000000"),
					MaxExchangeAmount: bigIntStr("50000000000000000000000000"),
				},
				{
					StableToken:       "StableTokenEUR",
					MinExchangeAmount: bigIntStr("40000000000000000000000"),
					MaxExchangeAmount: bigIntStr("40000000000000000000000000"),
				},
				{
					StableToken:       "StableTokenBRL",
					MinExchangeAmount: bigIntStr("40000000000000000000000"),
					MaxExchangeAmount: bigIntStr("40000000000000000000000000"),
				},
			},
		},
	}
}
