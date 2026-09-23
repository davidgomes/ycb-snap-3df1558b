// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package availabilitydistribution

import (
	"errors"
	"testing"
	"time"

	networkbridgemessages "github.com/ChainSafe/gossamer/dot/parachain/network-bridge/messages"
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestProcessAvailabilityDistributionMessageFetchPoV(t *testing.T) {
	var (
		relayParent    = common.Hash{0x42}
		sessionIndex   = parachaintypes.SessionIndex(3)
		validatorIndex = parachaintypes.ValidatorIndex(1)
		candidateHash  = parachaintypes.CandidateHash{Value: common.Hash{0x01}}
		authorityID    = parachaintypes.AuthorityDiscoveryID{0xBB}
		sessionInfo    = &SessionInfo{
			SessionIndex:  sessionIndex,
			DiscoveryKeys: []parachaintypes.AuthorityDiscoveryID{{0xAA}, authorityID, {0xCC}},
		}

		pov      = parachaintypes.PoV{BlockData: []byte{0x01, 0x02, 0x03}}
		otherPoV = parachaintypes.PoV{BlockData: []byte{0x04, 0x05, 0x06}}
	)

	povHash, err := pov.Hash()
	require.NoError(t, err)

	errRuntime := errors.New("no runtime")
	errSessionInfo := errors.New("no session info")

	type fetchPoVMessage = parachaintypes.AvailabilityDistributionMessageFetchPoV

	newMessage := func(fromValidator parachaintypes.ValidatorIndex) fetchPoVMessage {
		return fetchPoVMessage{
			RelayParent:   relayParent,
			FromValidator: fromValidator,
			ParaID:        parachaintypes.ParaID(7),
			CandidateHash: candidateHash,
			PovHash:       povHash,
			PovCh:         make(chan parachaintypes.OverseerFuncRes[parachaintypes.PoV]),
		}
	}

	type mocks struct {
		blockState   *MockBlockState
		runtime      *MockInstance
		sessionCache *MockSessionCache
	}

	setup := func(t *testing.T) (*AvailabilityDistribution, chan any, mocks) {
		t.Helper()

		ctrl := gomock.NewController(t)
		netMock := NewMockNetwork(ctrl)
		netMock.EXPECT().RegisterRequestHandler(protocol.ID("req_chunk/2"), gomock.Any())
		netMock.EXPECT().RegisterRequestHandler(protocol.ID("req_pov/1"), gomock.Any())

		m := mocks{
			blockState:   NewMockBlockState(ctrl),
			runtime:      NewMockInstance(ctrl),
			sessionCache: NewMockSessionCache(ctrl),
		}

		overseerCh := make(chan any)
		ad := NewAvailabilityDistribution(overseerCh, netMock, m.blockState, m.sessionCache)
		return ad, overseerCh, m
	}

	expectSessionInfo := func(m mocks, info *SessionInfo) {
		m.blockState.EXPECT().GetRuntime(relayParent).Return(m.runtime, nil)
		m.sessionCache.EXPECT().GetSessionIndexForChild(relayParent, m.runtime).Return(sessionIndex, nil)
		m.sessionCache.EXPECT().GetSessionInfo(sessionIndex, m.runtime).Return(info, nil)
	}

	receiveRequest := func(t *testing.T, overseerCh chan any) *networkbridgemessages.OutgoingRequest {
		t.Helper()

		select {
		case msg := <-overseerCh:
			sendRequests, ok := msg.(networkbridgemessages.SendRequests)
			require.True(t, ok, "unexpected message type %T", msg)
			require.Equal(t, networkbridgemessages.ImmediateError, sendRequests.IfDisconnected)
			require.Len(t, sendRequests.Requests, 1)

			request := sendRequests.Requests[0]
			require.Equal(t, authorityID, request.Recipient)

			povRequest, ok := request.Payload.(*networkbridgemessages.PoVFetchingRequest)
			require.True(t, ok, "unexpected payload type %T", request.Payload)
			require.Equal(t, candidateHash, povRequest.CandidateHash)
			return request
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for PoV fetching request")
			return nil
		}
	}

	receiveResult := func(
		t *testing.T,
		povCh chan parachaintypes.OverseerFuncRes[parachaintypes.PoV],
	) parachaintypes.OverseerFuncRes[parachaintypes.PoV] {
		t.Helper()

		select {
		case res, ok := <-povCh:
			require.True(t, ok, "PoV channel closed without a result")

			_, ok = <-povCh
			require.False(t, ok, "PoV channel should be closed after the result was sent")
			return res
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for PoV fetching result")
			return parachaintypes.OverseerFuncRes[parachaintypes.PoV]{}
		}
	}

	povResponse := func(t *testing.T, value any) *networkbridgemessages.PoVFetchingResponse {
		t.Helper()

		response := &networkbridgemessages.PoVFetchingResponse{}
		require.NoError(t, response.SetValue(value))
		return response
	}

	t.Run("pov_received", func(t *testing.T) {
		ad, overseerCh, m := setup(t)
		expectSessionInfo(m, sessionInfo)
		msg := newMessage(validatorIndex)

		require.NoError(t, ad.processMessage(msg))

		request := receiveRequest(t, overseerCh)
		request.Result <- networkbridgemessages.ReqRespResult{Response: povResponse(t, pov)}

		res := receiveResult(t, msg.PovCh)
		require.NoError(t, res.Err)
		require.Equal(t, pov, res.Data)
	})

	failureTestCases := []struct {
		description string
		respond     func(t *testing.T, request *networkbridgemessages.OutgoingRequest)
		expectedErr error
	}{
		{
			description: "no_such_pov",
			respond: func(t *testing.T, request *networkbridgemessages.OutgoingRequest) {
				request.Result <- networkbridgemessages.ReqRespResult{Response: povResponse(t, parachaintypes.NoSuchPoV{})}
			},
			expectedErr: errNoSuchPoV,
		},
		{
			description: "pov_hash_mismatch",
			respond: func(t *testing.T, request *networkbridgemessages.OutgoingRequest) {
				request.Result <- networkbridgemessages.ReqRespResult{Response: povResponse(t, otherPoV)}
			},
			expectedErr: errUnexpectedPoV,
		},
		{
			description: "network_error",
			respond: func(t *testing.T, request *networkbridgemessages.OutgoingRequest) {
				request.Result <- networkbridgemessages.ReqRespResult{Error: errors.New("network failure")}
			},
			expectedErr: parachaintypes.ErrFetchPoV,
		},
		{
			description: "unexpected_response_type",
			respond: func(t *testing.T, request *networkbridgemessages.OutgoingRequest) {
				request.Result <- networkbridgemessages.ReqRespResult{Response: &networkbridgemessages.ChunkFetchingResponse{}}
			},
			expectedErr: errUnexpectedResponse,
		},
		{
			description: "request_cancelled",
			respond: func(t *testing.T, request *networkbridgemessages.OutgoingRequest) {
				close(request.Result)
			},
			expectedErr: errRequestCancelled,
		},
	}

	for _, tc := range failureTestCases {
		t.Run(tc.description, func(t *testing.T) {
			ad, overseerCh, m := setup(t)
			expectSessionInfo(m, sessionInfo)
			msg := newMessage(validatorIndex)

			require.NoError(t, ad.processAvailabilityDistributionMessageFetchPoV(msg))

			tc.respond(t, receiveRequest(t, overseerCh))

			res := receiveResult(t, msg.PovCh)
			require.ErrorIs(t, res.Err, parachaintypes.ErrFetchPoV)
			require.ErrorIs(t, res.Err, tc.expectedErr)
			require.Equal(t, parachaintypes.PoV{}, res.Data)
		})
	}

	lookupFailureTestCases := []struct {
		description   string
		fromValidator parachaintypes.ValidatorIndex
		setupMocks    func(m mocks)
		expectedErr   error
	}{
		{
			description:   "invalid_validator_index",
			fromValidator: parachaintypes.ValidatorIndex(len(sessionInfo.DiscoveryKeys)),
			setupMocks:    func(m mocks) { expectSessionInfo(m, sessionInfo) },
			expectedErr:   errInvalidValidatorIndex,
		},
		{
			description:   "not_a_validator",
			fromValidator: validatorIndex,
			setupMocks:    func(m mocks) { expectSessionInfo(m, nil) },
			expectedErr:   errNotAValidator,
		},
		{
			description:   "runtime_unavailable",
			fromValidator: validatorIndex,
			setupMocks: func(m mocks) {
				m.blockState.EXPECT().GetRuntime(relayParent).Return(nil, errRuntime)
			},
			expectedErr: errRuntime,
		},
		{
			description:   "session_info_unavailable",
			fromValidator: validatorIndex,
			setupMocks: func(m mocks) {
				m.blockState.EXPECT().GetRuntime(relayParent).Return(m.runtime, nil)
				m.sessionCache.EXPECT().GetSessionIndexForChild(relayParent, m.runtime).Return(sessionIndex, nil)
				m.sessionCache.EXPECT().GetSessionInfo(sessionIndex, m.runtime).Return(nil, errSessionInfo)
			},
			expectedErr: errSessionInfo,
		},
	}

	for _, tc := range lookupFailureTestCases {
		t.Run(tc.description, func(t *testing.T) {
			ad, overseerCh, m := setup(t)
			tc.setupMocks(m)
			msg := newMessage(tc.fromValidator)

			err := ad.processAvailabilityDistributionMessageFetchPoV(msg)
			require.ErrorIs(t, err, tc.expectedErr)

			res := receiveResult(t, msg.PovCh)
			require.ErrorIs(t, res.Err, parachaintypes.ErrFetchPoV)
			require.ErrorIs(t, res.Err, tc.expectedErr)

			select {
			case msg := <-overseerCh:
				t.Fatalf("no request should be sent, got %T", msg)
			default:
			}
		})
	}
}

func TestSendPoVResultNilChannel(t *testing.T) {
	require.NotPanics(t, func() {
		sendPoVResult(nil, povResult{Err: parachaintypes.ErrFetchPoV})
	})
}
