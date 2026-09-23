package downloader

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"
)

var (
	opGoerliChainID = big.NewInt(420)

	// opGoerliDepositBlock and its user deposit nonces are taken from the embedded reference data.
	opGoerliDepositBlock  = uint64(8835769)
	opGoerliDepositNonces = []uint64{78756, 78757, 78758, 78759, 78760, 78761, 78762, 78763, 78764}
)

// makeDepositBlock returns the receipts and transactions of a block with an L1 info deposit,
// one user deposit per given nonce and a trailing non-deposit transaction.
func makeDepositBlock(l1InfoNonce uint64, userNonces []*uint64) (types.Receipts, types.Transactions) {
	var (
		receipts types.Receipts
		txs      types.Transactions
	)
	add := func(tx *types.Transaction, nonce *uint64) {
		i := byte(len(txs))
		receipts = append(receipts, &types.Receipt{
			Type:              tx.Type(),
			Status:            types.ReceiptStatusSuccessful,
			CumulativeGasUsed: 21000 * uint64(i+1),
			Logs:              []*types.Log{{Address: common.Address{i}, Topics: []common.Hash{{i}}, Data: []byte{i}}},
			DepositNonce:      nonce,
		})
		txs = append(txs, tx)
	}
	add(types.NewTx(&types.DepositTx{From: systemAddress}), &l1InfoNonce)
	for _, nonce := range userNonces {
		add(types.NewTx(&types.DepositTx{From: common.Address{0xaa}}), nonce)
	}
	add(types.NewTx(&types.DynamicFeeTx{}), nil)
	return receipts, txs
}

// deliverReceipts sends receipts through a network encoding round trip and converts them
// to the storage encoding, like the eth protocol handler does before handing them to the downloader.
func deliverReceipts[T any, PT interface {
	*T
	EncodeForStorage() rlp.RawValue
}](t *testing.T, sent PT) rlp.RawValue {
	enc, err := rlp.EncodeToBytes(sent)
	require.NoError(t, err)
	received := PT(new(T))
	require.NoError(t, rlp.DecodeBytes(enc, received))
	return received.EncodeForStorage()
}

func TestCorrectReceiptsFromNetwork(t *testing.T) {
	protocols := map[string]func(*testing.T, types.Receipts) rlp.RawValue{
		"eth68": func(t *testing.T, rs types.Receipts) rlp.RawValue {
			return deliverReceipts(t, eth.NewReceiptList68(rs))
		},
		"eth69": func(t *testing.T, rs types.Receipts) rlp.RawValue {
			return deliverReceipts(t, eth.NewReceiptList69(rs))
		},
	}
	for name, deliver := range protocols {
		t.Run(name, func(t *testing.T) {
			const l1InfoNonce = 424242
			nonces := make([]*uint64, len(opGoerliDepositNonces))
			for i, n := range opGoerliDepositNonces {
				nonces[i] = &n
			}
			wrong := uint64(12345)
			nonces[2], nonces[5] = &wrong, nil
			receipts, txs := makeDepositBlock(l1InfoNonce, nonces)

			out := correctReceipts(deliver(t, receipts), txs, opGoerliDepositBlock, opGoerliChainID)

			var corrected []*types.ReceiptForStorage
			require.NoError(t, rlp.DecodeBytes(out, &corrected))
			require.Len(t, corrected, len(receipts))
			for i, r := range corrected {
				require.Equal(t, receipts[i].Status, r.Status, "receipt %d", i)
				require.Equal(t, receipts[i].CumulativeGasUsed, r.CumulativeGasUsed, "receipt %d", i)
				require.Equal(t, receipts[i].Logs, r.Logs, "receipt %d", i)
			}
			require.Equal(t, uint64(l1InfoNonce), *corrected[0].DepositNonce, "system deposit must not be corrected")
			for i, want := range opGoerliDepositNonces {
				require.NotNil(t, corrected[i+1].DepositNonce, "user deposit %d", i)
				require.Equal(t, want, *corrected[i+1].DepositNonce, "user deposit %d", i)
			}
			require.Nil(t, corrected[len(corrected)-1].DepositNonce)
		})
	}
}

func TestCorrectReceiptsKeepsInputWithoutCorrection(t *testing.T) {
	nonces := make([]*uint64, len(opGoerliDepositNonces))
	for i, n := range opGoerliDepositNonces {
		nonces[i] = &n
	}
	receipts, txs := makeDepositBlock(0, nonces)
	in := deliverReceipts(t, eth.NewReceiptList69(receipts))

	require.Equal(t, in, correctReceipts(in, txs, opGoerliDepositBlock, opGoerliChainID))
}
