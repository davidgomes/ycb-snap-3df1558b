// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
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

// StatementKind is the kind of a statement about a candidate.
type StatementKind uint8

const (
	// SecondedStatement is a Seconded statement.
	SecondedStatement StatementKind = iota
	// ValidStatement is a Valid statement.
	ValidStatement
)

func (s *StatementFilter) bitsFor(kind StatementKind) *parachaintypes.BitVec {
	if kind == SecondedStatement {
		return &s.secondedInGroup
	}
	return &s.validatedInGroup
}

// Contains returns true if the statement at the given index in group is set for the given kind.
func (s *StatementFilter) Contains(indexInGroup uint, kind StatementKind) bool {
	set, err := s.bitsFor(kind).Get(indexInGroup)
	return err == nil && set
}

// Set sets the bit for the statement at the given index in group for the given kind.
// Out of range indices are ignored.
func (s *StatementFilter) Set(indexInGroup uint, kind StatementKind) {
	_ = s.bitsFor(kind).Set(indexInGroup, true)
}

// Clone returns a deep copy of the StatementFilter.
func (s *StatementFilter) Clone() *StatementFilter {
	seconded, _ := parachaintypes.NewBitVec(s.secondedInGroup.Bits())
	validated, _ := parachaintypes.NewBitVec(s.validatedInGroup.Bits())
	return &StatementFilter{secondedInGroup: seconded, validatedInGroup: validated}
}
