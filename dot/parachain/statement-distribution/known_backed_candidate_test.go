// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	"testing"

	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/stretchr/testify/require"
)

func newFilter(t *testing.T, seconded, validated []bool) *StatementFilter {
	t.Helper()

	secondedInGroup, err := parachaintypes.NewBitVec(seconded)
	require.NoError(t, err)
	validatedInGroup, err := parachaintypes.NewBitVec(validated)
	require.NoError(t, err)

	return &StatementFilter{secondedInGroup: secondedInGroup, validatedInGroup: validatedInGroup}
}

func TestStatementFilter_ContainsSetClone(t *testing.T) {
	t.Parallel()

	filter, err := NewStatementFilter(4, false)
	require.NoError(t, err)

	require.False(t, filter.Contains(1, statementKindSeconded))
	filter.Set(1, statementKindSeconded)
	require.True(t, filter.Contains(1, statementKindSeconded))
	require.False(t, filter.Contains(1, statementKindValid))

	filter.Set(2, statementKindValid)
	require.True(t, filter.Contains(2, statementKindValid))

	// out of bounds and unknown kinds are ignored
	filter.Set(10, statementKindValid)
	require.False(t, filter.Contains(10, statementKindValid))
	filter.Set(0, statementKind(42))
	require.False(t, filter.Contains(0, statementKind(42)))

	clone := filter.Clone()
	require.Equal(t, filter, clone)
	clone.Set(3, statementKindSeconded)
	require.True(t, clone.Contains(3, statementKindSeconded))
	require.False(t, filter.Contains(3, statementKindSeconded))
}

func TestKnownBackedCandidate_Manifests(t *testing.T) {
	t.Parallel()

	candidate := newKnownBackedCandidate(0, newFilter(t,
		[]bool{true, false, false}, []bool{false, false, false}))

	require.False(t, candidate.hasSentManifestTo(1))
	require.False(t, candidate.hasReceivedManifestFrom(1))

	sent := newFilter(t, []bool{true, false, false}, []bool{false, false, false})
	candidate.manifestSentTo(1, sent)
	require.True(t, candidate.hasSentManifestTo(1))
	require.False(t, candidate.hasReceivedManifestFrom(1))

	// the stored knowledge must not alias the argument
	sent.Set(2, statementKindSeconded)
	require.False(t, candidate.mutualKnowledge[1].localKnowledge.Contains(2, statementKindSeconded))

	received := newFilter(t, []bool{false, true, false}, []bool{false, false, false})
	candidate.manifestReceivedFrom(2, received)
	require.True(t, candidate.hasReceivedManifestFrom(2))
	require.False(t, candidate.hasSentManifestTo(2))
	require.Equal(t, received, candidate.mutualKnowledge[2].remoteKnowledge)
	require.Equal(t, received, candidate.mutualKnowledge[2].receivedKnowledge)
}

func TestKnownBackedCandidate_NoteFreshStatement(t *testing.T) {
	t.Parallel()

	candidate := newKnownBackedCandidate(0, newFilter(t,
		[]bool{true, false}, []bool{false, false}))

	require.False(t, candidate.noteFreshStatement(0, statementKindSeconded))
	require.True(t, candidate.noteFreshStatement(0, statementKindValid))
	require.False(t, candidate.noteFreshStatement(0, statementKindValid))
	require.True(t, candidate.noteFreshStatement(1, statementKindSeconded))
	require.True(t, candidate.localKnowledge.Contains(1, statementKindSeconded))
}

