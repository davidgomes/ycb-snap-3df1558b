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
	// leave version byte 0
	binary.BigEndian.PutUint32(r[1:5], uint32(denom))
	binary.BigEndian.PutUint32(r[5:], uint32(elasticity))
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

const (
	// HoloceneExtraDataVersionByte is the version byte for Holocene extraData.
	HoloceneExtraDataVersionByte = uint8(0x00)
	// JovianExtraDataVersionByte is the version byte for Jovian extraData, which
	// appends an 8-byte minimum base fee to the Holocene encoding.
	JovianExtraDataVersionByte = uint8(0x01)
)

// ForkChecker reports whether Holocene and Jovian are active at a timestamp.
// *params.ChainConfig implements this.
type ForkChecker interface {
	IsHolocene(time uint64) bool
	IsJovian(time uint64) bool
}

// ValidateOptimismExtraData validates header extraData for the active Optimism fork.
func ValidateOptimismExtraData(fc ForkChecker, time uint64, extraData []byte) error {
	if fc.IsJovian(time) {
		return ValidateJovianExtraData(extraData)
	} else if fc.IsHolocene(time) {
		return ValidateHoloceneExtraData(extraData)
	} else if len(extraData) > 0 { // pre-Holocene
		return errors.New("extraData must be empty before Holocene")
	}
	return nil
}

// DecodeOptimismExtraData decodes EIP-1559 parameters and, when Jovian is active,
// the minimum base fee from header extraData. The extraData is expected to have
// already been validated.
func DecodeOptimismExtraData(fc ForkChecker, time uint64, extraData []byte) (uint64, uint64, *uint64) {
	if fc.IsJovian(time) {
		return DecodeJovianExtraData(extraData)
	} else if fc.IsHolocene(time) {
		denominator, elasticity := DecodeHoloceneExtraData(extraData)
		return denominator, elasticity, nil
	}
	return 0, 0, nil
}

// EncodeOptimismExtraData encodes header extraData for the active Optimism fork.
func EncodeOptimismExtraData(fc ForkChecker, time uint64, denominator, elasticity uint64, minBaseFee *uint64) []byte {
	if fc.IsJovian(time) {
		if minBaseFee == nil {
			panic("minBaseFee cannot be nil since the Jovian upgrade is enabled")
		}
		return EncodeJovianExtraData(denominator, elasticity, *minBaseFee)
	} else if fc.IsHolocene(time) {
		return EncodeHoloceneExtraData(denominator, elasticity)
	}
	return nil
}

// DecodeJovianExtraData decodes the Jovian extraData parameters.
// Returns 0,0,nil if the format is invalid, and denominator, elasticity, nil for
// the Holocene length so pre-Jovian history can still be decoded.
func DecodeJovianExtraData(extra []byte) (uint64, uint64, *uint64) {
	if len(extra) == 9 {
		denominator, elasticity := DecodeHolocene1559Params(extra[1:9])
		return denominator, elasticity, nil
	} else if len(extra) == 17 {
		denominator, elasticity := DecodeHolocene1559Params(extra[1:9])
		minBaseFee := binary.BigEndian.Uint64(extra[9:])
		return denominator, elasticity, &minBaseFee
	}
	return 0, 0, nil
}

// EncodeJovianExtraData encodes the EIP-1559 parameters and minimum base fee into
// the 17-byte header extraData format. Panics if EIP-1559 parameters are outside uint32 range.
func EncodeJovianExtraData(denom, elasticity, minBaseFee uint64) []byte {
	r := make([]byte, 17)
	if denom > gomath.MaxUint32 || elasticity > gomath.MaxUint32 {
		panic("eip-1559 parameters out of uint32 range")
	}
	r[0] = JovianExtraDataVersionByte
	binary.BigEndian.PutUint32(r[1:5], uint32(denom))
	binary.BigEndian.PutUint32(r[5:9], uint32(elasticity))
	binary.BigEndian.PutUint64(r[9:], minBaseFee)
	return r
}

// ValidateJovianExtraData checks that header extraData uses the Jovian 17-byte format.
// The Holocene 9-byte encoding is rejected.
func ValidateJovianExtraData(extra []byte) error {
	if len(extra) != 17 {
		return fmt.Errorf("Jovian extraData should be 17 bytes, got %d", len(extra))
	}
	if extra[0] != JovianExtraDataVersionByte {
		return fmt.Errorf("Jovian extraData version byte should be %d, got %d", JovianExtraDataVersionByte, extra[0])
	}
	if err := ValidateHolocene1559Params(extra[1:9]); err != nil {
		return err
	}
	denominator, elasticity := DecodeHolocene1559Params(extra[1:9])
	if denominator == 0 {
		return errors.New("holocene extraData must encode a non-zero denominator")
	}
	if elasticity == 0 {
		return errors.New("holocene extraData must encode a non-zero elasticity")
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
		return fmt.Errorf("holocene extraData should have 0 version byte, got %d", extra[0])
	}
	return ValidateHolocene1559Params(extra[1:])
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
		var decodedDenom, decodedElasticity uint64
		decodedDenom, decodedElasticity, minBaseFee = DecodeOptimismExtraData(config, parent.Time, parent.Extra)
		if decodedDenom == 0 {
			// this shouldn't happen as the ExtraData should have been validated prior
			panic("invalid eip-1559 params in extradata")
		}
		denominator, elasticity = decodedDenom, decodedElasticity
	}
	parentGasTarget := parent.GasLimit / elasticity
	// If the parent gasUsed is the same as the target, the baseFee remains unchanged.
	if parent.GasUsed == parentGasTarget {
		return applyMinBaseFee(new(big.Int).Set(parent.BaseFee), minBaseFee)
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
			return applyMinBaseFee(num.Add(parent.BaseFee, common.Big1), minBaseFee)
		}
		return applyMinBaseFee(num.Add(parent.BaseFee, num), minBaseFee)
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
		return applyMinBaseFee(baseFee, minBaseFee)
	}
}

// applyMinBaseFee raises baseFee to the Jovian minimum when one is configured.
// A nil minimum leaves the computed fee unchanged, including a zero fee.
func applyMinBaseFee(baseFee *big.Int, minBaseFee *uint64) *big.Int {
	if minBaseFee == nil {
		return baseFee
	}
	minBaseFeeBig := new(big.Int).SetUint64(*minBaseFee)
	if baseFee.Cmp(minBaseFeeBig) < 0 {
		return minBaseFeeBig
	}
	return baseFee
}
