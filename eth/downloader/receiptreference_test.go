package downloader

import (
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeCorrection is a helper function to create a slice of receipts and a slice of corrected receipts
func makeCorrection(bn uint64, cid uint64, ns []uint64, ty []uint8) (types.Receipts, types.Receipts) {
	receipts := make(types.Receipts, len(ns))
	correctedReceipts := make(types.Receipts, len(ns))
	transactions := make(types.Transactions, len(ns))
	for i := range ns {
		receipts[i] = &types.Receipt{Type: ty[i], DepositNonce: &ns[i]}
		correctedReceipts[i] = &types.Receipt{Type: ty[i], DepositNonce: &ns[i]}
		transactions[i] = types.NewTx(&types.DepositTx{})
	}

	correctedReceipts = correctReceipts(correctedReceipts, transactions, bn, cid)

	return receipts, correctedReceipts
}

func TestCorrectReceipts(t *testing.T) {
	type testcase struct {
		blockNum uint64
		chainID  uint64
		nonces   []uint64
		txTypes  []uint8
		validate func(types.Receipts, types.Receipts)
	}

	// Tests use the real reference data, so block numbers and chainIDs are selected for different test cases
	testcases := []testcase{
		// Test case 1: No receipts
		{
			blockNum: 6825767,
			chainID:  420,
			nonces:   []uint64{},
			txTypes:  []uint8{},
			validate: func(receipts types.Receipts, correctedReceipts types.Receipts) {
				assert.Empty(t, correctedReceipts)
			},
		},
		// Test case 2: No deposits
		{
			blockNum: 6825767,
			chainID:  420,
			nonces:   []uint64{1, 2, 3},
			txTypes:  []uint8{1, 1, 1},
			validate: func(receipts types.Receipts, correctedReceipts types.Receipts) {
				assert.Equal(t, receipts, correctedReceipts)
			},
		},
		// Test case 3: all deposits with no correction
		{
			blockNum: 8835769,
			chainID:  420,
			nonces:   []uint64{78756, 78757, 78758, 78759, 78760, 78761, 78762, 78763, 78764},
			txTypes:  []uint8{126, 126, 126, 126, 126, 126, 126, 126, 126},
			validate: func(receipts types.Receipts, correctedReceipts types.Receipts) {
				assert.Equal(t, receipts, correctedReceipts)
			},
		},
		// Test case 4: all deposits with a correction
		{
			blockNum: 8835769,
			chainID:  420,
			nonces:   []uint64{78756, 78757, 78758, 12345, 78760, 78761, 78762, 78763, 78764},
			txTypes:  []uint8{126, 126, 126, 126, 126, 126, 126, 126, 126},
			validate: func(receipts types.Receipts, correctedReceipts types.Receipts) {
				assert.NotEqual(t, receipts[3], correctedReceipts[3])
				for i := range receipts {
					if i != 3 {
						assert.Equal(t, receipts[i], correctedReceipts[i])
					}
				}
			},
		},
		// Test case 5: deposits with several corrections and non-deposits
		{
			blockNum: 8835769,
			chainID:  420,
			nonces:   []uint64{0, 1, 2, 78759, 78760, 78761, 6, 78763, 78764, 9, 10, 11},
			txTypes:  []uint8{126, 126, 126, 126, 126, 126, 126, 126, 126, 1, 1, 1},
			validate: func(receipts types.Receipts, correctedReceipts types.Receipts) {
				// indexes 0, 1, 2, 6 were modified
				// indexes 9, 10, 11 were added too, but they are not user deposits
				assert.NotEqual(t, receipts[0], correctedReceipts[0])
				assert.NotEqual(t, receipts[1], correctedReceipts[1])
				assert.NotEqual(t, receipts[2], correctedReceipts[2])
				assert.NotEqual(t, receipts[6], correctedReceipts[6])
				for i := range receipts {
					if i != 0 && i != 1 && i != 2 && i != 6 {
						assert.Equal(t, receipts[i], correctedReceipts[i])
					}
				}
			},
		},
	}

	for _, tc := range testcases {
		receipts, correctedReceipts := makeCorrection(tc.blockNum, tc.chainID, tc.nonces, tc.txTypes)
		tc.validate(receipts, correctedReceipts)
	}
}

func TestCorrectReceiptsRLP(t *testing.T) {
	type testcase struct {
		blockNum uint64
		chainID  uint64
		nonces   []uint64
		txTypes  []uint8
		validate func(rlp.RawValue, rlp.RawValue)
	}

	// Tests use the real reference data, so block numbers and chainIDs are selected for different test cases
	testcases := []testcase{
		// Test case 1: No receipts
		{
			blockNum: 6825767,
			chainID:  420,
			nonces:   []uint64{},
			txTypes:  []uint8{},
			validate: func(originalRLP rlp.RawValue, correctedRLP rlp.RawValue) {
				assert.Empty(t, decodeStorage(t, originalRLP))
				assert.Empty(t, decodeStorage(t, correctedRLP))
			},
		},
		// Test case 2: No deposits
		{
			blockNum: 6825767,
			chainID:  420,
			nonces:   []uint64{1, 2, 3},
			txTypes:  []uint8{1, 1, 1},
			validate: func(originalRLP rlp.RawValue, correctedRLP rlp.RawValue) {
				original, corrected := decodeStorage(t, originalRLP), decodeStorage(t, correctedRLP)
				assert.Equal(t, original, corrected)
			},
		},
		// Test case 3: all deposits with no correction
		{
			blockNum: 8835769,
			chainID:  420,
			nonces:   []uint64{78756, 78757, 78758, 78759, 78760, 78761, 78762, 78763, 78764},
			txTypes:  []uint8{126, 126, 126, 126, 126, 126, 126, 126, 126},
			validate: func(originalRLP rlp.RawValue, correctedRLP rlp.RawValue) {
				original, corrected := decodeStorage(t, originalRLP), decodeStorage(t, correctedRLP)
				assert.Equal(t, original, corrected)
			},
		},
		// Test case 4: all deposits with a correction
		{
			blockNum: 8835769,
			chainID:  420,
			nonces:   []uint64{78756, 78757, 78758, 12345, 78760, 78761, 78762, 78763, 78764},
			txTypes:  []uint8{126, 126, 126, 126, 126, 126, 126, 126, 126},
			validate: func(originalRLP rlp.RawValue, correctedRLP rlp.RawValue) {
				original, corrected := decodeStorage(t, originalRLP), decodeStorage(t, correctedRLP)
				assert.NotEqual(t, original[3], corrected[3])
				for i := range original {
					if i != 3 {
						assert.Equal(t, original[i], corrected[i])
					}
				}
			},
		},
		// Test case 5: deposits with several corrections and non-deposits
		{
			blockNum: 8835769,
			chainID:  420,
			nonces:   []uint64{0, 1, 2, 78759, 78760, 78761, 6, 78763, 78764, 9, 10, 11},
			txTypes:  []uint8{126, 126, 126, 126, 126, 126, 126, 126, 126, 1, 1, 1},
			validate: func(originalRLP rlp.RawValue, correctedRLP rlp.RawValue) {
				original, corrected := decodeStorage(t, originalRLP), decodeStorage(t, correctedRLP)
				// indexes 0, 1, 2, 6 were modified
				// indexes 9, 10, 11 were added too, but they are not user deposits
				assert.NotEqual(t, original[0], corrected[0])
				assert.NotEqual(t, original[1], corrected[1])
				assert.NotEqual(t, original[2], corrected[2])
				assert.NotEqual(t, original[6], corrected[6])
				for i := range original {
					if i != 0 && i != 1 && i != 2 && i != 6 {
						assert.Equal(t, original[i], corrected[i])
					}
				}
			},
		},
	}

	for _, tc := range testcases {
		receipts, transactions := makeStorageTestData(tc.nonces, tc.txTypes)

		// The downloader receives receipts in the storage encoding.
		originalRLP, err := rlp.EncodeToBytes(receipts)
		require.NoError(t, err)

		correctedRLP := correctReceiptsRLP(originalRLP, transactions, tc.blockNum, tc.chainID)

		tc.validate(originalRLP, correctedRLP)
	}
}

// makeStorageTestData creates storage receipts and matching transactions. Deposit
// receipts only carry a deposit nonce, like in the real encoding.
func makeStorageTestData(nonces []uint64, txTypes []uint8) ([]*types.ReceiptForStorage, types.Transactions) {
	receipts := make([]*types.ReceiptForStorage, len(nonces))
	transactions := make(types.Transactions, len(nonces))
	for i := range nonces {
		r := &types.ReceiptForStorage{
			Type:              txTypes[i],
			Status:            types.ReceiptStatusSuccessful,
			CumulativeGasUsed: uint64(21000 * (i + 1)),
			Logs:              []*types.Log{},
		}
		if txTypes[i] == types.DepositTxType {
			r.DepositNonce = &nonces[i]
			transactions[i] = types.NewTx(&types.DepositTx{})
		} else {
			transactions[i] = types.NewTx(&types.AccessListTx{Nonce: nonces[i]})
		}
		receipts[i] = r
	}
	return receipts, transactions
}

func decodeStorage(t *testing.T, enc rlp.RawValue) []*types.ReceiptForStorage {
	t.Helper()
	var receipts []*types.ReceiptForStorage
	require.NoError(t, rlp.DecodeBytes(enc, &receipts))
	return receipts
}

// TestCorrectReceiptsRLPNetworkStorageEncoding runs the correction on receipts that went
// through the same encoding path as receipts delivered to the downloader by the eth protocol.
func TestCorrectReceiptsRLPNetworkStorageEncoding(t *testing.T) {
	const (
		blockNum = 8835769
		chainID  = 420
	)
	nonces := []uint64{0, 1, 2, 78759, 78760, 78761, 6, 78763, 78764, 9, 10, 11}
	txTypes := []uint8{126, 126, 126, 126, 126, 126, 126, 126, 126, 1, 1, 1}
	want := []uint64{78756, 78757, 78758, 78759, 78760, 78761, 78762, 78763, 78764}

	storageReceipts, transactions := makeStorageTestData(nonces, txTypes)
	receipts := make([]*types.Receipt, len(storageReceipts))
	for i, r := range storageReceipts {
		receipts[i] = (*types.Receipt)(r)
	}

	for name, originalRLP := range map[string]rlp.RawValue{
		"eth68": eth.NewReceiptList68(receipts).EncodeForStorage(),
		"eth69": eth.NewReceiptList69(receipts).EncodeForStorage(),
	} {
		t.Run(name, func(t *testing.T) {
			correctedRLP := correctReceiptsRLP(originalRLP, transactions, blockNum, chainID)
			require.NotEqual(t, originalRLP, correctedRLP)

			original, corrected := decodeStorage(t, originalRLP), decodeStorage(t, correctedRLP)
			require.Len(t, corrected, len(nonces))
			for i, n := range want {
				require.NotNil(t, corrected[i].DepositNonce)
				require.Equal(t, n, *corrected[i].DepositNonce, "receipt %d", i)
			}
			for i := len(want); i < len(nonces); i++ {
				require.Equal(t, original[i], corrected[i], "receipt %d", i)
			}
		})
	}
}

func TestCorrectReceiptsRLPUnchanged(t *testing.T) {
	nonces := []uint64{78756, 78757, 78758, 78759, 78760, 78761, 78762, 78763, 78764}
	txTypes := []uint8{126, 126, 126, 126, 126, 126, 126, 126, 126}
	receipts, transactions := makeStorageTestData(nonces, txTypes)
	enc, err := rlp.EncodeToBytes(receipts)
	require.NoError(t, err)
	originalRLP := rlp.RawValue(enc)

	// correct nonces, out of range block, unknown chain and malformed input are passed through
	require.Equal(t, originalRLP, correctReceiptsRLP(originalRLP, transactions, 8835769, 420))
	require.Equal(t, originalRLP, correctReceiptsRLP(originalRLP, transactions, 1, 420))
	require.Equal(t, originalRLP, correctReceiptsRLP(originalRLP, transactions, 8835769, 12345))
	malformed := rlp.RawValue{0xc2, 0x01}
	require.Equal(t, malformed, correctReceiptsRLP(malformed, transactions, 8835769, 420))
	require.Equal(t, originalRLP, correctReceiptsRLP(originalRLP, transactions[:3], 8835769, 420))
}
