// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	parachainutil "github.com/ChainSafe/gossamer/dot/parachain/util"
	"github.com/ChainSafe/gossamer/lib/common"
)

// peerState tracks a peer's view of the relay chain and the relay parents implied by it.
type peerState struct {
	view parachaintypes.View
	// implicitView contains every relay parent allowed under the heads of the peer's view,
	// as known by our local implicit view.
	implicitView map[common.Hash]struct{}
	// discoveryIDs is nil when the peer is not known to be an authority.
	discoveryIDs map[parachaintypes.AuthorityDiscoveryID]struct{}
}

func newPeerState(discoveryIDs map[parachaintypes.AuthorityDiscoveryID]struct{}) *peerState {
	return &peerState{
		implicitView: make(map[common.Hash]struct{}),
		discoveryIDs: discoveryIDs,
	}
}

// updateView replaces the peer's view and returns the implicit relay parents
// which weren't previously part of the peer's implicit view.
func (p *peerState) updateView(newView parachaintypes.View, localImplicit parachainutil.ImplicitView) []common.Hash {
	nextImplicit := make(map[common.Hash]struct{})
	var freshImplicit []common.Hash

	for _, head := range newView.Heads {
		for _, relayParent := range localImplicit.KnownAllowedRelayParentsUnder(head, nil) {
			if _, seen := nextImplicit[relayParent]; seen {
				continue
			}
			nextImplicit[relayParent] = struct{}{}

			if _, known := p.implicitView[relayParent]; !known {
				freshImplicit = append(freshImplicit, relayParent)
			}
		}
	}

	p.view = newView
	p.implicitView = nextImplicit
	return freshImplicit
}

// reconcileActiveLeaf adds the implicit relay parents of an active leaf to the peer's
// implicit view, if the leaf is part of the peer's view. Returns the newly added relay parents.
func (p *peerState) reconcileActiveLeaf(leafHash common.Hash, implicit []common.Hash) []common.Hash {
	if !p.view.Contains(leafHash) {
		return nil
	}

	if p.implicitView == nil {
		p.implicitView = make(map[common.Hash]struct{})
	}

	added := make([]common.Hash, 0, len(implicit))
	for _, relayParent := range implicit {
		if _, known := p.implicitView[relayParent]; known {
			continue
		}
		p.implicitView[relayParent] = struct{}{}
		added = append(added, relayParent)
	}
	return added
}

// knowsRelayParent returns true if the peer's view implies the given relay parent.
func (p *peerState) knowsRelayParent(relayParent common.Hash) bool {
	_, ok := p.implicitView[relayParent]
	return ok
}

// isAuthority returns true if the peer is known to use the given authority discovery ID.
func (p *peerState) isAuthority(authorityID parachaintypes.AuthorityDiscoveryID) bool {
	_, ok := p.discoveryIDs[authorityID]
	return ok
}

// iterKnownDiscoveryIDs returns the authority discovery IDs known for the peer, in no particular order.
func (p *peerState) iterKnownDiscoveryIDs() []parachaintypes.AuthorityDiscoveryID {
	ids := make([]parachaintypes.AuthorityDiscoveryID, 0, len(p.discoveryIDs))
	for id := range p.discoveryIDs {
		ids = append(ids, id)
	}
	return ids
}
