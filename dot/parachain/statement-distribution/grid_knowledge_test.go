// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	"testing"

	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/stretchr/testify/require"
)

// mustStatementFilter builds a StatementFilter from strings of '0' and '1', where '1' means known.
func mustStatementFilter(t *testing.T, seconded, validated string) *StatementFilter {
	t.Helper()

	toBits := func(s string) []bool {
		bits := make([]bool, len(s))
		for i, c := range s {
			bits[i] = c == '1'
		}
		return bits
	}

	secondedInGroup, err := parachaintypes.NewBitVec(toBits(seconded))
	require.NoError(t, err)
	validatedInGroup, err := parachaintypes.NewBitVec(toBits(validated))
	require.NoError(t, err)

	return &StatementFilter{
		secondedInGroup:  secondedInGroup,
		validatedInGroup: validatedInGroup,
	}
}

func TestStatementFilterKnowledgeQueries(t *testing.T) {
	t.Parallel()

	t.Run("set_marks_only_the_given_kind", func(t *testing.T) {
		t.Parallel()

		filter := mustStatementFilter(t, "000", "000")
		require.False(t, filter.contains(1, seconded))
		require.False(t, filter.contains(1, valid))

		filter.set(1, seconded)
		require.True(t, filter.contains(1, seconded))
		require.False(t, filter.contains(1, valid))

		filter.set(2, valid)
		require.Equal(t, mustStatementFilter(t, "010", "001"), filter)
	})

	t.Run("set_is_idempotent", func(t *testing.T) {
		t.Parallel()

		filter := mustStatementFilter(t, "010", "000")
		filter.set(1, seconded)
		require.Equal(t, mustStatementFilter(t, "010", "000"), filter)
	})

	t.Run("out_of_bounds_index_is_ignored", func(t *testing.T) {
		t.Parallel()

		filter := mustStatementFilter(t, "111", "111")
		require.False(t, filter.contains(3, seconded))
		require.False(t, filter.contains(3, valid))

		filter = mustStatementFilter(t, "000", "000")
		filter.set(3, seconded)
		filter.set(3, valid)
		require.Equal(t, mustStatementFilter(t, "000", "000"), filter)
	})

	t.Run("nil_filter_knows_nothing", func(t *testing.T) {
		t.Parallel()

		var filter *StatementFilter
		require.False(t, filter.contains(0, seconded))
		require.NotPanics(t, func() { filter.set(0, seconded) })
	})

	t.Run("clone_does_not_share_bits", func(t *testing.T) {
		t.Parallel()

		original := mustStatementFilter(t, "100", "010")
		cloned := original.clone()
		require.Equal(t, original, cloned)

		cloned.set(2, seconded)
		cloned.set(0, valid)
		require.Equal(t, mustStatementFilter(t, "100", "010"), original)
		require.Equal(t, mustStatementFilter(t, "101", "110"), cloned)
	})

	t.Run("clone_empty_filter", func(t *testing.T) {
		t.Parallel()

		original, err := NewStatementFilter(0, false)
		require.NoError(t, err)
		require.Equal(t, original, original.clone())
	})
}

