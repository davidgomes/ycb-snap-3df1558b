// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package eip1559

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"
)

// copyConfig does a _shallow_ copy of a given config. Safe to set new values, but
// do not use e.g. SetInt() on the numbers. For testing only
func copyConfig(original *params.ChainConfig) *params.ChainConfig {
	return &params.ChainConfig{
		ChainID:                 original.ChainID,
		HomesteadBlock:          original.HomesteadBlock,
		DAOForkBlock:            original.DAOForkBlock,
		DAOForkSupport:          original.DAOForkSupport,
		EIP150Block:             original.EIP150Block,
		EIP155Block:             original.EIP155Block,
		EIP158Block:             original.EIP158Block,
		ByzantiumBlock:          original.ByzantiumBlock,
		ConstantinopleBlock:     original.ConstantinopleBlock,
		PetersburgBlock:         original.PetersburgBlock,
		IstanbulBlock:           original.IstanbulBlock,
		MuirGlacierBlock:        original.MuirGlacierBlock,
		BerlinBlock:             original.BerlinBlock,
		LondonBlock:             original.LondonBlock,
		TerminalTotalDifficulty: original.TerminalTotalDifficulty,
		Ethash:                  original.Ethash,
		Clique:                  original.Clique,
	}
}

func config() *params.ChainConfig {
	config := copyConfig(params.TestChainConfig)
	config.LondonBlock = big.NewInt(5)
	return config
}

func opConfig() *params.ChainConfig {
	config := copyConfig(params.TestChainConfig)
	config.LondonBlock = big.NewInt(5)
	ct := uint64(10)
	eip1559DenominatorCanyon := uint64(250)
	config.CanyonTime = &ct
	ht := uint64(12)
	config.HoloceneTime = &ht
	config.Optimism = &params.OptimismConfig{
		EIP1559Elasticity:        6,
		EIP1559Denominator:       50,
		EIP1559DenominatorCanyon: &eip1559DenominatorCanyon,
	}
	return config
}

// jovianConfig extends opConfig with Jovian activated at time 14.
func jovianConfig() *params.ChainConfig {
	config := opConfig()
	jt := uint64(14)
	config.JovianTime = &jt
	return config
}

