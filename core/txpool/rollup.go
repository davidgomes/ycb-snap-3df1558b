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

package txpool

import (
	"math/big"

	"github.com/holiman/uint256"

	"github.com/ethereum/go-ethereum/core/types"
)

// RollupCostFunc returns the total rollup cost (L1 data availability cost plus operator
// cost) of a transaction at the pool's current head, or nil if no rollup cost applies.
type RollupCostFunc func(tx types.RollupTransaction) *uint256.Int

// RollupTransaction is a transaction whose total cost, including rollup costs, can be computed.
type RollupTransaction interface {
	types.RollupTransaction
	Cost() *big.Int
}

// TotalTxCost returns the transaction's total cost, consisting of the regular execution cost
// (gas * gas price + value) and the rollup costs (L1 cost and operator cost). Only the regular
// cost applies if rollupCostFn is nil. The returned bool reports whether the total overflows
// 256 bits, in which case the returned cost must not be used.
func TotalTxCost(tx RollupTransaction, rollupCostFn RollupCostFunc) (*uint256.Int, bool) {
	cost, overflow := uint256.FromBig(tx.Cost())
	if overflow {
		return nil, true
	}
	if rollupCostFn == nil {
		return cost, false
	}
	rollupCost := rollupCostFn(tx)
	if rollupCost == nil {
		return cost, false
	}
	return cost.AddOverflow(cost, rollupCost)
}