func TestKnownBackedCandidateManifestExchange(t *testing.T) {
	t.Parallel()

	const peer = parachaintypes.ValidatorIndex(10)

	t.Run("no_manifest_exchanged", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "000"))
		require.False(t, candidate.hasSentManifestTo(peer))
		require.False(t, candidate.hasReceivedManifestFrom(peer))
	})

	t.Run("manifest_sent_to", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "000"))
		localKnowledge := mustStatementFilter(t, "100", "000")
		candidate.manifestSentTo(peer, localKnowledge)

		require.True(t, candidate.hasSentManifestTo(peer))
		require.False(t, candidate.hasReceivedManifestFrom(peer))
		require.False(t, candidate.hasSentManifestTo(peer+1))

		knowledge := candidate.mutualKnowledge[peer]
		require.Equal(t, mustStatementFilter(t, "100", "000"), knowledge.localKnowledge)
		require.Equal(t, mustStatementFilter(t, "000", "000"), knowledge.receivedKnowledge)
		require.Nil(t, knowledge.remoteKnowledge)

		localKnowledge.set(1, valid)
		require.Equal(t, mustStatementFilter(t, "100", "000"), knowledge.localKnowledge)
	})

	t.Run("manifest_received_from", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "000"))
		remoteKnowledge := mustStatementFilter(t, "010", "001")
		candidate.manifestReceivedFrom(peer, remoteKnowledge)

		require.True(t, candidate.hasReceivedManifestFrom(peer))
		require.False(t, candidate.hasSentManifestTo(peer))
		require.False(t, candidate.hasReceivedManifestFrom(peer+1))

		knowledge := candidate.mutualKnowledge[peer]
		require.Equal(t, mustStatementFilter(t, "010", "001"), knowledge.remoteKnowledge)
		require.Nil(t, knowledge.localKnowledge)
		require.Nil(t, knowledge.receivedKnowledge)

		remoteKnowledge.set(0, seconded)
		require.Equal(t, mustStatementFilter(t, "010", "001"), knowledge.remoteKnowledge)
	})

	t.Run("sending_manifest_again_resets_received_knowledge", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "110", "000"))
		candidate.manifestReceivedFrom(peer, mustStatementFilter(t, "010", "000"))
		candidate.manifestSentTo(peer, mustStatementFilter(t, "100", "000"))
		candidate.sentOrReceivedDirectStatement(peer, 2, valid, true)
		require.Equal(t, mustStatementFilter(t, "000", "001"), candidate.mutualKnowledge[peer].receivedKnowledge)

		candidate.manifestSentTo(peer, mustStatementFilter(t, "110", "001"))
		knowledge := candidate.mutualKnowledge[peer]
		require.Equal(t, mustStatementFilter(t, "000", "000"), knowledge.receivedKnowledge)
		require.Equal(t, mustStatementFilter(t, "110", "001"), knowledge.localKnowledge)
		require.Equal(t, mustStatementFilter(t, "010", "001"), knowledge.remoteKnowledge)
	})

	t.Run("zero_value_candidate", func(t *testing.T) {
		t.Parallel()

		candidate := &knownBackedCandidate{}
		require.False(t, candidate.hasReceivedManifestFrom(peer))
		candidate.manifestReceivedFrom(peer, mustStatementFilter(t, "1", "0"))
		require.True(t, candidate.hasReceivedManifestFrom(peer))
	})
}

func TestKnownBackedCandidateNoteFreshStatement(t *testing.T) {
	t.Parallel()

	t.Run("reports_freshness_per_statement_kind", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "000", "000"))
		require.True(t, candidate.noteFreshStatement(0, seconded))
		require.False(t, candidate.noteFreshStatement(0, seconded))
		require.True(t, candidate.noteFreshStatement(0, valid))
		require.True(t, candidate.noteFreshStatement(2, valid))
		require.False(t, candidate.noteFreshStatement(2, valid))

		require.Equal(t, mustStatementFilter(t, "100", "101"), candidate.localKnowledge)
	})

	t.Run("does_not_change_knowledge_indicated_to_peers", func(t *testing.T) {
		t.Parallel()

		const peer = parachaintypes.ValidatorIndex(10)
		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "000"))
		candidate.manifestSentTo(peer, candidate.localKnowledge)

		require.True(t, candidate.noteFreshStatement(1, valid))
		require.Equal(t, mustStatementFilter(t, "100", "010"), candidate.localKnowledge)
		require.Equal(t, mustStatementFilter(t, "100", "000"), candidate.mutualKnowledge[peer].localKnowledge)
	})
}

