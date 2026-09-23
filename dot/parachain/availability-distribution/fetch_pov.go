// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package availabilitydistribution

import (
	"errors"
	"fmt"
	"time"

	networkbridgemessages "github.com/ChainSafe/gossamer/dot/parachain/network-bridge/messages"
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/ChainSafe/gossamer/lib/common"
)

var (
	errNotAValidator         = errors.New("not a validator in this session")
	errInvalidValidatorIndex = errors.New("invalid validator index")
	errRequestCancelled      = errors.New("request cancelled")
	errRequestTimeout        = errors.New("request timed out")
	errUnexpectedResponse    = errors.New("unexpected response type")
	errNoSuchPoV             = errors.New("validator does not have the requested PoV")
	errUnexpectedPoV         = errors.New("PoV hash does not match the expected hash")
)

type povResult = parachaintypes.OverseerFuncRes[parachaintypes.PoV]

// getAuthorityDiscoveryID looks up the authority discovery ID of the validator with the given index in the session
// of the children of the given relay parent.
func (ad *AvailabilityDistribution) getAuthorityDiscoveryID(
	relayParent common.Hash,
	validatorIndex parachaintypes.ValidatorIndex,
) (parachaintypes.AuthorityDiscoveryID, error) {
	rt, err := ad.blockState.GetRuntime(relayParent)
	if err != nil {
		return parachaintypes.AuthorityDiscoveryID{}, fmt.Errorf("instantiating runtime for block %s: %w",
			relayParent, err)
	}

	sessionIndex, err := ad.sessionCache.GetSessionIndexForChild(relayParent, rt)
	if err != nil {
		return parachaintypes.AuthorityDiscoveryID{}, fmt.Errorf("getting session index for block %s: %w",
			relayParent, err)
	}

	sessionInfo, err := ad.sessionCache.GetSessionInfo(sessionIndex, rt)
	if err != nil {
		return parachaintypes.AuthorityDiscoveryID{}, fmt.Errorf("getting session info for session %d: %w",
			sessionIndex, err)
	}

	if sessionInfo == nil {
		return parachaintypes.AuthorityDiscoveryID{}, fmt.Errorf("%w: session %d", errNotAValidator, sessionIndex)
	}

	if int(validatorIndex) >= len(sessionInfo.DiscoveryKeys) {
		return parachaintypes.AuthorityDiscoveryID{}, fmt.Errorf("%w: %d, number of validators in session %d: %d",
			errInvalidValidatorIndex, validatorIndex, sessionIndex, len(sessionInfo.DiscoveryKeys))
	}

	return sessionInfo.DiscoveryKeys[validatorIndex], nil
}

// fetchPoV requests the PoV described by msg from the given validator via the network bridge and delivers the
// result on msg.PovCh.
func fetchPoV(
	subsystemToOverseer chan<- any,
	authorityID parachaintypes.AuthorityDiscoveryID,
	msg parachaintypes.AvailabilityDistributionMessageFetchPoV,
) {
	request := networkbridgemessages.NewOutgoingRequest(
		authorityID,
		&networkbridgemessages.PoVFetchingRequest{CandidateHash: msg.CandidateHash},
	)

	subsystemToOverseer <- networkbridgemessages.SendRequests{
		Requests:       []*networkbridgemessages.OutgoingRequest{request},
		IfDisconnected: networkbridgemessages.ImmediateError,
	}

	pov, err := receivePoV(request, msg.PovHash)
	if err != nil {
		logger.Warnf("fetching PoV %s for candidate %s of para %d from validator %d: %s",
			msg.PovHash, msg.CandidateHash, msg.ParaID, msg.FromValidator, err)
		sendPoVResult(msg.PovCh, povResult{Err: fmt.Errorf("%w: %w", parachaintypes.ErrFetchPoV, err)})
		return
	}

	sendPoVResult(msg.PovCh, povResult{Data: pov})
}

// receivePoV waits for the response to a PoV fetching request and checks that the PoV matches the expected hash.
func receivePoV(
	request *networkbridgemessages.OutgoingRequest,
	expectedHash common.Hash,
) (parachaintypes.PoV, error) {
	var result networkbridgemessages.ReqRespResult

	select {
	case res, ok := <-request.Result:
		if !ok {
			return parachaintypes.PoV{}, errRequestCancelled
		}
		result = res
	case <-time.After(parachaintypes.SubsystemRequestTimeout):
		request.Cancel()
		return parachaintypes.PoV{}, errRequestTimeout
	}

	if result.Error != nil {
		return parachaintypes.PoV{}, result.Error
	}

	response, ok := result.Response.(*networkbridgemessages.PoVFetchingResponse)
	if !ok || response == nil {
		return parachaintypes.PoV{}, fmt.Errorf("%w: %T", errUnexpectedResponse, result.Response)
	}

	value, err := response.Value()
	if err != nil {
		return parachaintypes.PoV{}, fmt.Errorf("getting PoV fetching response value: %w", err)
	}

	switch value := value.(type) {
	case parachaintypes.PoV:
		hash, err := value.Hash()
		if err != nil {
			return parachaintypes.PoV{}, fmt.Errorf("hashing PoV: %w", err)
		}

		if hash != expectedHash {
			return parachaintypes.PoV{}, fmt.Errorf("%w: expected %s, got %s", errUnexpectedPoV, expectedHash, hash)
		}

		return value, nil
	case parachaintypes.NoSuchPoV:
		return parachaintypes.PoV{}, errNoSuchPoV
	default:
		return parachaintypes.PoV{}, fmt.Errorf("%w: %T", errUnexpectedResponse, value)
	}
}

// sendPoVResult delivers exactly one result on ch and closes it afterwards. The requester stops listening after a
// timeout, so give up if nobody receives the result in time instead of blocking forever.
func sendPoVResult(ch chan povResult, result povResult) {
	if ch == nil {
		return
	}
	defer close(ch)

	select {
	case ch <- result:
	case <-time.After(parachaintypes.SubsystemRequestTimeout):
		logger.Debugf("no receiver for PoV fetching result (err: %v)", result.Err)
	}
}
