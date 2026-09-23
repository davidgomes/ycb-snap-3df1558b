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
	"encoding/binary"
	"errors"
	"fmt"
	gomath "math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// VerifyEIP1559Header verifies some header attributes which were changed in EIP-1559,
// - gas limit check
// - basefee check
func VerifyEIP1559Header(config *params.ChainConfig, parent, header *types.Header) error {
	// Verify that the gas limit remains within allowed bounds
	parentGasLimit := parent.GasLimit
	if !config.IsLondon(parent.Number) {
		parentGasLimit = parent.GasLimit * config.ElasticityMultiplier()
	}
	if config.Optimism == nil { // gasLimit can adjust instantly in optimism
		if err := misc.VerifyGaslimit(parentGasLimit, header.GasLimit); err != nil {
			return err
		}
	}
	// Verify the header is not malformed
	if header.BaseFee == nil {
		return errors.New("header is missing baseFee")
	}
	// Verify the baseFee is correct based on the parent header.
	expectedBaseFee := CalcBaseFee(config, parent, header.Time)
	if header.BaseFee.Cmp(expectedBaseFee) != 0 {
		return fmt.Errorf("invalid baseFee: have %s, want %s, parentBaseFee %s, parentGasUsed %d",
			header.BaseFee, expectedBaseFee, parent.BaseFee, parent.GasUsed)
	}
	return nil
}

const (
	// HoloceneExtraDataVersionByte is the version byte of the 9-byte Holocene header extraData format.
	HoloceneExtraDataVersionByte = uint8(0x00)
	// MinBaseFeeExtraDataVersionByte is the version byte of the 17-byte Jovian header extraData format,
	// which extends the Holocene format with a minimum base fee.
	MinBaseFeeExtraDataVersionByte = uint8(0x01)
)

// ForkChecker reports which upgrades that affect the header extraData format are active.
// It is implemented by *params.ChainConfig.
type ForkChecker interface {
	IsHolocene(time uint64) bool
	IsMinBaseFee(time uint64) bool
}

// ValidateOptimismExtraData checks that the header extraData of a block with the given timestamp
// is well-formed for the upgrades active at that time.
func ValidateOptimismExtraData(fc ForkChecker, time uint64, extraData []byte) error {
	if fc.IsMinBaseFee(time) {
		return ValidateMinBaseFeeExtraData(extraData)
	} else if fc.IsHolocene(time) {
		return ValidateHoloceneExtraData(extraData)
	} else if len(extraData) > 0 {
		return errors.New("extraData must be empty before Holocene")
	}
	return nil
}

// DecodeOptimismExtraData decodes the EIP-1559 denominator and elasticity, and, once Jovian is active,
// the minimum base fee, from the header extraData of a block with the given timestamp.
//
// The extraData is expected to have been validated with ValidateOptimismExtraData. Returns 0,0,nil
// before Holocene or if the format is invalid.
func DecodeOptimismExtraData(fc ForkChecker, time uint64, extraData []byte) (uint64, uint64, *uint64) {
	if fc.IsMinBaseFee(time) {
		return DecodeMinBaseFeeExtraData(extraData)
	} else if fc.IsHolocene(time) {
		denominator, elasticity := DecodeHoloceneExtraData(extraData)
		return denominator, elasticity, nil
	}
	return 0, 0, nil
}

// EncodeOptimismExtraData encodes the EIP-1559 parameters, and, once Jovian is active, the minimum base
// fee, into the header extraData format required at the given timestamp. Returns nil before Holocene.
// Will panic if minBaseFee is nil while Jovian is active, or if the EIP-1559 parameters are outside
// uint32 range.
func EncodeOptimismExtraData(fc ForkChecker, time uint64, denominator, elasticity uint64, minBaseFee *uint64) []byte {
	if fc.IsMinBaseFee(time) {
		if minBaseFee == nil {
			panic("minBaseFee must be set once Jovian is active")
		}
		return EncodeMinBaseFeeExtraData(denominator, elasticity, *minBaseFee)
	} else if fc.IsHolocene(time) {
		return EncodeHoloceneExtraData(denominator, elasticity)
	}
	return nil
}

