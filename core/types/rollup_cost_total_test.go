package types

import (
	"math/big"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func TestNewTotalRollupCostFuncNonOptimism(t *testing.T) {
	require.Nil(t, NewTotalRollupCostFunc(params.TestChainConfig, &testStateGetter{}))
}

func TestNewTotalRollupCostFuncIsthmusActivation(t *testing.T) {
	zero := uint64(0)
	isthmusTime := uint64(100)
	config := &params.ChainConfig{
		Optimism:     params.OptimismTestConfig.Optimism,
		RegolithTime: &zero,
		EcotoneTime:  &zero,
		FjordTime:    &zero,
		HoloceneTime: &zero,
		IsthmusTime:  &isthmusTime,
	}
	statedb := &testStateGetter{
		baseFee:             baseFee,
		overhead:            overhead,
		scalar:              scalar,
		blobBaseFee:         blobBaseFee,
		baseFeeScalar:       uint32(baseFeeScalar.Uint64()),
		blobBaseFeeScalar:   uint32(blobBaseFeeScalar.Uint64()),
		operatorFeeScalar:   uint32(operatorFeeScalar.Uint64()),
		operatorFeeConstant: operatorFeeConstant.Uint64(),
	}
	const gasLimit = 21_000
	tx := NewTransaction(0, common.Address{1}, big.NewInt(0), gasLimit, big.NewInt(0), []byte{1, 2, 3})

	costFn := NewTotalRollupCostFunc(config, statedb)
	require.NotNil(t, costFn)

	l1Cost := uint256.MustFromBig(NewL1CostFunc(config, statedb)(tx.RollupCostData(), isthmusTime-1))
	require.Equal(t, 1, l1Cost.Sign())
	require.Equal(t, l1Cost, costFn(tx, isthmusTime-1), "pre-Isthmus total rollup cost must only contain the L1 cost")

	// operatorFeeScalar * gasLimit / 1e6 + operatorFeeConstant
	opCost := new(big.Int).Mul(operatorFeeScalar, big.NewInt(gasLimit))
	opCost.Div(opCost, big.NewInt(1e6)).Add(opCost, operatorFeeConstant)
	postL1Cost := uint256.MustFromBig(NewL1CostFunc(config, statedb)(tx.RollupCostData(), isthmusTime))
	expected := new(uint256.Int).Add(postL1Cost, uint256.MustFromBig(opCost))
	require.Equal(t, expected, costFn(tx, isthmusTime), "Isthmus total rollup cost must contain the L1 and operator cost")

	// Deposit-like txs without rollup cost data still pay the operator cost based on gas.
	require.Equal(t, uint256.MustFromBig(opCost), costFn(noRollupCostDataTx{gas: gasLimit}, isthmusTime))
	require.True(t, costFn(noRollupCostDataTx{gas: gasLimit}, isthmusTime-1).IsZero())
}

func TestExtractFeeParams(t *testing.T) {
	var scalars common.Hash
	scalars[scalarSectionStart+3] = 7
	scalars[scalarSectionStart+7] = 9
	baseScalar, blobScalar := ExtractEcotoneFeeParams(scalars[:])
	require.EqualValues(t, 7, baseScalar.Uint64())
	require.EqualValues(t, 9, blobScalar.Uint64())

	opParams := common.Hash{23: 3, 31: 5}
	opScalar, opConst := ExtractOperatorFeeParams(opParams)
	require.EqualValues(t, 3, opScalar.Uint64())
	require.EqualValues(t, 5, opConst.Uint64())
}

type noRollupCostDataTx struct{ gas uint64 }

func (noRollupCostDataTx) RollupCostData() RollupCostData { return RollupCostData{} }
func (tx noRollupCostDataTx) Gas() uint64                 { return tx.gas }
