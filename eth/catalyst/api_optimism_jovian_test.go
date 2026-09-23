package catalyst

import (
	"testing"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"
)

func TestCheckOptimismPayloadAttributesMinBaseFee(t *testing.T) {
	zero := uint64(0)
	holocene := &params.ChainConfig{
		Optimism:     &params.OptimismConfig{},
		CanyonTime:   &zero,
		HoloceneTime: &zero,
	}
	jovian := *holocene
	jovian.JovianTime = &zero

	attrs := func(minBaseFee *uint64) *engine.PayloadAttributes {
		return &engine.PayloadAttributes{
			GasLimit:      new(uint64),
			EIP1559Params: []byte{0, 1, 2, 3, 4, 5, 6, 7},
			MinBaseFee:    minBaseFee,
		}
	}

	require.NoError(t, checkOptimismPayloadAttributes(attrs(nil), holocene))
	require.EqualError(t, checkOptimismPayloadAttributes(attrs(&zero), holocene), "non-nil minBaseFee pre-Jovian")
	require.NoError(t, checkOptimismPayloadAttributes(attrs(&zero), &jovian))
	require.EqualError(t, checkOptimismPayloadAttributes(attrs(nil), &jovian), "nil minBaseFee post-Jovian")
}