// DecodeHolocene1559Params extracts the Holcene 1599 parameters from the encoded form defined here:
// https://github.com/ethereum-optimism/specs/blob/main/specs/protocol/holocene/exec-engine.md#eip-1559-parameters-in-payloadattributesv3
//
// Returns 0,0 if the format is invalid, though ValidateHolocene1559Params should be used instead of this function for
// validity checking.
func DecodeHolocene1559Params(params []byte) (uint64, uint64) {
	if len(params) != 8 {
		return 0, 0
	}
	denominator := binary.BigEndian.Uint32(params[:4])
	elasticity := binary.BigEndian.Uint32(params[4:])
	return uint64(denominator), uint64(elasticity)
}

// DecodeHoloceneExtraData decodes the Holocene 1559 parameters from the encoded form defined here:
// https://github.com/ethereum-optimism/specs/blob/main/specs/protocol/holocene/exec-engine.md#eip-1559-parameters-in-block-header
//
// Returns 0,0 if the format is invalid, though ValidateHoloceneExtraData should be used instead of this function for
// validity checking.
func DecodeHoloceneExtraData(extra []byte) (uint64, uint64) {
	if len(extra) != 9 {
		return 0, 0
	}
	return DecodeHolocene1559Params(extra[1:])
}

// EncodeHolocene1559Params encodes the eip-1559 parameters into 'PayloadAttributes.EIP1559Params' format. Will panic if
// either value is outside uint32 range.
func EncodeHolocene1559Params(denom, elasticity uint64) []byte {
	r := make([]byte, 8)
	if denom > gomath.MaxUint32 || elasticity > gomath.MaxUint32 {
		panic("eip-1559 parameters out of uint32 range")
	}
	binary.BigEndian.PutUint32(r[:4], uint32(denom))
	binary.BigEndian.PutUint32(r[4:], uint32(elasticity))
	return r
}

// EncodeHoloceneExtraData encodes the eip-1559 parameters into the header 'ExtraData' format. Will panic if either
// value is outside uint32 range.
func EncodeHoloceneExtraData(denom, elasticity uint64) []byte {
	r := make([]byte, 9)
	if denom > gomath.MaxUint32 || elasticity > gomath.MaxUint32 {
		panic("eip-1559 parameters out of uint32 range")
	}
	r[0] = HoloceneExtraDataVersionByte
	binary.BigEndian.PutUint32(r[1:5], uint32(denom))
	binary.BigEndian.PutUint32(r[5:], uint32(elasticity))
	return r
}

// DecodeMinBaseFeeExtraData decodes the EIP-1559 parameters and the minimum base fee from the Jovian header
// 'ExtraData' format defined here:
// https://github.com/ethereum-optimism/specs/blob/main/specs/protocol/jovian/exec-engine.md#minimum-base-fee-in-block-header
//
// A 9-byte Holocene extraData is decoded as well, returning a nil minimum base fee, since the parent of the
// first Jovian block, or a Jovian genesis block, may still carry the Holocene format. Returns 0,0,nil if the
// format is invalid, though ValidateMinBaseFeeExtraData should be used instead of this function for validity
// checking.
func DecodeMinBaseFeeExtraData(extra []byte) (uint64, uint64, *uint64) {
	switch len(extra) {
	case 9:
		denominator, elasticity := DecodeHolocene1559Params(extra[1:])
		return denominator, elasticity, nil
	case 17:
		denominator, elasticity := DecodeHolocene1559Params(extra[1:9])
		minBaseFee := binary.BigEndian.Uint64(extra[9:])
		return denominator, elasticity, &minBaseFee
	default:
		return 0, 0, nil
	}
}

// EncodeMinBaseFeeExtraData encodes the EIP-1559 parameters and the minimum base fee into the Jovian header
// 'ExtraData' format. Will panic if either EIP-1559 parameter is outside uint32 range.
func EncodeMinBaseFeeExtraData(denom, elasticity, minBaseFee uint64) []byte {
	r := make([]byte, 17)
	if denom > gomath.MaxUint32 || elasticity > gomath.MaxUint32 {
		panic("eip-1559 parameters out of uint32 range")
	}
	r[0] = MinBaseFeeExtraDataVersionByte
	binary.BigEndian.PutUint32(r[1:5], uint32(denom))
	binary.BigEndian.PutUint32(r[5:9], uint32(elasticity))
	binary.BigEndian.PutUint64(r[9:], minBaseFee)
	return r
}

// ValidateHolocene1559Params checks if the encoded parameters are valid according to the Holocene
// upgrade.
func ValidateHolocene1559Params(params []byte) error {
	if len(params) != 8 {
		return fmt.Errorf("holocene eip-1559 params should be 8 bytes, got %d", len(params))
	}
	d, e := DecodeHolocene1559Params(params)
	if e != 0 && d == 0 {
		return errors.New("holocene params cannot have a 0 denominator unless elasticity is also 0")
	}
	return nil
}