// TestBlockGasLimits tests the gasLimit checks for blocks both across
// the EIP-1559 boundary and post-1559 blocks
func TestBlockGasLimits(t *testing.T) {
	initial := new(big.Int).SetUint64(params.InitialBaseFee)

	for i, tc := range []struct {
		pGasLimit uint64
		pNum      int64
		gasLimit  uint64
		ok        bool
	}{
		// Transitions from non-london to london
		{10000000, 4, 20000000, true},  // No change
		{10000000, 4, 20019530, true},  // Upper limit
		{10000000, 4, 20019531, false}, // Upper +1
		{10000000, 4, 19980470, true},  // Lower limit
		{10000000, 4, 19980469, false}, // Lower limit -1
		// London to London
		{20000000, 5, 20000000, true},
		{20000000, 5, 20019530, true},  // Upper limit
		{20000000, 5, 20019531, false}, // Upper limit +1
		{20000000, 5, 19980470, true},  // Lower limit
		{20000000, 5, 19980469, false}, // Lower limit -1
		{40000000, 5, 40039061, true},  // Upper limit
		{40000000, 5, 40039062, false}, // Upper limit +1
		{40000000, 5, 39960939, true},  // lower limit
		{40000000, 5, 39960938, false}, // Lower limit -1
	} {
		parent := &types.Header{
			GasUsed:  tc.pGasLimit / 2,
			GasLimit: tc.pGasLimit,
			BaseFee:  initial,
			Number:   big.NewInt(tc.pNum),
		}
		header := &types.Header{
			GasUsed:  tc.gasLimit / 2,
			GasLimit: tc.gasLimit,
			BaseFee:  initial,
			Number:   big.NewInt(tc.pNum + 1),
		}
		err := VerifyEIP1559Header(config(), parent, header)
		if tc.ok && err != nil {
			t.Errorf("test %d: Expected valid header: %s", i, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("test %d: Expected invalid header", i)
		}
	}
}

// TestCalcBaseFee assumes all blocks are 1559-blocks
func TestCalcBaseFee(t *testing.T) {
	tests := []struct {
		parentBaseFee   int64
		parentGasLimit  uint64
		parentGasUsed   uint64
		expectedBaseFee int64
	}{
		{params.InitialBaseFee, 20000000, 10000000, params.InitialBaseFee}, // usage == target
		{params.InitialBaseFee, 20000000, 9000000, 987500000},              // usage below target
		{params.InitialBaseFee, 20000000, 11000000, 1012500000},            // usage above target
	}
	for i, test := range tests {
		parent := &types.Header{
			Number:   common.Big32,
			GasLimit: test.parentGasLimit,
			GasUsed:  test.parentGasUsed,
			BaseFee:  big.NewInt(test.parentBaseFee),
		}
		if have, want := CalcBaseFee(config(), parent, 0), big.NewInt(test.expectedBaseFee); have.Cmp(want) != 0 {
			t.Errorf("test %d: have %d  want %d, ", i, have, want)
		}
	}
}

// TestCalcBaseFeeOptimism assumes all blocks are 1559-blocks but tests the Canyon activation
func TestCalcBaseFeeOptimism(t *testing.T) {
	tests := []struct {
		parentBaseFee   int64
		parentGasLimit  uint64
		parentGasUsed   uint64
		expectedBaseFee int64
		postCanyon      bool
	}{
		{params.InitialBaseFee, 30_000_000, 5_000_000, params.InitialBaseFee, false}, // usage == target
		{params.InitialBaseFee, 30_000_000, 4_000_000, 996000000, false},             // usage below target
		{params.InitialBaseFee, 30_000_000, 10_000_000, 1020000000, false},           // usage above target
		{params.InitialBaseFee, 30_000_000, 5_000_000, params.InitialBaseFee, true},  // usage == target
		{params.InitialBaseFee, 30_000_000, 4_000_000, 999200000, true},              // usage below target
		{params.InitialBaseFee, 30_000_000, 10_000_000, 1004000000, true},            // usage above target
	}
	for i, test := range tests {
		parent := &types.Header{
			Number:   common.Big32,
			GasLimit: test.parentGasLimit,
			GasUsed:  test.parentGasUsed,
			BaseFee:  big.NewInt(test.parentBaseFee),
			Time:     6,
		}
		if test.postCanyon {
			parent.Time = 8
		}
		if have, want := CalcBaseFee(opConfig(), parent, parent.Time+2), big.NewInt(test.expectedBaseFee); have.Cmp(want) != 0 {
			t.Errorf("test %d: have %d  want %d, ", i, have, want)
		}
		if test.postCanyon {
			// make sure Holocene activation doesn't change the outcome; since these tests have empty eip1559 params,
			// they should be handled using the Canyon config.
			parent.Time = 10
			if have, want := CalcBaseFee(opConfig(), parent, parent.Time+2), big.NewInt(test.expectedBaseFee); have.Cmp(want) != 0 {
				t.Errorf("test %d: have %d  want %d, ", i, have, want)
			}
		}
	}
}

// TestCalcBaseFeeOptimismHolocene assumes all blocks are Optimism blocks post-Holocene upgrade
func TestCalcBaseFeeOptimismHolocene(t *testing.T) {
	parentBaseFee := int64(10_000_000)
	parentGasLimit := uint64(30_000_000)

	tests := []struct {
		parentGasUsed     uint64
		expectedBaseFee   int64
		denom, elasticity uint64
	}{
		{parentGasLimit / 2, parentBaseFee, 10, 2},  // target
		{10_000_000, 9_666_667, 10, 2},              // below
		{20_000_000, 10_333_333, 10, 2},             // above
		{parentGasLimit / 10, parentBaseFee, 2, 10}, // target
		{1_000_000, 6_666_667, 2, 10},               // below
		{30_000_000, 55_000_000, 2, 10},             // above
	}
	for i, test := range tests {
		parent := &types.Header{
			Number:   common.Big32,
			GasLimit: parentGasLimit,
			GasUsed:  test.parentGasUsed,
			BaseFee:  big.NewInt(parentBaseFee),
			Time:     12,
			Extra:    EncodeHoloceneExtraData(test.denom, test.elasticity),
		}
		if have, want := CalcBaseFee(opConfig(), parent, parent.Time+2), big.NewInt(test.expectedBaseFee); have.Cmp(want) != 0 {
			t.Errorf("test %d: have %d  want %d, ", i, have, want)
		}
	}
}

// TestCalcBaseFeeJovian tests that the minimum base fee from the parent header is enforced once
// Jovian is active at the parent's timestamp, and that it is not enforced before.
func TestCalcBaseFeeJovian(t *testing.T) {
	const (
		parentGasLimit  = uint64(30_000_000)
		denom           = uint64(50)
		elasticity      = uint64(3)
		parentGasTarget = parentGasLimit / elasticity
		preJovian       = uint64(12) // Holocene active
		postJovian      = uint64(14)
		minBaseFee      = uint64(1e9)
	)

	tests := []struct {
		name            string
		parentBaseFee   uint64
		parentGasUsed   uint64
		parentTime      uint64
		parentExtra     []byte
		expectedBaseFee uint64
	}{
		{
			name:            "pre-Jovian parent is not clamped",
			parentBaseFee:   1,
			parentGasUsed:   parentGasTarget - 1_000_000,
			parentTime:      preJovian,
			parentExtra:     EncodeHoloceneExtraData(denom, elasticity),
			expectedBaseFee: 1,
		},
		{
			name:            "gas used at target is clamped",
			parentBaseFee:   1,
			parentGasUsed:   parentGasTarget,
			parentTime:      postJovian,
			parentExtra:     EncodeMinBaseFeeExtraData(denom, elasticity, minBaseFee),
			expectedBaseFee: minBaseFee,
		},
		{
			name:            "gas used above target is clamped",
			parentBaseFee:   1,
			parentGasUsed:   parentGasTarget + 1_000_000,
			parentTime:      postJovian,
			parentExtra:     EncodeMinBaseFeeExtraData(denom, elasticity, minBaseFee),
			expectedBaseFee: minBaseFee,
		},
		{
			// 2e9 + 2e9 * 10_000_000 / 10_000_000 / 50 = 2_040_000_000
			name:            "gas used above target is not clamped when above minimum",
			parentBaseFee:   2e9,
			parentGasUsed:   parentGasTarget + 10_000_000,
			parentTime:      postJovian,
			parentExtra:     EncodeMinBaseFeeExtraData(denom, elasticity, minBaseFee),
			expectedBaseFee: 2_040_000_000,
		},
		{
			// 1e9 - 1e9 * 1_000_000 / 10_000_000 / 50 = 998_000_000
			name:            "gas used below target is clamped",
			parentBaseFee:   1e9,
			parentGasUsed:   parentGasTarget - 1_000_000,
			parentTime:      postJovian,
			parentExtra:     EncodeMinBaseFeeExtraData(denom, elasticity, minBaseFee),
			expectedBaseFee: minBaseFee,
		},
		{
			// 2e9 - 2e9 * 1_000_000 / 10_000_000 / 50 = 1_996_000_000
			name:            "gas used below target is not clamped when above minimum",
			parentBaseFee:   2e9,
			parentGasUsed:   parentGasTarget - 1_000_000,
			parentTime:      postJovian,
			parentExtra:     EncodeMinBaseFeeExtraData(denom, elasticity, minBaseFee),
			expectedBaseFee: 1_996_000_000,
		},
		{
			name:            "zero minimum is not enforced",
			parentBaseFee:   1e9,
			parentGasUsed:   parentGasTarget - 1_000_000,
			parentTime:      postJovian,
			parentExtra:     EncodeMinBaseFeeExtraData(denom, elasticity, 0),
			expectedBaseFee: 998_000_000,
		},
		{
			name:            "post-Jovian parent with Holocene extraData is not clamped",
			parentBaseFee:   1,
			parentGasUsed:   parentGasTarget - 1_000_000,
			parentTime:      postJovian,
			parentExtra:     EncodeHoloceneExtraData(denom, elasticity),
			expectedBaseFee: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parent := &types.Header{
				Number:   common.Big32,
				GasLimit: parentGasLimit,
				GasUsed:  test.parentGasUsed,
				BaseFee:  new(big.Int).SetUint64(test.parentBaseFee),
				Time:     test.parentTime,
				Extra:    test.parentExtra,
			}
			have := CalcBaseFee(jovianConfig(), parent, parent.Time+2)
			require.Equal(t, new(big.Int).SetUint64(test.expectedBaseFee), have)
		})
	}
}

