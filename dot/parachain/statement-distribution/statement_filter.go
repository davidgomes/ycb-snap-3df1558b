// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
)

// statementKind is the kind of a statement, either Seconded or Valid.
type statementKind uint8

const (
	statementKindSeconded statementKind = iota
	statementKindValid
)

// StatementFilter contains bitfields indicating the statements that are known or undesired about a candidate.
type StatementFilter struct {
	// Seconded statements. '1' is known or undesired.
	secondedInGroup parachaintypes.BitVec
	// Valid statements. '1' is known or undesired.
	validatedInGroup parachaintypes.BitVec
}

// NewStatementFilter creates a new StatementFilter.
// If full is true, the StatementFilter will be initialised with all bits set to 1.
func NewStatementFilter(groupSize uint, full bool) (*StatementFilter, error) {
	bits := make([]bool, groupSize)
	if full {
		for i := range bits {
			bits[i] = true
		}
	}

	secondedInGroup, err := parachaintypes.NewBitVec(bits)
	if err != nil {
		return nil, err
	}

	validatedInGroup, err := parachaintypes.NewBitVec(bits)
	if err != nil {
		return nil, err
	}

	return &StatementFilter{
		secondedInGroup:  secondedInGroup,
		validatedInGroup: validatedInGroup,
	}, nil
}

// HasLen returns true if the StatementFilter has the specified length in both groups.
func (s *StatementFilter) HasLen(len int) bool {
	return s.secondedInGroup.Len() == len && s.validatedInGroup.Len() == len
}

// BackingValidators determines the number of backing validators in the StatementFilter.
func (s *StatementFilter) BackingValidators() int {
	count := 0

	for i, seconded := range s.secondedInGroup.Bits() {
		validated, err := s.validatedInGroup.Get(uint(i))
		if err != nil {
			panic("both groups were constructed with the same size. qed")
		}

		if seconded || validated { // no double-counting
			count++
		}
	}

	return count
}

// HasSeconded returns true if the StatementFilter has at least one seconded statement.
func (s *StatementFilter) HasSeconded() bool {
	return s.secondedInGroup.CountOnes() > 0
}

// MaskSeconded masks out Seconded statements in the filter according to the provided BitVec.
// Bits appearing in mask will not appear in the filter afterwards.
func (s *StatementFilter) MaskSeconded(mask parachaintypes.BitVec) {
	s.secondedInGroup.Mask(mask)
}

// MaskValid masks out Valid statements in the filter according to the provided BitVec.
// Bits appearing in mask will not appear in the filter afterwards.
func (s *StatementFilter) MaskValid(mask parachaintypes.BitVec) {
	s.validatedInGroup.Mask(mask)
}

func (s *StatementFilter) bitsFor(kind statementKind) *parachaintypes.BitVec {
	switch kind {
	case statementKindSeconded:
		return &s.secondedInGroup
	case statementKindValid:
		return &s.validatedInGroup
	default:
		return nil
	}
}

// Contains returns true if the statement of the given kind from the validator at
// indexInGroup is present in the filter. Out of bounds indices are never contained.
func (s *StatementFilter) Contains(indexInGroup uint, kind statementKind) bool {
	bits := s.bitsFor(kind)
	if bits == nil {
		return false
	}

	set, err := bits.Get(indexInGroup)
	return err == nil && set
}

// Set marks the statement of the given kind from the validator at indexInGroup
// as present in the filter. Out of bounds indices are ignored.
func (s *StatementFilter) Set(indexInGroup uint, kind statementKind) {
	bits := s.bitsFor(kind)
	if bits == nil {
		return
	}

	_ = bits.Set(indexInGroup, true)
}

// Clone returns a deep copy of the StatementFilter.
func (s *StatementFilter) Clone() *StatementFilter {
	secondedInGroup, _ := parachaintypes.NewBitVec(s.secondedInGroup.Bits())
	validatedInGroup, _ := parachaintypes.NewBitVec(s.validatedInGroup.Bits())

	return &StatementFilter{
		secondedInGroup:  secondedInGroup,
		validatedInGroup: validatedInGroup,
	}
}