// ValidateHoloceneExtraData checks if the header extraData is valid according to the Holocene
// upgrade.
func ValidateHoloceneExtraData(extra []byte) error {
	if len(extra) != 9 {
		return fmt.Errorf("holocene extraData should be 9 bytes, got %d", len(extra))
	}
	if extra[0] != HoloceneExtraDataVersionByte {
		return fmt.Errorf("holocene extraData should have %d version byte, got %d", HoloceneExtraDataVersionByte, extra[0])
	}
	return ValidateHolocene1559Params(extra[1:])
}

// ValidateMinBaseFeeExtraData checks if the header extraData is valid according to the Jovian upgrade,
// which requires the minimum base fee to be encoded after the Holocene EIP-1559 parameters.
func ValidateMinBaseFeeExtraData(extra []byte) error {
	if len(extra) != 17 {
		return fmt.Errorf("MinBaseFee extraData should be 17 bytes, got %d", len(extra))
	}
	if extra[0] != MinBaseFeeExtraDataVersionByte {
		return fmt.Errorf("MinBaseFee extraData should have %d version byte, got %d", MinBaseFeeExtraDataVersionByte, extra[0])
	}
	return ValidateHolocene1559Params(extra[1:9])
}

// CalcBaseFee calculates the basefee of the header.
// The time belongs to the new block to check which upgrades are active.
func CalcBaseFee(config *params.ChainConfig, parent *types.Header, time uint64) *big.Int {
	// If the current block is the first EIP-1559 block, return the InitialBaseFee.
	if !config.IsLondon(parent.Number) {
		return new(big.Int).SetUint64(params.InitialBaseFee)
	}
	elasticity := config.ElasticityMultiplier()
	denominator := config.BaseFeeChangeDenominator(time)
	var minBaseFee *uint64
	if config.IsHolocene(parent.Time) {
		denominator, elasticity, minBaseFee = DecodeOptimismExtraData(config, parent.Time, parent.Extra)
		if denominator == 0 {
			// this shouldn't happen as the ExtraData should have been validated prior
			panic("invalid eip-1559 params in extradata")
		}
	}
	baseFee := calcBaseFee(parent, denominator, elasticity)
	if minBaseFee != nil {
		if floor := new(big.Int).SetUint64(*minBaseFee); baseFee.Cmp(floor) < 0 {
			return floor
		}
	}
	return baseFee
}

// calcBaseFee applies the EIP-1559 base fee update rule to the parent header, using the given
// base fee change denominator and elasticity multiplier.
func calcBaseFee(parent *types.Header, denominator, elasticity uint64) *big.Int {
	parentGasTarget := parent.GasLimit / elasticity
	// If the parent gasUsed is the same as the target, the baseFee remains unchanged.
	if parent.GasUsed == parentGasTarget {
		return new(big.Int).Set(parent.BaseFee)
	}

	var (
		num   = new(big.Int)
		denom = new(big.Int)
	)

	if parent.GasUsed > parentGasTarget {
		// If the parent block used more gas than its target, the baseFee should increase.
		// max(1, parentBaseFee * gasUsedDelta / parentGasTarget / baseFeeChangeDenominator)
		num.SetUint64(parent.GasUsed - parentGasTarget)
		num.Mul(num, parent.BaseFee)
		num.Div(num, denom.SetUint64(parentGasTarget))
		num.Div(num, denom.SetUint64(denominator))
		if num.Cmp(common.Big1) < 0 {
			return num.Add(parent.BaseFee, common.Big1)
		}
		return num.Add(parent.BaseFee, num)
	} else {
		// Otherwise if the parent block used less gas than its target, the baseFee should decrease.
		// max(0, parentBaseFee * gasUsedDelta / parentGasTarget / baseFeeChangeDenominator)
		num.SetUint64(parentGasTarget - parent.GasUsed)
		num.Mul(num, parent.BaseFee)
		num.Div(num, denom.SetUint64(parentGasTarget))
		num.Div(num, denom.SetUint64(denominator))

		baseFee := num.Sub(parent.BaseFee, num)
		if baseFee.Cmp(common.Big0) < 0 {
			baseFee = common.Big0
		}
		return baseFee
	}
}
