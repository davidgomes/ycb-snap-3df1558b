// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	"sort"

	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
)

// mutualKnowledge is the knowledge we share with a single peer about a candidate.
type mutualKnowledge struct {
	// remoteKnowledge is what the remote peer knows, as reported by them. Updated as statements flow.
	remoteKnowledge *StatementFilter
	// localKnowledge is what we have provided to the peer. Updated as statements flow.
	localKnowledge *StatementFilter
	// receivedKnowledge is the knowledge received from the remote in its manifest, never updated afterwards.
	receivedKnowledge *StatementFilter
}

// knownBackedCandidate tracks statement distribution knowledge about a backed candidate.
type knownBackedCandidate struct {
	groupIndex      parachaintypes.GroupIndex
	localKnowledge  *StatementFilter
	mutualKnowledge map[parachaintypes.ValidatorIndex]*mutualKnowledge
}

func newKnownBackedCandidate(
	groupIndex parachaintypes.GroupIndex, localKnowledge *StatementFilter,
) *knownBackedCandidate {
	return &knownBackedCandidate{
		groupIndex:      groupIndex,
		localKnowledge:  localKnowledge,
		mutualKnowledge: make(map[parachaintypes.ValidatorIndex]*mutualKnowledge),
	}
}

func (k *knownBackedCandidate) knowledgeFor(v parachaintypes.ValidatorIndex) *mutualKnowledge {
	m, ok := k.mutualKnowledge[v]
	if !ok {
		m = &mutualKnowledge{}
		k.mutualKnowledge[v] = m
	}
	return m
}

func (k *knownBackedCandidate) hasReceivedManifestFrom(v parachaintypes.ValidatorIndex) bool {
	m, ok := k.mutualKnowledge[v]
	return ok && m.remoteKnowledge != nil
}

func (k *knownBackedCandidate) hasSentManifestTo(v parachaintypes.ValidatorIndex) bool {
	m, ok := k.mutualKnowledge[v]
	return ok && m.localKnowledge != nil
}

func (k *knownBackedCandidate) manifestSentTo(v parachaintypes.ValidatorIndex, localKnowledge *StatementFilter) {
	m := k.knowledgeFor(v)
	m.localKnowledge = localKnowledge.Clone()
}

func (k *knownBackedCandidate) manifestReceivedFrom(
	v parachaintypes.ValidatorIndex, remoteKnowledge *StatementFilter,
) {
	m := k.knowledgeFor(v)
	m.receivedKnowledge = remoteKnowledge.Clone()
	m.remoteKnowledge = remoteKnowledge.Clone()
}

func sortedValidators(validators []parachaintypes.ValidatorIndex) []parachaintypes.ValidatorIndex {
	sort.Slice(validators, func(i, j int) bool { return validators[i] < validators[j] })
	return validators
}

// directStatementSenders returns validators we have exchanged manifests with in both directions
// whose received knowledge does not include the given statement.
func (k *knownBackedCandidate) directStatementSenders(
	groupIndex parachaintypes.GroupIndex, originatorIndexInGroup uint, kind StatementKind,
) []parachaintypes.ValidatorIndex {
	if groupIndex != k.groupIndex {
		return nil
	}
	senders := []parachaintypes.ValidatorIndex{}
	for v, m := range k.mutualKnowledge {
		if m.localKnowledge != nil && m.receivedKnowledge != nil &&
			!m.receivedKnowledge.Contains(originatorIndexInGroup, kind) {
			senders = append(senders, v)
		}
	}
	return sortedValidators(senders)
}

// directStatementRecipients returns validators we have exchanged manifests with in both directions
// who do not yet know the given statement.
func (k *knownBackedCandidate) directStatementRecipients(
	groupIndex parachaintypes.GroupIndex, originatorIndexInGroup uint, kind StatementKind,
) []parachaintypes.ValidatorIndex {
	if groupIndex != k.groupIndex {
		return nil
	}
	recipients := []parachaintypes.ValidatorIndex{}
	for v, m := range k.mutualKnowledge {
		if m.localKnowledge != nil && m.remoteKnowledge != nil &&
			!m.remoteKnowledge.Contains(originatorIndexInGroup, kind) {
			recipients = append(recipients, v)
		}
	}
	return sortedValidators(recipients)
}

func (k *knownBackedCandidate) noteFreshStatement(statementIndexInGroup uint, kind StatementKind) bool {
	present := k.localKnowledge.Contains(statementIndexInGroup, kind)
	k.localKnowledge.Set(statementIndexInGroup, kind)
	return !present
}

func (k *knownBackedCandidate) sentOrReceivedDirectStatement(
	v parachaintypes.ValidatorIndex, statementIndexInGroup uint, kind StatementKind, received bool,
) {
	m, ok := k.mutualKnowledge[v]
	if !ok || m.remoteKnowledge == nil || m.localKnowledge == nil {
		return
	}
	m.remoteKnowledge.Set(statementIndexInGroup, kind)
	m.localKnowledge.Set(statementIndexInGroup, kind)
	if received && m.receivedKnowledge != nil {
		m.receivedKnowledge.Set(statementIndexInGroup, kind)
	}
}

// isPendingStatement returns true if we know the statement but the validator does not,
// considering only validators we have exchanged manifests with in both directions.
func (k *knownBackedCandidate) isPendingStatement(
	v parachaintypes.ValidatorIndex, statementIndexInGroup uint, kind StatementKind,
) bool {
	if !k.localKnowledge.Contains(statementIndexInGroup, kind) {
		return false
	}
	m, ok := k.mutualKnowledge[v]
	if !ok || m.localKnowledge == nil || m.remoteKnowledge == nil {
		return false
	}
	return !m.remoteKnowledge.Contains(statementIndexInGroup, kind)
}

// pendingStatements returns the statements we know that the validator does not, or nil
// if manifests have not been exchanged in both directions.
func (k *knownBackedCandidate) pendingStatements(v parachaintypes.ValidatorIndex) *StatementFilter {
	m, ok := k.mutualKnowledge[v]
	if !ok || m.localKnowledge == nil || m.remoteKnowledge == nil {
		return nil
	}
	pending := k.localKnowledge.Clone()
	pending.MaskSeconded(m.remoteKnowledge.secondedInGroup)
	pending.MaskValid(m.remoteKnowledge.validatedInGroup)
	return pending
}
