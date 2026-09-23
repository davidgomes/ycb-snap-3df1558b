// Copyright 2025 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package network

// ValidationVersion is a supported version of the validation peer-set protocol.
type ValidationVersion uint32

// Validation protocol versions, numbered as on the wire.
const (
	ValidationVersionV1 ValidationVersion = iota + 1
	ValidationVersionV2
	ValidationVersionV3
)
