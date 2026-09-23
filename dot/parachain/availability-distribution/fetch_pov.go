// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package availabilitydistribution

import (
	"errors"
	"fmt"

	networkbridgemessages "github.com/ChainSafe/gossamer/dot/parachain/network-bridge/messages"
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
)

// processAvailabilityDistributionMessageFetchPoV fetches a PoV from the validator that
// advertised it and delivers the result on the request channel.
//
// The handler must answer PovCh. Candidate backing waits on that channel and otherwise
// times out, which aborts validation of the candidate.
func (ad *AvailabilityDistribution) processAvailabilityDistributionMessageFetchPoV(
	msg parachaintypes.AvailabilityDistributionMessageFetchPoV,
) error {
	go ad.fetchPoV(msg)
	return nil
}

func (ad *AvailabilityDistribution) fetchPoV(msg parachaintypes.AvailabilityDistributionMessageFetchPoV) {
	pov, err := ad.requestPoV(msg)
	if err != nil {
		logger.Warnf(
			"failed to fetch PoV: para=%d relay_parent=%s candidate=%s pov_hash=%s from_validator=%d: %s",
			msg.ParaID,
			msg.RelayParent,
			msg.CandidateHash,
			msg.PovHash,
			msg.FromValidator,
			err,
		)
		respondFetchPoV(msg.PovCh, parachaintypes.OverseerFuncRes[parachaintypes.PoV]{
			Err: fmt.Errorf("%w: %s", parachaintypes.ErrFetchPoV, err),
		})
		return
	}

	respondFetchPoV(msg.PovCh, parachaintypes.OverseerFuncRes[parachaintypes.PoV]{Data: pov})
}

func (ad *AvailabilityDistribution) requestPoV(
	msg parachaintypes.AvailabilityDistributionMessageFetchPoV,
) (parachaintypes.PoV, error) {
	rt, err := ad.blockState.GetRuntime(msg.RelayParent)
	if err != nil {
		return parachaintypes.PoV{}, fmt.Errorf("getting runtime for relay parent: %w", err)
	}
	defer rt.Stop()

	sessionIndex, err := ad.sessionCache.GetSessionIndexForChild(msg.RelayParent, rt)
	if err != nil {
		return parachaintypes.PoV{}, fmt.Errorf("getting session index: %w", err)
	}

	sessionInfo, err := rt.ParachainHostSessionInfo(sessionIndex)
	if err != nil {
		return parachaintypes.PoV{}, fmt.Errorf("getting session info: %w", err)
	}
	if sessionInfo == nil {
		return parachaintypes.PoV{}, errors.New("session info unavailable")
	}

	validatorIndex := int(msg.FromValidator)
	if validatorIndex < 0 || validatorIndex >= len(sessionInfo.DiscoveryKeys) {
		return parachaintypes.PoV{}, fmt.Errorf("invalid validator index %d", msg.FromValidator)
	}
	authorityID := sessionInfo.DiscoveryKeys[validatorIndex]

	request := networkbridgemessages.NewOutgoingRequest(
		authorityID,
		&networkbridgemessages.PoVFetchingRequest{CandidateHash: msg.CandidateHash},
	)
	ad.subSystemToOverseer <- networkbridgemessages.SendRequests{
		Requests:       []*networkbridgemessages.OutgoingRequest{request},
		IfDisconnected: networkbridgemessages.ImmediateError,
	}

	// Result is closed without a value when the request is cancelled before a response arrives.
	// Do not also select on Done: the sender cancels the request after delivering the result,
	// so both channels can be ready for a successful fetch.
	result, ok := <-request.Result
	if !ok {
		return parachaintypes.PoV{}, errors.New("PoV request cancelled")
	}

	if result.Error != nil {
		return parachaintypes.PoV{}, result.Error
	}

	response, ok := result.Response.(*networkbridgemessages.PoVFetchingResponse)
	if !ok || response == nil {
		return parachaintypes.PoV{}, fmt.Errorf("unexpected PoV response type %T", result.Response)
	}

	value, err := response.Value()
	if err != nil {
		return parachaintypes.PoV{}, fmt.Errorf("reading PoV response: %w", err)
	}

	pov, ok := value.(parachaintypes.PoV)
	if !ok {
		return parachaintypes.PoV{}, errors.New("validator does not have the PoV")
	}

	gotHash, err := pov.Hash()
	if err != nil {
		return parachaintypes.PoV{}, fmt.Errorf("hashing PoV: %w", err)
	}
	if gotHash != msg.PovHash {
		return parachaintypes.PoV{}, fmt.Errorf("PoV hash mismatch: got %s", gotHash)
	}

	return pov, nil
}

func respondFetchPoV(
	ch chan parachaintypes.OverseerFuncRes[parachaintypes.PoV],
	res parachaintypes.OverseerFuncRes[parachaintypes.PoV],
) {
	if ch == nil {
		return
	}
	ch <- res
	close(ch)
}