func TestKnownBackedCandidatePendingStatementsFromLocalKnowledge(t *testing.T) {
	t.Parallel()

	const peer = parachaintypes.ValidatorIndex(10)

	t.Run("nothing_pending_without_full_manifest_exchange", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "010"))
		require.Nil(t, candidate.pendingStatements(peer))
		require.False(t, candidate.isPendingStatement(peer, 0, seconded))

		candidate.manifestReceivedFrom(peer, mustStatementFilter(t, "000", "000"))
		require.Nil(t, candidate.pendingStatements(peer))
		require.False(t, candidate.isPendingStatement(peer, 0, seconded))

		other := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "010"))
		other.manifestSentTo(peer, mustStatementFilter(t, "100", "010"))
		require.Nil(t, other.pendingStatements(peer))
		require.False(t, other.isPendingStatement(peer, 0, seconded))
	})

	t.Run("uses_full_local_knowledge_rather_than_indicated_knowledge", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "010"))
		candidate.manifestSentTo(peer, mustStatementFilter(t, "100", "000"))
		candidate.manifestReceivedFrom(peer, mustStatementFilter(t, "000", "000"))
		require.Equal(t, mustStatementFilter(t, "100", "010"), candidate.pendingStatements(peer))

		candidate.noteFreshStatement(2, valid)
		require.Equal(t, mustStatementFilter(t, "100", "011"), candidate.pendingStatements(peer))
	})

	t.Run("excludes_statements_known_by_the_peer", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "011"))
		candidate.manifestSentTo(peer, mustStatementFilter(t, "100", "011"))
		candidate.manifestReceivedFrom(peer, mustStatementFilter(t, "100", "001"))

		require.Equal(t, mustStatementFilter(t, "000", "010"), candidate.pendingStatements(peer))
		require.False(t, candidate.isPendingStatement(peer, 0, seconded))
		require.True(t, candidate.isPendingStatement(peer, 1, valid))
		require.False(t, candidate.isPendingStatement(peer, 2, valid))
		// anything the peer doesn't know is pending, regardless of our own knowledge
		require.True(t, candidate.isPendingStatement(peer, 2, seconded))

		require.Nil(t, candidate.pendingStatements(peer+1))
		require.False(t, candidate.isPendingStatement(peer+1, 1, valid))
	})

	t.Run("cleared_after_sending_statement", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "010"))
		candidate.manifestSentTo(peer, mustStatementFilter(t, "100", "010"))
		candidate.manifestReceivedFrom(peer, mustStatementFilter(t, "100", "000"))
		require.True(t, candidate.isPendingStatement(peer, 1, valid))

		candidate.sentOrReceivedDirectStatement(peer, 1, valid, false)
		require.False(t, candidate.isPendingStatement(peer, 1, valid))
		require.Equal(t, mustStatementFilter(t, "000", "000"), candidate.pendingStatements(peer))
	})

	t.Run("returns_a_copy", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "000"))
		candidate.manifestSentTo(peer, mustStatementFilter(t, "100", "000"))
		candidate.manifestReceivedFrom(peer, mustStatementFilter(t, "000", "000"))

		pending := candidate.pendingStatements(peer)
		pending.set(1, seconded)
		require.Equal(t, mustStatementFilter(t, "100", "000"), candidate.localKnowledge)
		require.Equal(t, mustStatementFilter(t, "000", "000"), candidate.mutualKnowledge[peer].remoteKnowledge)
	})
}

func TestKnownBackedCandidateDirectStatementPeers(t *testing.T) {
	t.Parallel()

	const (
		groupIndex = parachaintypes.GroupIndex(2)
		// exchanged manifests with us, knowing only the seconded statement
		exchangedA = parachaintypes.ValidatorIndex(10)
		// only sent us a manifest
		receivedOnly = parachaintypes.ValidatorIndex(11)
		// only got a manifest from us
		sentOnly = parachaintypes.ValidatorIndex(12)
		// exchanged manifests with us, knowing only the valid statement from the second validator
		exchangedB = parachaintypes.ValidatorIndex(13)
	)

	newCandidate := func(t *testing.T) *knownBackedCandidate {
		t.Helper()

		candidate := newKnownBackedCandidate(groupIndex, mustStatementFilter(t, "100", "010"))
		candidate.manifestSentTo(exchangedA, mustStatementFilter(t, "100", "000"))
		candidate.manifestReceivedFrom(exchangedA, mustStatementFilter(t, "100", "000"))
		candidate.manifestReceivedFrom(receivedOnly, mustStatementFilter(t, "000", "000"))
		candidate.manifestSentTo(sentOnly, mustStatementFilter(t, "100", "000"))
		candidate.manifestSentTo(exchangedB, mustStatementFilter(t, "100", "000"))
		candidate.manifestReceivedFrom(exchangedB, mustStatementFilter(t, "000", "010"))
		return candidate
	}

	t.Run("recipients_are_peers_not_knowing_the_statement", func(t *testing.T) {
		t.Parallel()

		candidate := newCandidate(t)
		require.Equal(t,
			[]parachaintypes.ValidatorIndex{exchangedA},
			candidate.directStatementRecipients(groupIndex, 1, valid))
		require.Equal(t,
			[]parachaintypes.ValidatorIndex{exchangedB},
			candidate.directStatementRecipients(groupIndex, 0, seconded))
		require.Equal(t,
			[]parachaintypes.ValidatorIndex{exchangedA, exchangedB},
			candidate.directStatementRecipients(groupIndex, 2, valid))
	})

	t.Run("senders_are_peers_that_did_not_send_the_statement", func(t *testing.T) {
		t.Parallel()

		candidate := newCandidate(t)
		require.Equal(t,
			map[parachaintypes.ValidatorIndex]bool{exchangedA: false, exchangedB: false},
			candidate.directStatementSenders(groupIndex, 1, valid))
		require.Equal(t,
			map[parachaintypes.ValidatorIndex]bool{exchangedA: true, exchangedB: true},
			candidate.directStatementSenders(groupIndex, 0, seconded))

		candidate.sentOrReceivedDirectStatement(exchangedB, 1, valid, true)
		require.Equal(t,
			map[parachaintypes.ValidatorIndex]bool{exchangedA: false},
			candidate.directStatementSenders(groupIndex, 1, valid))
	})

	t.Run("sending_statement_updates_senders_and_recipients", func(t *testing.T) {
		t.Parallel()

		candidate := newCandidate(t)
		candidate.sentOrReceivedDirectStatement(exchangedA, 1, valid, false)

		require.Empty(t, candidate.directStatementRecipients(groupIndex, 1, valid))
		require.Equal(t,
			map[parachaintypes.ValidatorIndex]bool{exchangedA: true, exchangedB: false},
			candidate.directStatementSenders(groupIndex, 1, valid))
	})

	t.Run("other_group", func(t *testing.T) {
		t.Parallel()

		candidate := newCandidate(t)
		require.Empty(t, candidate.directStatementRecipients(groupIndex+1, 1, valid))
		require.Empty(t, candidate.directStatementSenders(groupIndex+1, 1, valid))
	})
}

