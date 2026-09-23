// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	"slices"

	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
)

// statementKind is the kind of statement a validator issued about a candidate.
type statementKind uint8

const (
	seconded statementKind = iota
	valid
)

// contains returns true if the filter has the statement of the given kind for the validator at
// the given index in the backing group. Out of bounds indices are never contained.
func (s *StatementFilter) contains(index uint, kind statementKind) bool {
	if s == nil {
		return false
	}

	var bit bool
	switch kind {
	case seconded:
		bit, _ = s.secondedInGroup.Get(index)
	case valid:
		bit, _ = s.validatedInGroup.Get(index)
	}

	return bit
}

// set marks the statement of the given kind for the validator at the given index in the backing
// group as known. Out of bounds indices are ignored.
func (s *StatementFilter) set(index uint, kind statementKind) {
	if s == nil {
		return
	}

	switch kind {
	case seconded:
		_ = s.secondedInGroup.Set(index, true)
	case valid:
		_ = s.validatedInGroup.Set(index, true)
	}
}

// mutualKnowledge is the knowledge that we have about a remote peer concerning a candidate,
// and that they have about us concerning the candidate.
type mutualKnowledge struct {
	// remoteKnowledge is the knowledge the remote peer has about the candidate, as far as we're aware.
	// Non-nil only if they have advertised, acknowledged, or requested the candidate.
	remoteKnowledge *StatementFilter
	// localKnowledge is the knowledge we have indicated to the remote peer about the candidate.
	// Non-nil only if we have advertised, acknowledged, or requested the candidate from them.
	localKnowledge *StatementFilter
	// receivedKnowledge is the knowledge the peer circulated to us. Unlike remoteKnowledge and
	// localKnowledge, which after the manifest exchange include both what we sent to the peer and
	// what we received from the peer, it only includes the statements the peer sent us directly.
	// It is used to determine whether the peer may still send us a statement directly.
	receivedKnowledge *StatementFilter
}

