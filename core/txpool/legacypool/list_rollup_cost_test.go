package legacypool

import (
	"math/big"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

func setOperatorFeeConstant(pool *LegacyPool, opFeeConst uint64) {
	var opFeeParams common.Hash
	new(big.Int).SetUint64(opFeeConst).FillBytes(opFeeParams[24:32])
	pool.mu.Lock()
	pool.currentState.SetState(types.L1BlockAddr, types.OperatorFeeParamsSlot, opFeeParams)
	pool.mu.Unlock()
}

// TestOperatorFeeInsufficientFunds checks that a tx whose sender can pay for the
// regular tx cost, but not for the operator fee, is rejected by the pool.
func TestOperatorFeeInsufficientFunds(t *testing.T) {
	t.Parallel()

	pool, key := setupPoolWithConfig(params.OptimismTestConfig)
	defer pool.Close()

	const opFee = 1000
	setOperatorFeeConstant(pool, opFee)

	tx := transaction(0, 100_000, key)
	from, _ := deriveSender(tx)
	testAddBalance(pool, from, new(big.Int).Add(tx.Cost(), big.NewInt(opFee-1)))

	err := pool.addRemoteSync(tx)
	require.ErrorIs(t, err, core.ErrInsufficientFunds)
	require.ErrorContains(t, err, "overshot 1")

	testAddBalance(pool, from, big.NewInt(1))
	require.NoError(t, pool.addRemoteSync(tx))
	require.Equal(t, new(big.Int).Add(tx.Cost(), big.NewInt(opFee)), pool.pending[from].totalcost.ToBig())
}

// TestOperatorFeeIgnoredPreIsthmus checks that operator fee params don't influence
// the sufficient funds check before Isthmus is active.
func TestOperatorFeeIgnoredPreIsthmus(t *testing.T) {
	t.Parallel()

	config := *params.OptimismTestConfig
	config.IsthmusTime = nil
	pool, key := setupPoolWithConfig(&config)
	defer pool.Close()

	setOperatorFeeConstant(pool, 1000)

	tx := transaction(0, 100_000, key)
	from, _ := deriveSender(tx)
	testAddBalance(pool, from, tx.Cost())
	require.NoError(t, pool.addRemoteSync(tx))
}

// TestOperatorFeePendingBalanceDrop checks that a pending tx that can't cover its
// operator fee anymore gets dropped when the balance decreases.
func TestOperatorFeePendingBalanceDrop(t *testing.T) {
	t.Parallel()

	pool, key := setupPoolWithConfig(params.OptimismTestConfig)
	defer pool.Close()

	const opFee = 1000
	setOperatorFeeConstant(pool, opFee)

	tx := transaction(0, 100_000, key)
	from, _ := deriveSender(tx)
	cost, overflow := txpool.TotalTxCost(tx, pool.rollupCostFn)
	require.False(t, overflow)
	testAddBalance(pool, from, cost.ToBig())
	require.NoError(t, pool.addRemoteSync(tx))

	pending, _ := pool.Stats()
	require.Equal(t, 1, pending)

	// The balance still covers the regular tx cost, but not the operator fee anymore.
	pool.mu.Lock()
	pool.currentState.SubBalance(from, uint256.NewInt(1), tracing.BalanceChangeUnspecified)
	pool.mu.Unlock()
	<-pool.requestReset(nil, nil)

	pending, queued := pool.Stats()
	require.Zero(t, pending)
	require.Zero(t, queued)
}

type testRollupCostFnProvider struct{ fn txpool.RollupCostFunc }

func (p *testRollupCostFnProvider) RollupCostFunc() txpool.RollupCostFunc { return p.fn }

func constRollupCostFn(c uint64) txpool.RollupCostFunc {
	return func(types.RollupTransaction) *uint256.Int { return uint256.NewInt(c) }
}

// TestListRollupCostChange checks that the total cost accounting of a list stays
// consistent if the rollup cost function changes while txs are in the list.
func TestListRollupCostChange(t *testing.T) {
	key, _ := crypto.GenerateKey()
	tx0 := pricedTransaction(0, 100_000, big.NewInt(1), key)
	tx0Replacement := pricedTransaction(0, 100_000, big.NewInt(2), key)
	tx1 := pricedTransaction(1, 100_000, big.NewInt(1), key)

	prv := &testRollupCostFnProvider{fn: constRollupCostFn(10)}
	list := newRollupList(true, prv)

	inserted, _ := list.Add(tx0, DefaultConfig.PriceBump)
	require.True(t, inserted)
	require.Equal(t, new(big.Int).Add(tx0.Cost(), big.NewInt(10)), list.totalcost.ToBig())

	// Rollup cost increases, e.g. due to a new head with higher fee params.
	prv.fn = constRollupCostFn(20)
	inserted, _ = list.Add(tx1, DefaultConfig.PriceBump)
	require.True(t, inserted)
	expTotal := new(big.Int).Add(tx0.Cost(), tx1.Cost())
	require.Equal(t, expTotal.Add(expTotal, big.NewInt(30)), list.totalcost.ToBig())

	inserted, old := list.Add(tx0Replacement, DefaultConfig.PriceBump)
	require.True(t, inserted)
	require.Equal(t, tx0, old)
	expTotal = new(big.Int).Add(tx0Replacement.Cost(), tx1.Cost())
	require.Equal(t, expTotal.Add(expTotal, big.NewInt(40)), list.totalcost.ToBig())

	// Removing txs must subtract the costs they were added with, not the current ones.
	prv.fn = constRollupCostFn(50)
	removed, invalids := list.Remove(tx0Replacement)
	require.True(t, removed)
	require.Equal(t, types.Transactions{tx1}, invalids)
	require.True(t, list.totalcost.IsZero())
	require.Empty(t, list.txCosts)
}

// TestListFilterRollupCost checks that Filter takes rollup costs into account.
func TestListFilterRollupCost(t *testing.T) {
	key, _ := crypto.GenerateKey()
	tx0 := pricedTransaction(0, 100_000, big.NewInt(1), key)
	tx1 := pricedTransaction(1, 100_000, big.NewInt(1), key)

	list := newRollupList(true, &testRollupCostFnProvider{fn: constRollupCostFn(10)})
	list.Add(tx0, DefaultConfig.PriceBump)
	list.Add(tx1, DefaultConfig.PriceBump)

	balance := uint256.MustFromBig(new(big.Int).Add(tx0.Cost(), big.NewInt(9)))
	drops, invalids := list.Filter(balance, 100_000)
	require.Len(t, drops, 2)
	require.Empty(t, invalids)
	require.True(t, list.Empty())
	require.True(t, list.totalcost.IsZero())
}
