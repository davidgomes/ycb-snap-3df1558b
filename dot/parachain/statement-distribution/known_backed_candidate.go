// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	"slices"

	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
)

// mutualKnowledge tracks what we and a single grid peer know about a backed candidate.
type mutualKnowledge struct {
	// remoteKnowledge is the knowledge the peer has advertised to us via a manifest,
	// updated as statements are exchanged. nil until a manifest was received.
	remoteKnowledge *StatementFilter
	// localKnowledge is the knowledge we have advertised to the peer via a manifest,
	// updated as statements are exchanged. nil until a manifest was sent.
	localKnowledge *StatementFilter
	// receivedKnowledge is the knowledge the peer has advertised to us plus the
	// statements it has sent us since; it is not updated with statements we send.
	receivedKnowledge *StatementFilter
}

// knownBackedCandidate tracks the grid knowledge about a single backed candidate.
type knownBackedCandidate struct {
	groupIndex parachaintypes.GroupIndex
	// localKnowledge contains all the statements we have for the candidate.
	localKnowledge  *StatementFilter
	mutualKnowledge map[parachaintypes.ValidatorIndex]*mutualKnowledge
}

func newKnownBackedCandidate(
	groupIndex parachaintypes.GroupIndex,
	localKnowledge *StatementFilter,
) *knownBackedCandidate {
	return &knownBackedCandidate{
		groupIndex:      groupIndex,
		localKnowledge:  localKnowledge,
		mutualKnowledge: make(map[parachaintypes.ValidatorIndex]*mutualKnowledge),
	}
}

func (k *knownBackedCandidate) mutualKnowledgeFor(validator parachaintypes.ValidatorIndex) *mutualKnowledge {
	knowledge, ok := k.mutualKnowledge[validator]
	if !ok {
		knowledge = &mutualKnowledge{}
		k.mutualKnowledge[validator] = knowledge
	}
	return knowledge
}

func (k *knownBackedCandidate) hasReceivedManifestFrom(validator parachaintypes.ValidatorIndex) bool {
	knowledge, ok := k.mutualKnowledge[validator]
	return ok && knowledge.remoteKnowledge != nil
}

func (k *knownBackedCandidate) hasSentManifestTo(validator parachaintypes.ValidatorIndex) bool {
	knowledge, ok := k.mutualKnowledge[validator]
	return ok && knowledge.localKnowledge != nil
}

// manifestSentTo records the knowledge we advertised to the validator in a manifest.
func (k *knownBackedCandidate) manifestSentTo(
	validator parachaintypes.ValidatorIndex,
	localKnowledge *StatementFilter,
) {
	k.mutualKnowledgeFor(validator).localKnowledge = localKnowledge.Clone()
}

// manifestReceivedFrom records the knowledge the validator advertised to us in a manifest.
func (k *knownBackedCandidate) manifestReceivedFrom(
	validator parachaintypes.ValidatorIndex,
	remoteKnowledge *StatementFilter,
) {
	knowledge := k.mutualKnowledgeFor(validator)
	knowledge.remoteKnowledge = remoteKnowledge.Clone()
	knowledge.receivedKnowledge = remoteKnowledge.Clone()
}

// directStatementSenders returns the validators we have received a manifest from
// that could still send us the given statement.
func (k *knownBackedCandidate) directStatementSenders(
	groupIndex parachaintypes.GroupIndex,
	originatorIndexInGroup uint,
	kind statementKind,
) []parachaintypes.ValidatorIndex {
	if groupIndex != k.groupIndex {
		return nil
	}

	var senders []parachaintypes.ValidatorIndex
	for validator, knowledge := range k.mutualKnowledge {
		if knowledge.remoteKnowledge == nil || knowledge.receivedKnowledge == nil {
			continue
		}
		if !knowledge.receivedKnowledge.Contains(originatorIndexInGroup, kind) {
			senders = append(senders, validator)
		}
	}

	slices.Sort(senders)
	return senders
}

// directStatementRecipients returns the validators we have sent a manifest to
// that do not yet know the given statement.
func (k *knownBackedCandidate) directStatementRecipients(
	groupIndex parachaintypes.GroupIndex,
	originatorIndexInGroup uint,
	kind statementKind,
) []parachaintypes.ValidatorIndex {
	if groupIndex != k.groupIndex {
		return nil
	}

	var recipients []parachaintypes.ValidatorIndex
	for validator, knowledge := range k.mutualKnowledge {
		if knowledge.localKnowledge == nil || knowledge.remoteKnowledge == nil {
			continue
		}
		if !knowledge.remoteKnowledge.Contains(originatorIndexInGroup, kind) {
			recipients = append(recipients, validator)
		}
	}

	slices.Sort(recipients)
	return recipients
}

// noteFreshStatement adds the statement to our local knowledge and returns
// true if it was not already known.
func (k *knownBackedCandidate) noteFreshStatement(statementIndexInGroup uint, kind statementKind) bool {
	reallyFresh := !k.localKnowledge.Contains(statementIndexInGroup, kind)
	k.localKnowledge.Set(statementIndexInGroup, kind)
	return reallyFresh
}

// sentOrReceivedDirectStatement records that the statement was exchanged with the validator.
func (k *knownBackedCandidate) sentOrReceivedDirectStatement(
	validator parachaintypes.ValidatorIndex,
	statementIndexInGroup uint,
	kind statementKind,
	received bool,
) {
	knowledge, ok := k.mutualKnowledge[validator]
	if !ok {
		return
	}

	if knowledge.remoteKnowledge != nil && knowledge.localKnowledge != nil {
		knowledge.remoteKnowledge.Set(statementIndexInGroup, kind)
		knowledge.localKnowledge.Set(statementIndexInGroup, kind)
	}

	if received && knowledge.receivedKnowledge != nil {
		knowledge.receivedKnowledge.Set(statementIndexInGroup, kind)
	}
}

// isPendingStatement returns true if manifests were exchanged with the validator
// and the validator does not know the given statement yet.
func (k *knownBackedCandidate) isPendingStatement(
	validator parachaintypes.ValidatorIndex,
	statementIndexInGroup uint,
	kind statementKind,
) bool {
	knowledge, ok := k.mutualKnowledge[validator]
	if !ok || knowledge.localKnowledge == nil || knowledge.remoteKnowledge == nil {
		return false
	}

	return !knowledge.remoteKnowledge.Contains(statementIndexInGroup, kind)
}

// pendingStatements returns the statements we know that the validator does not,
// or nil if manifests were not exchanged with the validator. The full local
// knowledge is used, as the local knowledge in the mutual knowledge may be outdated.
func (k *knownBackedCandidate) pendingStatements(validator parachaintypes.ValidatorIndex) *StatementFilter {
	knowledge, ok := k.mutualKnowledge[validator]
	if !ok || knowledge.localKnowledge == nil || knowledge.remoteKnowledge == nil {
		return nil
	}

	pending := k.localKnowledge.Clone()
	pending.MaskSeconded(knowledge.remoteKnowledge.secondedInGroup)
	pending.MaskValid(knowledge.remoteKnowledge.validatedInGroup)
	return pending
}
