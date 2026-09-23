// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package network

// ValidationVersion is the validation protocol version advertised by a peer.
type ValidationVersion byte

const (
	ValidationVersionV1 ValidationVersion = iota + 1
	ValidationVersionV2
	ValidationVersionV3
)
