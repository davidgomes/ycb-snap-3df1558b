// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package availabilitydistribution

import (
	"testing"

	"github.com/ChainSafe/gossamer/dot/parachain/network-bridge/messages"
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestFetchPoV(t *testing.T) {
	ctrl := gomock.NewController(t)
	netMock := NewMockNetwork(ctrl)
	blockStateMock := NewMockBlockState(ctrl)
	sessionCache := NewMockSessionCache(ctrl)
	rt := NewMockInstance(ctrl)

	netMock.EXPECT().RegisterRequestHandler(protocol.ID("req_chunk/2"), gomock.Any())
	netMock.EXPECT().RegisterRequestHandler(protocol.ID("req_pov/1"), gomock.Any())

	overseerCh := make(chan any)
	ad := NewAvailabilityDistribution(overseerCh, netMock, blockStateMock, sessionCache)

	relayParent := common.Hash{0x11}
	authority := parachaintypes.AuthorityDiscoveryID{0x42}
	pov := parachaintypes.PoV{BlockData: []byte{1, 2, 3, 4}}
	povHash, err := pov.Hash()
	require.NoError(t, err)

	msg := parachaintypes.AvailabilityDistributionMessageFetchPoV{
		RelayParent:   relayParent,
		FromValidator: 0,
		ParaID:        1000,
		CandidateHash: parachaintypes.CandidateHash{Value: common.Hash{0x22}},
		PovHash:       povHash,
		PovCh:         make(chan parachaintypes.OverseerFuncRes[parachaintypes.PoV]),
	}

	blockStateMock.EXPECT().GetRuntime(relayParent).Return(rt, nil)
	rt.EXPECT().Stop()
	sessionCache.EXPECT().GetSessionIndexForChild(relayParent, rt).Return(parachaintypes.SessionIndex(7), nil)
	rt.EXPECT().ParachainHostSessionInfo(parachaintypes.SessionIndex(7)).Return(&parachaintypes.SessionInfo{
		DiscoveryKeys: []parachaintypes.AuthorityDiscoveryID{authority},
	}, nil)

	errCh := make(chan error, 1)
	go func() {
		errCh <- ad.processAvailabilityDistributionMessageFetchPoV(msg)
	}()

	sent := (<-overseerCh).(messages.SendRequests)
	require.Len(t, sent.Requests, 1)
	require.Equal(t, authority, sent.Requests[0].Recipient)
	require.Equal(t, messages.PoVFetchingV1, sent.Requests[0].Payload.Protocol())

	response := &messages.PoVFetchingResponse{}
	require.NoError(t, response.SetValue(pov))
	sent.Requests[0].Result <- messages.ReqRespResult{Response: response}

	require.NoError(t, <-errCh)
	res := <-msg.PovCh
	require.NoError(t, res.Err)
	require.Equal(t, pov, res.Data)
}
