// Copyright 2025 The op-geth Authors
// This file is part of the op-geth library.
//
// The op-geth library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The op-geth library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the op-geth library. If not, see <http://www.gnu.org/licenses/>.

package legacypool

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

// testSetL1FeeParams sets the Ecotone L1 fee parameters of the L1Block contract.
func testSetL1FeeParams(t *testing.T, pool *LegacyPool, l1BaseFee *big.Int, baseFeeScalar uint32) {
	var scalars common.Hash
	offset := 32 - types.BaseFeeScalarSlotOffset - 4
	binary.BigEndian.PutUint32(scalars[offset:offset+4], baseFeeScalar)
	s, blobS := types.ExtractEcotoneFeeParams(scalars[:])
	require.EqualValues(t, baseFeeScalar, s.Uint64())
	require.Zero(t, blobS.Sign())

	pool.mu.Lock()
	defer pool.mu.Unlock()
	pool.currentState.SetState(types.L1BlockAddr, types.L1FeeScalarsSlot, scalars)
	pool.currentState.SetState(types.L1BlockAddr, types.L1BaseFeeSlot, common.BigToHash(l1BaseFee))
}

// testSetOperatorFeeParams sets the Isthmus operator fee parameters of the L1Block contract.
func testSetOperatorFeeParams(t *testing.T, pool *LegacyPool, scalar uint32, constant uint64) {
	var opFeeParams common.Hash
	binary.BigEndian.PutUint32(opFeeParams[20:24], scalar)
	binary.BigEndian.PutUint64(opFeeParams[24:32], constant)
	s, c := types.ExtractOperatorFeeParams(opFeeParams)
	require.EqualValues(t, scalar, s.Uint64())
	require.EqualValues(t, constant, c.Uint64())

	pool.mu.Lock()
	defer pool.mu.Unlock()
	pool.currentState.SetState(types.L1BlockAddr, types.OperatorFeeParamsSlot, opFeeParams)
}

func testRollupCost(pool *LegacyPool, tx *types.Transaction) *uint256.Int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	return pool.rollupCostFn(tx)
}

func testTotalTxCost(t *testing.T, pool *LegacyPool, tx *types.Transaction) *uint256.Int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	cost, overflow := txpool.TotalTxCost(tx, pool.rollupCostFn)
	require.False(t, overflow)
	return cost
}

// testPendingTotalCost returns the pending total cost of addr, or nil if it has no pending txs.
func testPendingTotalCost(pool *LegacyPool, addr common.Address) *uint256.Int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if list := pool.pending[addr]; list != nil {
		return new(uint256.Int).Set(list.totalcost)
	}
	return nil
}

// TestRollupCostSufficientFunds tests that the pool only accepts transactions whose
// sender can pay for the value, the gas and all applicable rollup costs.
func TestRollupCostSufficientFunds(t *testing.T) {
	preIsthmus := *params.OptimismTestConfig
	preIsthmus.IsthmusTime = nil

	tests := []struct {
		name            string
		config          *params.ChainConfig
		l1Cost, opFee   bool
		wantRollupCost0 bool // whether the rollup cost is zero
	}{
		{name: "no-rollup-cost", config: params.OptimismTestConfig, wantRollupCost0: true},
		{name: "l1-cost", config: params.OptimismTestConfig, l1Cost: true},
		{name: "operator-fee", config: params.OptimismTestConfig, opFee: true},
		{name: "l1-cost-and-operator-fee", config: params.OptimismTestConfig, l1Cost: true, opFee: true},
		{name: "pre-isthmus-operator-fee", config: &preIsthmus, opFee: true, wantRollupCost0: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool, key := setupPoolWithConfig(tt.config)
			defer pool.Close()
			require.NotNil(t, pool.rollupCostFn)

			if tt.l1Cost {
				testSetL1FeeParams(t, pool, big.NewInt(1e6), 1)
			}
			if tt.opFee {
				testSetOperatorFeeParams(t, pool, 1_000_000, 7) // 1 wei per gas + 7 wei
			}
			tx := transaction(0, 100_000, key)
			from, _ := deriveSender(tx)

			rollupCost := testRollupCost(pool, tx)
			require.NotNil(t, rollupCost)
			if tt.wantRollupCost0 {
				require.True(t, rollupCost.IsZero(), "rollup cost must be zero")
			} else {
				require.False(t, rollupCost.IsZero(), "rollup cost must be non-zero")
			}
			if tt.opFee && !tt.wantRollupCost0 {
				require.GreaterOrEqual(t, rollupCost.Uint64(), tx.Gas()+7, "rollup cost must contain the operator fee")
			}

			// Funds covering only the regular cost are insufficient if rollup costs apply
			testAddBalance(pool, from, tx.Cost())
			if !tt.wantRollupCost0 {
				require.ErrorIs(t, pool.addRemoteSync(tx), core.ErrInsufficientFunds)
				// The rollup costs are all that's missing
				testAddBalance(pool, from, rollupCost.ToBig())
			}
			require.NoError(t, pool.addRemoteSync(tx))
			require.Equal(t, testTotalTxCost(t, pool, tx), testPendingTotalCost(pool, from))
		})
	}
}