func TestKnownBackedCandidate_SendersAndRecipients(t *testing.T) {
	t.Parallel()

	candidate := newKnownBackedCandidate(1, newFilter(t,
		[]bool{true, true, false}, []bool{false, false, false}))

	// validator 5: manifests exchanged both ways, remote knows seconded 0
	candidate.manifestReceivedFrom(5, newFilter(t, []bool{true, false, false}, []bool{false, false, false}))
	candidate.manifestSentTo(5, newFilter(t, []bool{true, true, false}, []bool{false, false, false}))
	// validator 6: only received a manifest, remote knows nothing
	candidate.manifestReceivedFrom(6, newFilter(t, []bool{false, false, false}, []bool{false, false, false}))
	// validator 7: only sent a manifest
	candidate.manifestSentTo(7, newFilter(t, []bool{true, true, false}, []bool{false, false, false}))

	require.Nil(t, candidate.directStatementSenders(2, 0, statementKindSeconded))
	require.Nil(t, candidate.directStatementRecipients(2, 0, statementKindSeconded))

	require.Equal(t, []parachaintypes.ValidatorIndex{6},
		candidate.directStatementSenders(1, 0, statementKindSeconded))
	require.Equal(t, []parachaintypes.ValidatorIndex{5, 6},
		candidate.directStatementSenders(1, 1, statementKindSeconded))

	require.Nil(t, candidate.directStatementRecipients(1, 0, statementKindSeconded))
	require.Equal(t, []parachaintypes.ValidatorIndex{5},
		candidate.directStatementRecipients(1, 1, statementKindSeconded))

	// sending a statement updates remote and local but not received knowledge
	candidate.sentOrReceivedDirectStatement(5, 1, statementKindSeconded, false)
	require.Nil(t, candidate.directStatementRecipients(1, 1, statementKindSeconded))
	require.Equal(t, []parachaintypes.ValidatorIndex{5, 6},
		candidate.directStatementSenders(1, 1, statementKindSeconded))

	// receiving a statement also updates received knowledge
	candidate.sentOrReceivedDirectStatement(5, 2, statementKindValid, true)
	require.True(t, candidate.mutualKnowledge[5].receivedKnowledge.Contains(2, statementKindValid))
	require.Equal(t, []parachaintypes.ValidatorIndex{6},
		candidate.directStatementSenders(1, 2, statementKindValid))

	// without both manifests, only received knowledge is updated on receipt
	candidate.sentOrReceivedDirectStatement(6, 0, statementKindValid, true)
	require.False(t, candidate.mutualKnowledge[6].remoteKnowledge.Contains(0, statementKindValid))
	require.True(t, candidate.mutualKnowledge[6].receivedKnowledge.Contains(0, statementKindValid))

	// unknown validators are ignored
	candidate.sentOrReceivedDirectStatement(99, 0, statementKindValid, true)
	_, ok := candidate.mutualKnowledge[99]
	require.False(t, ok)
}

func TestKnownBackedCandidate_PendingStatements(t *testing.T) {
	t.Parallel()

	candidate := newKnownBackedCandidate(0, newFilter(t,
		[]bool{true, false, false}, []bool{false, false, false}))

	candidate.manifestSentTo(1, newFilter(t, []bool{true, false, false}, []bool{false, false, false}))
	candidate.manifestReceivedFrom(1, newFilter(t, []bool{false, false, false}, []bool{false, true, false}))
	candidate.manifestSentTo(2, newFilter(t, []bool{true, false, false}, []bool{false, false, false}))

	require.True(t, candidate.isPendingStatement(1, 0, statementKindSeconded))
	require.False(t, candidate.isPendingStatement(1, 1, statementKindValid))
	require.False(t, candidate.isPendingStatement(2, 0, statementKindSeconded))
	require.False(t, candidate.isPendingStatement(3, 0, statementKindSeconded))

	require.Nil(t, candidate.pendingStatements(2))
	require.Nil(t, candidate.pendingStatements(3))

	// pending statements are based on the full local knowledge
	candidate.noteFreshStatement(1, statementKindValid)
	candidate.noteFreshStatement(2, statementKindValid)
	require.Equal(t,
		newFilter(t, []bool{true, false, false}, []bool{false, false, true}),
		candidate.pendingStatements(1))

	candidate.sentOrReceivedDirectStatement(1, 0, statementKindSeconded, false)
	require.False(t, candidate.isPendingStatement(1, 0, statementKindSeconded))
	require.Equal(t,
		newFilter(t, []bool{false, false, false}, []bool{false, false, true}),
		candidate.pendingStatements(1))

	// computing pending statements must not mutate local knowledge
	require.True(t, candidate.localKnowledge.Contains(0, statementKindSeconded))
}