func TestKnownBackedCandidateSentOrReceivedDirectStatement(t *testing.T) {
	t.Parallel()

	const peer = parachaintypes.ValidatorIndex(10)

	t.Run("unknown_validator", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "000"))
		candidate.sentOrReceivedDirectStatement(peer, 0, seconded, true)
		require.Empty(t, candidate.mutualKnowledge)
	})

	t.Run("after_manifest_exchange", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "110", "000"))
		candidate.manifestSentTo(peer, mustStatementFilter(t, "100", "000"))
		candidate.manifestReceivedFrom(peer, mustStatementFilter(t, "010", "000"))

		candidate.sentOrReceivedDirectStatement(peer, 2, valid, false)
		knowledge := candidate.mutualKnowledge[peer]
		require.Equal(t, mustStatementFilter(t, "100", "001"), knowledge.localKnowledge)
		require.Equal(t, mustStatementFilter(t, "010", "001"), knowledge.remoteKnowledge)
		require.Equal(t, mustStatementFilter(t, "000", "000"), knowledge.receivedKnowledge)

		candidate.sentOrReceivedDirectStatement(peer, 0, valid, true)
		require.Equal(t, mustStatementFilter(t, "100", "101"), knowledge.localKnowledge)
		require.Equal(t, mustStatementFilter(t, "010", "101"), knowledge.remoteKnowledge)
		require.Equal(t, mustStatementFilter(t, "000", "100"), knowledge.receivedKnowledge)
	})

	t.Run("only_manifest_sent", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "000"))
		candidate.manifestSentTo(peer, mustStatementFilter(t, "100", "000"))

		candidate.sentOrReceivedDirectStatement(peer, 1, valid, true)
		knowledge := candidate.mutualKnowledge[peer]
		require.Equal(t, mustStatementFilter(t, "100", "000"), knowledge.localKnowledge)
		require.Equal(t, mustStatementFilter(t, "000", "010"), knowledge.receivedKnowledge)
		require.Nil(t, knowledge.remoteKnowledge)
	})

	t.Run("only_manifest_received", func(t *testing.T) {
		t.Parallel()

		candidate := newKnownBackedCandidate(1, mustStatementFilter(t, "100", "000"))
		candidate.manifestReceivedFrom(peer, mustStatementFilter(t, "010", "000"))

		candidate.sentOrReceivedDirectStatement(peer, 1, valid, true)
		knowledge := candidate.mutualKnowledge[peer]
		require.Equal(t, mustStatementFilter(t, "010", "000"), knowledge.remoteKnowledge)
		require.Nil(t, knowledge.localKnowledge)
		require.Nil(t, knowledge.receivedKnowledge)
	})
}
