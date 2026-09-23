// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	"testing"

	"github.com/ChainSafe/gossamer/dot/parachain/network"
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	parachainutil "github.com/ChainSafe/gossamer/dot/parachain/util"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

// stubImplicitView answers KnownAllowedRelayParentsUnder from a fixed table keyed by block hash.
type stubImplicitView struct {
	allowedUnder map[common.Hash][]common.Hash
}

func (*stubImplicitView) ActivateLeaf(common.Hash, chan<- any) error { return nil }

func (*stubImplicitView) DeactivateLeaf(common.Hash) []common.Hash { return nil }

func (*stubImplicitView) AllAllowedRelayParents() []common.Hash { return nil }

func (s *stubImplicitView) KnownAllowedRelayParentsUnder(
	blockHash common.Hash, _ *parachaintypes.ParaID,
) []common.Hash {
	return s.allowedUnder[blockHash]
}

func TestPeerStateUpdateViewTracksImplicitRelayParents(t *testing.T) {
	t.Parallel()

	leafA, a1, a2 := common.Hash{0xa0}, common.Hash{0xa1}, common.Hash{0xa2}
	leafB := common.Hash{0xb0}
	unknownHead := common.Hash{0xff}

	localImplicit := &stubImplicitView{allowedUnder: map[common.Hash][]common.Hash{
		leafA: {leafA, a1, a2},
		leafB: {leafB, a1},
	}}

	ps := &peerState{protocolVersion: network.ValidationVersionV3}

	viewA := parachaintypes.View{Heads: []common.Hash{leafA}, FinalizedNumber: 1}
	fresh := ps.updateView(viewA, localImplicit)
	require.ElementsMatch(t, []common.Hash{leafA, a1, a2}, fresh)
	require.Equal(t, viewA, ps.view)
	for _, relayParent := range []common.Hash{leafA, a1, a2} {
		require.True(t, ps.knowsRelayParent(relayParent))
	}

	// a1 is shared by both leaves, so only leafB is fresh.
	fresh = ps.updateView(parachaintypes.View{Heads: []common.Hash{leafA, leafB}}, localImplicit)
	require.Equal(t, []common.Hash{leafB}, fresh)

	// Dropping leafA from the view forgets the relay-parents only it implied.
	fresh = ps.updateView(parachaintypes.View{Heads: []common.Hash{leafB}}, localImplicit)
	require.Empty(t, fresh)
	require.True(t, ps.knowsRelayParent(leafB))
	require.True(t, ps.knowsRelayParent(a1))
	require.False(t, ps.knowsRelayParent(leafA))
	require.False(t, ps.knowsRelayParent(a2))

	// A head our implicit view doesn't recognise is only known explicitly.
	fresh = ps.updateView(parachaintypes.View{Heads: []common.Hash{unknownHead}}, localImplicit)
	require.Empty(t, fresh)
	require.Empty(t, ps.implicitView)
	require.True(t, ps.knowsRelayParent(unknownHead))
	require.False(t, ps.knowsRelayParent(leafB))
	require.False(t, ps.knowsRelayParent(a1))
}

func TestPeerStateUpdateViewWithHeadBehindOurLeaves(t *testing.T) {
	t.Parallel()

	leaf, a1, a2, a3 := common.Hash{0x10}, common.Hash{0x11}, common.Hash{0x12}, common.Hash{0x13}

	localImplicit := parachainutil.NewBackingImplicitView(nil, nil)
	localImplicit.ActivateLeafFromProspectiveParachains(
		&parachainutil.BlockInfoProspectiveParachains{Hash: leaf, ParentHash: a1, Number: 3},
		[]*parachainutil.BlockInfoProspectiveParachains{
			{Hash: a1, ParentHash: a2, Number: 2},
			{Hash: a2, ParentHash: a3, Number: 1},
		},
	)

	ps := &peerState{}
	fresh := ps.updateView(parachaintypes.View{Heads: []common.Hash{leaf}}, localImplicit)
	require.ElementsMatch(t, []common.Hash{a1, a2}, fresh)

	// a1 is stored in our view as an ancestor but was never an active leaf, so it implies nothing.
	fresh = ps.updateView(parachaintypes.View{Heads: []common.Hash{a1}}, localImplicit)
	require.Empty(t, fresh)
	require.True(t, ps.knowsRelayParent(a1))
	require.False(t, ps.knowsRelayParent(a2))
	require.False(t, ps.knowsRelayParent(leaf))
}

func TestPeerStateReconcileActiveLeafOnlyForLeavesInView(t *testing.T) {
	t.Parallel()

	leaf, p1, p2, p3 := common.Hash{0x20}, common.Hash{0x21}, common.Hash{0x22}, common.Hash{0x23}
	otherLeaf := common.Hash{0x30}

	ps := &peerState{view: parachaintypes.View{Heads: []common.Hash{leaf}}}

	fresh := ps.reconcileActiveLeaf(otherLeaf, []common.Hash{otherLeaf, p1})
	require.Empty(t, fresh)
	require.False(t, ps.knowsRelayParent(p1))

	fresh = ps.reconcileActiveLeaf(leaf, []common.Hash{leaf, p1, p2})
	require.Equal(t, []common.Hash{leaf, p1, p2}, fresh)
	require.True(t, ps.knowsRelayParent(p2))

	fresh = ps.reconcileActiveLeaf(leaf, []common.Hash{leaf, p1, p3, p3})
	require.Equal(t, []common.Hash{p3}, fresh)
	require.True(t, ps.knowsRelayParent(p3))
}

func TestPeerStateDiscoveryIDs(t *testing.T) {
	t.Parallel()

	alice := parachaintypes.AuthorityDiscoveryID{0x01}
	bob := parachaintypes.AuthorityDiscoveryID{0x02}
	charlie := parachaintypes.AuthorityDiscoveryID{0x03}

	unknownIDs := &peerState{}
	require.False(t, unknownIDs.isAuthority(alice))
	require.Empty(t, unknownIDs.iterKnownDiscoveryIDs())

	knownIDs := &peerState{discoveryIDs: map[parachaintypes.AuthorityDiscoveryID]struct{}{
		alice: {},
		bob:   {},
	}}
	require.True(t, knownIDs.isAuthority(alice))
	require.True(t, knownIDs.isAuthority(bob))
	require.False(t, knownIDs.isAuthority(charlie))
	require.ElementsMatch(t, []parachaintypes.AuthorityDiscoveryID{alice, bob}, knownIDs.iterKnownDiscoveryIDs())
}