// TestRollupCostPendingAccounting tests that the pending total cost includes rollup
// costs, also across replacements.
func TestRollupCostPendingAccounting(t *testing.T) {
	t.Parallel()

	pool, key := setupPoolWithConfig(params.OptimismTestConfig)
	defer pool.Close()
	testSetL1FeeParams(t, pool, big.NewInt(1e6), 1)
	testSetOperatorFeeParams(t, pool, 0, 42)

	tx0 := pricedTransaction(0, 100_000, big.NewInt(100), key)
	tx0Replacement := pricedTransaction(0, 100_000, big.NewInt(110), key)
	tx1 := pricedTransaction(1, 100_000, big.NewInt(100), key)
	from, _ := deriveSender(tx0)
	testAddBalance(pool, from, big.NewInt(params.Ether))

	cost0 := testTotalTxCost(t, pool, tx0)
	require.Greater(t, cost0.Uint64(), tx0.Cost().Uint64())

	require.NoError(t, pool.addRemoteSync(tx0))
	require.Equal(t, cost0, testPendingTotalCost(pool, from))

	require.NoError(t, pool.addRemoteSync(tx1))
	want := new(uint256.Int).Add(cost0, testTotalTxCost(t, pool, tx1))
	require.Equal(t, want, testPendingTotalCost(pool, from))

	require.NoError(t, pool.addRemoteSync(tx0Replacement))
	want = new(uint256.Int).Add(testTotalTxCost(t, pool, tx0Replacement), testTotalTxCost(t, pool, tx1))
	require.Equal(t, want, testPendingTotalCost(pool, from))
}

// TestRollupCostIncreaseAfterInsertion tests that the pool's cost accounting stays
// consistent if rollup costs rise after transactions were added.
func TestRollupCostIncreaseAfterInsertion(t *testing.T) {
	t.Parallel()

	pool, key := setupPoolWithConfig(params.OptimismTestConfig)
	defer pool.Close()
	testSetOperatorFeeParams(t, pool, 0, 10)

	tx0 := transaction(0, 100_000, key)
	tx1 := transaction(1, 100_000, key)
	from, _ := deriveSender(tx0)
	testAddBalance(pool, from, big.NewInt(params.Ether))
	require.NoError(t, pool.addRemoteSync(tx0))
	require.NoError(t, pool.addRemoteSync(tx1))
	cost1 := testTotalTxCost(t, pool, tx1)

	// New head: operator fee rises, tx0 got included
	testSetOperatorFeeParams(t, pool, 0, 1000)
	testSetNonce(pool, from, 1)
	<-pool.requestReset(nil, nil)
	require.Greater(t, testTotalTxCost(t, pool, tx1).Uint64(), cost1.Uint64())

	require.Nil(t, pool.Get(tx0.Hash()))
	require.NotNil(t, pool.Get(tx1.Hash()))
	require.Equal(t, cost1, testPendingTotalCost(pool, from), "pending cost must be the cost tx1 was accounted with")

	// New head: tx1 got included
	testSetNonce(pool, from, 2)
	<-pool.requestReset(nil, nil)
	require.Nil(t, pool.Get(tx1.Hash()))
	require.Nil(t, testPendingTotalCost(pool, from))
	require.NoError(t, validatePoolInternals(pool))
}

// TestRollupCostIncreaseDropsUnaffordable tests that pending transactions whose sender
// can't pay for them anymore after a rollup cost increase are dropped.
func TestRollupCostIncreaseDropsUnaffordable(t *testing.T) {
	t.Parallel()

	pool, key := setupPoolWithConfig(params.OptimismTestConfig)
	defer pool.Close()
	testSetOperatorFeeParams(t, pool, 0, 10)

	tx := transaction(0, 100_000, key)
	from, _ := deriveSender(tx)
	testAddBalance(pool, from, testTotalTxCost(t, pool, tx).ToBig())
	require.NoError(t, pool.addRemoteSync(tx))

	// New head without fee changes: tx stays affordable
	<-pool.requestReset(nil, nil)
	require.NotNil(t, pool.Get(tx.Hash()))

	// New head with operator fee raised by 1 wei
	testSetOperatorFeeParams(t, pool, 0, 11)
	<-pool.requestReset(nil, nil)
	require.Nil(t, pool.Get(tx.Hash()))
	require.Nil(t, testPendingTotalCost(pool, from))
	require.NoError(t, validatePoolInternals(pool))
}

