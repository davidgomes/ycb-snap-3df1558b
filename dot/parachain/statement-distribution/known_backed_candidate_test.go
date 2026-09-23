// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	"testing"

	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/stretchr/testify/require"
)

func filterWith(t *testing.T, size uint, seconded, valid []uint) *StatementFilter {
	t.Helper()
	f, err := NewStatementFilter(size, false)
	require.NoError(t, err)
	for _, i := range seconded {
		f.Set(i, SecondedStatement)
	}
	for _, i := range valid {
		f.Set(i, ValidStatement)
	}
	return f
}

func TestKnownBackedCandidate(t *testing.T) {
	t.Parallel()

	local := filterWith(t, 3, []uint{0}, []uint{1})
	k := newKnownBackedCandidate(parachaintypes.GroupIndex(1), local)

	peer := parachaintypes.ValidatorIndex(5)
	other := parachaintypes.ValidatorIndex(7)

	require.False(t, k.hasSentManifestTo(peer))
	require.False(t, k.hasReceivedManifestFrom(peer))
	require.Nil(t, k.pendingStatements(peer))
	require.False(t, k.isPendingStatement(peer, 0, SecondedStatement))

	k.manifestSentTo(peer, local)
	require.True(t, k.hasSentManifestTo(peer))
	require.Nil(t, k.pendingStatements(peer))

	k.manifestReceivedFrom(peer, filterWith(t, 3, []uint{0}, nil))
	require.True(t, k.hasReceivedManifestFrom(peer))

	require.False(t, k.isPendingStatement(peer, 0, SecondedStatement))
	require.True(t, k.isPendingStatement(peer, 1, ValidStatement))
	require.False(t, k.isPendingStatement(peer, 2, ValidStatement))

	require.Equal(t, filterWith(t, 3, nil, []uint{1}), k.pendingStatements(peer))

	require.Equal(t, []parachaintypes.ValidatorIndex{peer},
		k.directStatementRecipients(1, 2, SecondedStatement))
	require.Empty(t, k.directStatementRecipients(1, 0, SecondedStatement))
	require.Nil(t, k.directStatementRecipients(2, 2, SecondedStatement))
	require.Equal(t, []parachaintypes.ValidatorIndex{peer},
		k.directStatementSenders(1, 2, ValidStatement))
	require.Nil(t, k.directStatementSenders(0, 2, ValidStatement))

	require.True(t, k.noteFreshStatement(2, SecondedStatement))
	require.False(t, k.noteFreshStatement(2, SecondedStatement))
	require.True(t, k.isPendingStatement(peer, 2, SecondedStatement))

	k.sentOrReceivedDirectStatement(peer, 2, SecondedStatement, false)
	require.False(t, k.isPendingStatement(peer, 2, SecondedStatement))
	require.Empty(t, k.directStatementRecipients(1, 2, SecondedStatement))
	require.Equal(t, []parachaintypes.ValidatorIndex{peer},
		k.directStatementSenders(1, 2, SecondedStatement))

	k.sentOrReceivedDirectStatement(peer, 2, ValidStatement, true)
	require.Empty(t, k.directStatementSenders(1, 2, ValidStatement))

	// no-op for unknown peer
	k.sentOrReceivedDirectStatement(other, 1, ValidStatement, true)
	require.False(t, k.hasSentManifestTo(other))
}
