// Copyright 2023 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package overseer

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	types "github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var runtimeTestVersion = runtime.Version{
	SpecName:         []byte{1},
	ImplName:         []byte{2},
	AuthoringVersion: 3,
	SpecVersion:      4,
	ImplVersion:      5,
	APIItems: []runtime.APIItem{{
		Name: common.MustBlake2b8([]byte("ParachainHost")),
		Ver:  1,
	}},
	TransactionVersion: 7,
}

type TestSubsystem struct {
	name             string
	finalizedCounter atomic.Int32
	importedCounter  atomic.Int32
}

func (s *TestSubsystem) Name() parachaintypes.SubSystemName {
	return parachaintypes.SubSystemName(s.name)
}

func (s *TestSubsystem) Run(ctx context.Context, overseerToSubSystem <-chan any) {
	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				fmt.Printf("%s ctx error: %v\n", s.name, err)
			}
			fmt.Printf("%s overseer stopping\n", s.name)
			return
		case overseerSignal := <-overseerToSubSystem:
			fmt.Printf("%s received from overseer %v\n", s.name, overseerSignal)
			incrementCounters(overseerSignal, &s.finalizedCounter, &s.importedCounter)
		}
	}
}

func (s *TestSubsystem) ProcessActiveLeavesUpdateSignal(update parachaintypes.ActiveLeavesUpdateSignal) error {
	fmt.Printf("%s ProcessActiveLeavesUpdateSignal\n", s.name)
	return nil
}

func (s *TestSubsystem) ProcessBlockFinalizedSignal(signal parachaintypes.BlockFinalizedSignal) error {
	fmt.Printf("%s ProcessActiveLeavesUpdateSignal\n", s.name)
	return nil
}

func (s *TestSubsystem) String() parachaintypes.SubSystemName {
	return parachaintypes.SubSystemName(s.name)
}

func (s *TestSubsystem) Stop() {}

func TestHandleBlockEvents(t *testing.T) {
	ctrl := gomock.NewController(t)

	blockState := NewMockBlockState(ctrl)

	finalizedNotifierChan := make(chan *types.FinalisationInfo)
	importedBlockNotiferChan := make(chan *types.Block)

	blockState.EXPECT().GetFinalisedNotifierChannel().Return(finalizedNotifierChan)
	blockState.EXPECT().GetImportedBlockNotifierChannel().Return(importedBlockNotiferChan)
	blockState.EXPECT().FreeFinalisedNotifierChannel(finalizedNotifierChan)
	blockState.EXPECT().FreeImportedBlockNotifierChannel(importedBlockNotiferChan)

	runtimeInstanceMock := NewMockInstance(ctrl)
	runtimeInstanceMock.EXPECT().Version().
		Return(runtimeTestVersion, nil)

	blockState.EXPECT().
		GetRuntime(gomock.AssignableToTypeOf(common.Hash{})).
		Return(runtimeInstanceMock, nil)

	overseer := NewOverseer(blockState)

	require.NotNil(t, overseer)

	subSystem1 := &TestSubsystem{
		name:             "subSystem1",
		finalizedCounter: atomic.Int32{},
		importedCounter:  atomic.Int32{},
	}

	subSystem2 := &TestSubsystem{
		name:             "subSystem2",
		finalizedCounter: atomic.Int32{},
		importedCounter:  atomic.Int32{},
	}

	overseer.RegisterSubsystem(subSystem1)
	overseer.RegisterSubsystem(subSystem2)

	err := overseer.Start()
	require.NoError(t, err)
	finalizedNotifierChan <- &types.FinalisationInfo{}
	importedBlockNotiferChan <- &types.Block{}

	// let subsystems run for a bit
	time.Sleep(4000 * time.Millisecond)

	err = overseer.Stop()
	require.NoError(t, err)

	require.Equal(t, int32(1), subSystem1.finalizedCounter.Load())
	require.Equal(t, int32(1), subSystem1.importedCounter.Load())
	require.Equal(t, int32(1), subSystem2.finalizedCounter.Load())
	require.Equal(t, int32(1), subSystem2.importedCounter.Load())
}

type recordingSubsystem struct {
	name     parachaintypes.SubSystemName
	received chan any
}

func (s *recordingSubsystem) Name() parachaintypes.SubSystemName {
	return s.name
}

func (s *recordingSubsystem) Run(ctx context.Context, overseerToSubSystem <-chan any) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-overseerToSubSystem:
			select {
			case s.received <- msg:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (*recordingSubsystem) ProcessActiveLeavesUpdateSignal(parachaintypes.ActiveLeavesUpdateSignal) error {
	return nil
}

func (*recordingSubsystem) ProcessBlockFinalizedSignal(parachaintypes.BlockFinalizedSignal) error {
	return nil
}

func (*recordingSubsystem) Stop() {}

func TestProcessMessagesRoutesFetchPoV(t *testing.T) {
	ctrl := gomock.NewController(t)

	blockState := NewMockBlockState(ctrl)
	blockState.EXPECT().GetImportedBlockNotifierChannel().Return(make(chan *types.Block))
	blockState.EXPECT().GetFinalisedNotifierChannel().Return(make(chan *types.FinalisationInfo))
	blockState.EXPECT().FreeImportedBlockNotifierChannel(gomock.Any())
	blockState.EXPECT().FreeFinalisedNotifierChannel(gomock.Any())

	overseer := NewOverseer(blockState)

	availabilityDistribution := &recordingSubsystem{
		name:     parachaintypes.AvailabilityDistribution,
		received: make(chan any, 1),
	}
	overseer.RegisterSubsystem(availabilityDistribution)

	require.NoError(t, overseer.Start())
	t.Cleanup(func() {
		require.NoError(t, overseer.Stop())
	})

	send := func(msg any) {
		t.Helper()
		select {
		case overseer.SubsystemsToOverseer <- msg:
		case <-time.After(time.Second):
			t.Fatalf("overseer is blocked, could not send %T", msg)
		}
	}

	// Neither an unknown message nor a message for an unregistered subsystem may block the overseer.
	send(struct{}{})
	send(parachaintypes.DistributeBitfield{})

	fetchPoV := parachaintypes.AvailabilityDistributionMessageFetchPoV{
		RelayParent:   common.Hash{0x01},
		FromValidator: parachaintypes.ValidatorIndex(2),
		CandidateHash: parachaintypes.CandidateHash{Value: common.Hash{0x03}},
		PovHash:       common.Hash{0x04},
		PovCh:         make(chan parachaintypes.OverseerFuncRes[parachaintypes.PoV]),
	}
	send(fetchPoV)

	select {
	case msg := <-availabilityDistribution.received:
		require.Equal(t, fetchPoV, msg)
	case <-time.After(time.Second):
		t.Fatal("FetchPoV message was not routed to availability distribution")
	}
}

func incrementCounters(msg any, finalizedCounter *atomic.Int32, importedCounter *atomic.Int32) {
	if msg == nil {
		return
	}

	switch msg.(type) {
	case parachaintypes.BlockFinalizedSignal:
		finalizedCounter.Add(1)
	case parachaintypes.ActiveLeavesUpdateSignal:
		importedCounter.Add(1)
	}
}