type testRollupCostProvider struct {
	fn txpool.RollupCostFunc
}

func (p *testRollupCostProvider) rollupCostFunc() txpool.RollupCostFunc { return p.fn }

func fixedRollupCost(cost *uint256.Int) txpool.RollupCostFunc {
	return func(types.RollupTransaction) *uint256.Int { return new(uint256.Int).Set(cost) }
}

func txCostPlus(tx *types.Transaction, rollupCost uint64) *uint256.Int {
	return new(uint256.Int).AddUint64(uint256.MustFromBig(tx.Cost()), rollupCost)
}

// TestRollupListCostChange tests that a list subtracts transaction costs as they were
// accounted when added, independent of the current rollup cost.
func TestRollupListCostChange(t *testing.T) {
	key, _ := crypto.GenerateKey()
	prv := &testRollupCostProvider{fn: fixedRollupCost(uint256.NewInt(5))}
	l := newRollupList(true, prv)

	tx0, tx1, tx2 := transaction(0, 1000, key), transaction(1, 1000, key), transaction(2, 1000, key)
	for _, tx := range []*types.Transaction{tx0, tx1, tx2} {
		inserted, _ := l.Add(tx, DefaultConfig.PriceBump)
		require.True(t, inserted)
	}
	want := new(uint256.Int).Add(txCostPlus(tx0, 5), txCostPlus(tx1, 5))
	want.Add(want, txCostPlus(tx2, 5))
	require.Equal(t, want, l.totalcost)

	prv.fn = fixedRollupCost(uint256.NewInt(500))
	require.Len(t, l.Forward(1), 1)
	require.Equal(t, new(uint256.Int).Add(txCostPlus(tx1, 5), txCostPlus(tx2, 5)), l.totalcost)

	// A replacement is accounted at the current rollup cost
	tx1Replacement := pricedTransaction(1, 1000, big.NewInt(2), key)
	inserted, old := l.Add(tx1Replacement, DefaultConfig.PriceBump)
	require.True(t, inserted)
	require.Equal(t, tx1, old)
	require.Equal(t, new(uint256.Int).Add(txCostPlus(tx1Replacement, 500), txCostPlus(tx2, 5)), l.totalcost)

	removed, invalids := l.Remove(tx1Replacement)
	require.True(t, removed)
	require.Equal(t, types.Transactions{tx2}, invalids)
	require.True(t, l.totalcost.IsZero())
	require.Empty(t, l.txCosts)
}

// TestRollupListFilter tests that Filter checks transactions against their current
// rollup costs, including costs overflowing 256 bits.
func TestRollupListFilter(t *testing.T) {
	key, _ := crypto.GenerateKey()
	prv := &testRollupCostProvider{fn: fixedRollupCost(uint256.NewInt(5))}
	l := newRollupList(false, prv)

	tx0, tx1 := transaction(0, 1000, key), transaction(1, 1000, key)
	l.Add(tx0, DefaultConfig.PriceBump)
	l.Add(tx1, DefaultConfig.PriceBump)
	balance := txCostPlus(tx0, 5)

	drops, _ := l.Filter(balance, 1000)
	require.Empty(t, drops)

	prv.fn = fixedRollupCost(uint256.NewInt(6))
	drops, _ = l.Filter(balance, 1000)
	require.ElementsMatch(t, types.Transactions{tx0, tx1}, drops)
	require.True(t, l.totalcost.IsZero())

	prv.fn = fixedRollupCost(uint256.NewInt(5))
	inserted, _ := l.Add(tx0, DefaultConfig.PriceBump)
	require.True(t, inserted)

	maxCost := new(uint256.Int).SetAllOne()
	prv.fn = fixedRollupCost(maxCost)
	inserted, _ = l.Add(tx1, DefaultConfig.PriceBump)
	require.False(t, inserted, "tx with overflowing total cost must be rejected")

	drops, _ = l.Filter(maxCost, 1000)
	require.Equal(t, types.Transactions{tx0}, drops, "tx with overflowing total cost must be dropped")
	require.True(t, l.totalcost.IsZero())
	require.Empty(t, l.txCosts)
}
