// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	"github.com/ChainSafe/gossamer/dot/parachain/network"
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	parachainutil "github.com/ChainSafe/gossamer/dot/parachain/util"
	"github.com/ChainSafe/gossamer/lib/common"
)

// peerState tracks a connected peer's view of the relay chain, along with the relay-parents
// implicitly allowed under that view.
type peerState struct {
	view            parachaintypes.View
	protocolVersion network.ValidationVersion
	// implicitView holds the relay-parents allowed under the peer's view heads, as far as our own
	// implicit view can tell. Heads we don't recognise contribute nothing.
	implicitView map[common.Hash]struct{}
	// discoveryIDs holds the peer's authority discovery IDs, nil when they are not known.
	discoveryIDs map[parachaintypes.AuthorityDiscoveryID]struct{}
}

// updateView replaces the peer's view, returning the implicit relay-parents which weren't previously
// part of it.
func (ps *peerState) updateView(newView parachaintypes.View, localImplicit parachainutil.ImplicitView) []common.Hash {
	nextImplicit := make(map[common.Hash]struct{})
	freshImplicit := make([]common.Hash, 0)

	for _, head := range newView.Heads {
		for _, relayParent := range localImplicit.KnownAllowedRelayParentsUnder(head, nil) {
			if _, seen := nextImplicit[relayParent]; seen {
				continue
			}
			nextImplicit[relayParent] = struct{}{}

			if _, known := ps.implicitView[relayParent]; !known {
				freshImplicit = append(freshImplicit, relayParent)
			}
		}
	}

	ps.view = newView
	ps.implicitView = nextImplicit
	return freshImplicit
}

// reconcileActiveLeaf adds the implicit relay-parents under a newly activated leaf to the peer's
// implicit view, provided the leaf is in the peer's view. Returns the relay-parents that were new to it.
func (ps *peerState) reconcileActiveLeaf(leafHash common.Hash, implicit []common.Hash) []common.Hash {
	if !ps.view.Contains(leafHash) {
		return []common.Hash{}
	}

	if ps.implicitView == nil {
		ps.implicitView = make(map[common.Hash]struct{}, len(implicit))
	}

	fresh := make([]common.Hash, 0, len(implicit))
	for _, relayParent := range implicit {
		if _, known := ps.implicitView[relayParent]; known {
			continue
		}
		ps.implicitView[relayParent] = struct{}{}
		fresh = append(fresh, relayParent)
	}
	return fresh
}

// knowsRelayParent reports whether the peer knows the relay-parent, either explicitly through its
// view or implicitly through one of its heads. Relay-parents implied by heads we don't recognise
// are not accounted for.
func (ps *peerState) knowsRelayParent(relayParent common.Hash) bool {
	_, implicit := ps.implicitView[relayParent]
	return implicit || ps.view.Contains(relayParent)
}

func (ps *peerState) isAuthority(authorityID parachaintypes.AuthorityDiscoveryID) bool {
	_, ok := ps.discoveryIDs[authorityID]
	return ok
}

// iterKnownDiscoveryIDs returns the peer's known authority discovery IDs in no particular order.
func (ps *peerState) iterKnownDiscoveryIDs() []parachaintypes.AuthorityDiscoveryID {
	ids := make([]parachaintypes.AuthorityDiscoveryID, 0, len(ps.discoveryIDs))
	for id := range ps.discoveryIDs {
		ids = append(ids, id)
	}
	return ids
}
