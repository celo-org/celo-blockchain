package config

const (
	thousand = 1000
	million  = 1000 * 1000

	// Default intrinsic gas cost of transactions paying for gas in alternative currencies.
	// Calculated to estimate 1 balance read, 1 debit, and 4 credit transactions.
	IntrinsicGasForAlternativeFeeCurrency uint64 = 50 * thousand

	// Contract communication gas limits
	MaxGasForCalculateTargetEpochPaymentAndRewards uint64 = 2 * million
	MaxGasForCommitments                           uint64 = 2 * million
	MaxGasForComputeCommitment                     uint64 = 2 * million
	MaxGasForBlockRandomness                       uint64 = 2 * million
	MaxGasForDebitGasFeesTransactions              uint64 = 1 * million
	MaxGasForCreditGasFeesTransactions             uint64 = 1 * million
	MaxGasForDistributeEpochPayment                uint64 = 1 * million
	MaxGasForDistributeEpochRewards                uint64 = 1 * million
	MaxGasForElectValidators                       uint64 = 50 * million
	MaxGasForElectNValidatorSigners                uint64 = 50 * million
	MaxGasForGetAddressFor                         uint64 = 100 * thousand
	MaxGasForGetElectableValidators                uint64 = 100 * thousand
	MaxGasForGetEligibleValidatorGroupsVoteTotals  uint64 = 1 * million
	MaxGasForGetGasPriceMinimum                    uint64 = 2 * million
	MaxGasForGetGroupEpochRewards                  uint64 = 500 * thousand
	MaxGasForGetMembershipInLastEpoch              uint64 = 1 * million
	MaxGasForGetOrComputeTobinTax                  uint64 = 1 * million
	MaxGasForGetRegisteredValidators               uint64 = 2 * million
	MaxGasForGetValidator                          uint64 = 100 * thousand
	MaxGasForGetWhiteList                          uint64 = 200 * thousand
	MaxGasForGetTransferWhitelist                  uint64 = 2 * million
	MaxGasForIncreaseSupply                        uint64 = 50 * thousand
	MaxGasForIsFrozen                              uint64 = 20 * thousand
	MaxGasForMedianRate                            uint64 = 100 * thousand
	MaxGasForReadBlockchainParameter               uint64 = 40 * thousand // ad-hoc measurement is ~26k
	MaxGasForRevealAndCommit                       uint64 = 2 * million
	MaxGasForUpdateGasPriceMinimum                 uint64 = 2 * million
	MaxGasForUpdateTargetVotingYield               uint64 = 2 * million
	MaxGasForUpdateValidatorScore                  uint64 = 1 * million
	MaxGasForTotalSupply                           uint64 = 50 * thousand
	MaxGasForMintGas                               uint64 = 5 * million
	MaxGasToReadErc20Balance                       uint64 = 100 * thousand
	MaxGasForIsReserveLow                          uint64 = 1 * million
	MaxGasForGetCarbonOffsettingPartner            uint64 = 20 * thousand
)