func TestMinBaseFeeExtraData(t *testing.T) {
	extra := EncodeMinBaseFeeExtraData(250, 6, 1e9)
	// version | denominator | elasticity | minBaseFee
	require.Equal(t, hexutil.MustDecode("0x01"+"000000fa"+"00000006"+"000000003b9aca00"), extra)
	require.NoError(t, ValidateMinBaseFeeExtraData(extra))

	d, e, m := DecodeMinBaseFeeExtraData(extra)
	require.Equal(t, uint64(250), d)
	require.Equal(t, uint64(6), e)
	require.NotNil(t, m)
	require.Equal(t, uint64(1e9), *m)

	// The Holocene format is decoded without a minimum base fee.
	d, e, m = DecodeMinBaseFeeExtraData(EncodeHoloceneExtraData(250, 6))
	require.Equal(t, uint64(250), d)
	require.Equal(t, uint64(6), e)
	require.Nil(t, m)

	d, e, m = DecodeMinBaseFeeExtraData(extra[:16])
	require.Zero(t, d)
	require.Zero(t, e)
	require.Nil(t, m)

	require.Panics(t, func() { EncodeMinBaseFeeExtraData(1<<32, 6, 0) })
	require.Panics(t, func() { EncodeMinBaseFeeExtraData(250, 1<<32, 0) })
}