// knownBackedCandidate keeps track of the statements known about a candidate we have confirmed
// as having been backed, and of the knowledge exchanged about it with each validator.
type knownBackedCandidate struct {
	groupIndex parachaintypes.GroupIndex
	// localKnowledge contains all the statements we have about the candidate. It is always up to
	// date, as opposed to the localKnowledge of a mutualKnowledge, which is only what we indicated
	// to that peer.
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

func (c *knownBackedCandidate) mutualKnowledgeWith(validator parachaintypes.ValidatorIndex) *mutualKnowledge {
	if c.mutualKnowledge == nil {
		c.mutualKnowledge = make(map[parachaintypes.ValidatorIndex]*mutualKnowledge)
	}

	k, ok := c.mutualKnowledge[validator]
	if !ok {
		k = &mutualKnowledge{}
		c.mutualKnowledge[validator] = k
	}

	return k
}

func (c *knownBackedCandidate) hasReceivedManifestFrom(validator parachaintypes.ValidatorIndex) bool {
	k, ok := c.mutualKnowledge[validator]
	return ok && k.remoteKnowledge != nil
}

func (c *knownBackedCandidate) hasSentManifestTo(validator parachaintypes.ValidatorIndex) bool {
	k, ok := c.mutualKnowledge[validator]
	return ok && k.localKnowledge != nil
}

// manifestSentTo records that we sent a manifest with the given knowledge to the validator.
// It resets the knowledge received directly from the validator.
func (c *knownBackedCandidate) manifestSentTo(
	validator parachaintypes.ValidatorIndex,
	localKnowledge *StatementFilter,
) {
	k := c.mutualKnowledgeWith(validator)

	receivedKnowledge, err := NewStatementFilter(uint(localKnowledge.secondedInGroup.Len()), false)
	if err != nil {
		panic("group size is within the maximum allowed bitvec length. qed")
	}

	k.receivedKnowledge = receivedKnowledge
	k.localKnowledge = localKnowledge.clone()
}

// manifestReceivedFrom records that the validator sent us a manifest with the given knowledge.
func (c *knownBackedCandidate) manifestReceivedFrom(
	validator parachaintypes.ValidatorIndex,
	remoteKnowledge *StatementFilter,
) {
	c.mutualKnowledgeWith(validator).remoteKnowledge = remoteKnowledge.clone()
}

// directStatementSenders returns the validators that may send us the given statement directly.
// These are the validators we have exchanged manifests with which have not sent us the statement
// yet. Each one is mapped to whether it should already know the statement because we sent it
// to them.
func (c *knownBackedCandidate) directStatementSenders(
	groupIndex parachaintypes.GroupIndex,
	originatorIndexInGroup uint,
	kind statementKind,
) map[parachaintypes.ValidatorIndex]bool {
	senders := make(map[parachaintypes.ValidatorIndex]bool)
	if groupIndex != c.groupIndex {
		return senders
	}

	for validator, k := range c.mutualKnowledge {
		if k.remoteKnowledge == nil || k.receivedKnowledge == nil {
			continue
		}

		if k.receivedKnowledge.contains(originatorIndexInGroup, kind) {
			continue
		}

		senders[validator] = k.localKnowledge.contains(originatorIndexInGroup, kind)
	}

	return senders
}

// directStatementRecipients returns the validators, in ascending order, to which we may send the
// given statement directly. These are the validators we have exchanged manifests with which don't
// know the statement yet.
func (c *knownBackedCandidate) directStatementRecipients(
	groupIndex parachaintypes.GroupIndex,
	originatorIndexInGroup uint,
	kind statementKind,
) []parachaintypes.ValidatorIndex {
	if groupIndex != c.groupIndex {
		return nil
	}

	var recipients []parachaintypes.ValidatorIndex
	for validator, k := range c.mutualKnowledge {
		if k.localKnowledge == nil || k.remoteKnowledge == nil {
			continue
		}

		if k.remoteKnowledge.contains(originatorIndexInGroup, kind) {
			continue
		}

		recipients = append(recipients, validator)
	}

	slices.Sort(recipients)
	return recipients
}

// noteFreshStatement adds the statement to our local knowledge, returning true if it was not
// already known.
func (c *knownBackedCandidate) noteFreshStatement(statementIndexInGroup uint, kind statementKind) bool {
	reallyFresh := !c.localKnowledge.contains(statementIndexInGroup, kind)
	c.localKnowledge.set(statementIndexInGroup, kind)

	return reallyFresh
}

// sentOrReceivedDirectStatement records that the statement was exchanged directly with the
// validator, either sent by us or, if received is true, sent to us.
func (c *knownBackedCandidate) sentOrReceivedDirectStatement(
	validator parachaintypes.ValidatorIndex,
	statementIndexInGroup uint,
	kind statementKind,
	received bool,
) {
	k, ok := c.mutualKnowledge[validator]
	if !ok {
		return
	}

	if k.remoteKnowledge != nil && k.localKnowledge != nil {
		k.remoteKnowledge.set(statementIndexInGroup, kind)
		k.localKnowledge.set(statementIndexInGroup, kind)
	}

	if received {
		k.receivedKnowledge.set(statementIndexInGroup, kind)
	}
}

// isPendingStatement returns true if, after exchanging manifests with the validator, it doesn't
// know the statement.
func (c *knownBackedCandidate) isPendingStatement(
	validator parachaintypes.ValidatorIndex,
	statementIndexInGroup uint,
	kind statementKind,
) bool {
	k, ok := c.mutualKnowledge[validator]
	if !ok || k.localKnowledge == nil || k.remoteKnowledge == nil {
		return false
	}

	return !k.remoteKnowledge.contains(statementIndexInGroup, kind)
}

// pendingStatements returns the statements we know that the validator doesn't, or nil if we
// haven't exchanged manifests with it. Our full local knowledge is used rather than the knowledge
// we indicated to the validator, as the latter may be outdated.
func (c *knownBackedCandidate) pendingStatements(validator parachaintypes.ValidatorIndex) *StatementFilter {
	k, ok := c.mutualKnowledge[validator]
	if !ok || k.localKnowledge == nil || k.remoteKnowledge == nil {
		return nil
	}

	pending := c.localKnowledge.clone()
	pending.MaskSeconded(k.remoteKnowledge.secondedInGroup)
	pending.MaskValid(k.remoteKnowledge.validatedInGroup)

	return pending
}
