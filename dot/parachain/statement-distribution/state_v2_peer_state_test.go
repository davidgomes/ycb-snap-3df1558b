// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package statementdistribution

import (
	"testing"

	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

type peerStateFakeImplicitView struct {
	allowed map[common.Hash][]common.Hash
}

func (*peerStateFakeImplicitView) Leaves() []common.Hash                      { return nil }
func (*peerStateFakeImplicitView) AllAllowedRelayParents() []common.Hash      { return nil }
func (*peerStateFakeImplicitView) ActivateLeaf(common.Hash, chan<- any) error { return nil }
func (*peerStateFakeImplicitView) DeactivateLeaf(common.Hash) []common.Hash   { return nil }
func (f *peerStateFakeImplicitView) KnownAllowedRelayParentsUnder(
	blockHash common.Hash, _ *parachaintypes.ParaID) []common.Hash {
	return f.allowed[blockHash]
}

func TestPeerStateUpdateViewAndReconcile(t *testing.T) {
	a, b, c, d := common.Hash{1}, common.Hash{2}, common.Hash{3}, common.Hash{4}
	local := &peerStateFakeImplicitView{allowed: map[common.Hash][]common.Hash{
		a: {a, b},
		c: {c, b},
	}}

	ps := newPeerState(nil)
	fresh := ps.updateView(parachaintypes.View{Heads: []common.Hash{a}}, local)
	require.ElementsMatch(t, []common.Hash{a, b}, fresh)
	require.True(t, ps.knowsRelayParent(b))

	fresh = ps.updateView(parachaintypes.View{Heads: []common.Hash{a, c}}, local)
	require.Equal(t, []common.Hash{c}, fresh)

	fresh = ps.updateView(parachaintypes.View{Heads: []common.Hash{c}}, local)
	require.Empty(t, fresh)
	require.False(t, ps.knowsRelayParent(a))

	require.Empty(t, ps.reconcileActiveLeaf(a, []common.Hash{a, d}))
	require.Equal(t, []common.Hash{d}, ps.reconcileActiveLeaf(c, []common.Hash{c, d}))
	require.True(t, ps.knowsRelayParent(d))
}

func TestPeerStateDiscoveryIDs(t *testing.T) {
	id := parachaintypes.AuthorityDiscoveryID{7}
	require.False(t, newPeerState(nil).isAuthority(id))
	require.Empty(t, newPeerState(nil).iterKnownDiscoveryIDs())

	ps := newPeerState(map[parachaintypes.AuthorityDiscoveryID]struct{}{id: {}})
	require.True(t, ps.isAuthority(id))
	require.False(t, ps.isAuthority(parachaintypes.AuthorityDiscoveryID{8}))
	require.Equal(t, []parachaintypes.AuthorityDiscoveryID{id}, ps.iterKnownDiscoveryIDs())
}