func TestValidateMinBaseFeeExtraData(t *testing.T) {
	valid := EncodeMinBaseFeeExtraData(250, 6, 1e9)
	wrongVersion := append([]byte{HoloceneExtraDataVersionByte}, valid[1:]...)
	zeroDenominator := EncodeMinBaseFeeExtraData(0, 6, 1e9)

	for _, test := range []struct {
		extra    []byte
		expected string
	}{
		{valid, ""},
		{EncodeMinBaseFeeExtraData(0, 0, 0), ""},
		{nil, "MinBaseFee extraData should be 17 bytes, got 0"},
		{EncodeHoloceneExtraData(250, 6), "MinBaseFee extraData should be 17 bytes, got 9"},
		{append(valid, 0), "MinBaseFee extraData should be 17 bytes, got 18"},
		{wrongVersion, "MinBaseFee extraData should have 1 version byte, got 0"},
		{zeroDenominator, "holocene params cannot have a 0 denominator unless elasticity is also 0"},
	} {
		t.Run(fmt.Sprintf("%x", test.extra), func(t *testing.T) {
			err := ValidateMinBaseFeeExtraData(test.extra)
			if test.expected == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, test.expected)
			}
		})
	}
}

func TestOptimismExtraData(t *testing.T) {
	config := jovianConfig()
	minBaseFee := uint64(1e9)
	holoceneExtra := EncodeHoloceneExtraData(250, 6)
	jovianExtra := EncodeMinBaseFeeExtraData(250, 6, minBaseFee)

	for _, test := range []struct {
		name          string
		time          uint64
		extra         []byte
		minBaseFee    *uint64
		expectedError string
	}{
		{name: "pre-Holocene", time: 10, extra: nil},
		{name: "Holocene", time: 12, extra: holoceneExtra},
		{name: "Jovian", time: 14, extra: jovianExtra, minBaseFee: &minBaseFee},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded := EncodeOptimismExtraData(config, test.time, 250, 6, test.minBaseFee)
			require.Equal(t, test.extra, encoded)
			require.NoError(t, ValidateOptimismExtraData(config, test.time, encoded))

			d, e, m := DecodeOptimismExtraData(config, test.time, encoded)
			require.Equal(t, test.minBaseFee, m)
			if test.extra == nil {
				require.Zero(t, d)
				require.Zero(t, e)
			} else {
				require.Equal(t, uint64(250), d)
				require.Equal(t, uint64(6), e)
			}
		})
	}

	require.EqualError(t, ValidateOptimismExtraData(config, 10, holoceneExtra), "extraData must be empty before Holocene")
	require.EqualError(t, ValidateOptimismExtraData(config, 12, jovianExtra), "holocene extraData should be 9 bytes, got 17")
	require.EqualError(t, ValidateOptimismExtraData(config, 14, holoceneExtra), "MinBaseFee extraData should be 17 bytes, got 9")
	require.Panics(t, func() { EncodeOptimismExtraData(config, 14, 250, 6, nil) })
}
