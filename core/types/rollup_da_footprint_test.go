package types

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"
)

func jovianL1AttributesDeposit(scalar uint16) *Transaction {
	data := make([]byte, JovianL1AttributesLen)
	copy(data, JovianL1AttributesSelector)
	binary.BigEndian.PutUint16(data[IsthmusL1AttributesLen:], scalar)
	return NewTx(&DepositTx{To: &L1BlockAddr, Data: data})
}

func isthmusL1AttributesDeposit() *Transaction {
	data := make([]byte, IsthmusL1AttributesLen)
	copy(data, IsthmusL1AttributesSelector)
	return NewTx(&DepositTx{To: &L1BlockAddr, Data: data})
}

func daFootprintTestTx(t *testing.T, dataLen int) *Transaction {
	t.Helper()
	data := make([]byte, dataLen)
	_, err := rand.Read(data)
	require.NoError(t, err)
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	to := common.HexToAddress("0xda")
	return MustSignNewTx(key, LatestSignerForChainID(big.NewInt(1)), &DynamicFeeTx{
		ChainID:   big.NewInt(1),
		To:        &to,
		Gas:       params.TxGas + uint64(dataLen)*params.TxCostFloorPerToken*params.TxTokenPerNonZeroByte,
		GasFeeCap: big.NewInt(1e9),
		GasTipCap: big.NewInt(1),
		Data:      data,
	})
}

func TestJovianL1AttributesSelectorMatchesSignature(t *testing.T) {
	require.Equal(t, crypto.Keccak256([]byte("setL1BlockValuesJovian()"))[:4], JovianL1AttributesSelector)
	require.Equal(t, crypto.Keccak256([]byte("setL1BlockValuesIsthmus()"))[:4], IsthmusL1AttributesSelector)
}

func TestExtractDAFootprintGasScalarFromL1Attributes(t *testing.T) {
	scalar, err := ExtractDAFootprintGasScalar(jovianL1AttributesDeposit(1234).Data())
	require.NoError(t, err)
	require.Equal(t, uint16(1234), scalar)

	_, err = ExtractDAFootprintGasScalar(isthmusL1AttributesDeposit().Data())
	require.Error(t, err, "Isthmus L1 attributes don't contain a DA footprint gas scalar")

	_, err = ExtractDAFootprintGasScalar(jovianL1AttributesDeposit(1234).Data()[:JovianL1AttributesLen-1])
	require.Error(t, err, "truncated Jovian L1 attributes")

	_, err = ExtractDAFootprintGasScalar(nil)
	require.Error(t, err)
}

func TestDAFootprintGasScalarOfBlockTxs(t *testing.T) {
	userTx := daFootprintTestTx(t, 10)
	truncated := NewTx(&DepositTx{To: &L1BlockAddr, Data: jovianL1AttributesDeposit(800).Data()[:JovianL1AttributesLen-1]})

	for _, tt := range []struct {
		name     string
		txs      []*Transaction
		expected uint16
		err      bool
	}{
		{name: "no-txs", expected: DAFootprintGasScalarDefault},
		{name: "no-deposit", txs: []*Transaction{userTx}, expected: DAFootprintGasScalarDefault},
		{name: "isthmus-l1-attributes", txs: []*Transaction{isthmusL1AttributesDeposit(), userTx}, expected: DAFootprintGasScalarDefault},
		{name: "jovian-zero-scalar", txs: []*Transaction{jovianL1AttributesDeposit(0), userTx}, expected: DAFootprintGasScalarDefault},
		{name: "jovian-scalar", txs: []*Transaction{jovianL1AttributesDeposit(800), userTx}, expected: 800},
		{name: "jovian-truncated", txs: []*Transaction{truncated, userTx}, err: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scalar, err := DAFootprintGasScalar(tt.txs)
			if tt.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, scalar)
		})
	}
}

func TestCalcDAFootprintOfBlockTxs(t *testing.T) {
	const scalar = 800
	var (
		deposit   = jovianL1AttributesDeposit(scalar)
		minSizeTx = daFootprintTestTx(t, 0)
		largeTx   = daFootprintTestTx(t, 1000)
	)
	require.Zero(t, deposit.DAFootprint(scalar), "deposits have no DA footprint")
	require.Equal(t, MinTransactionSize.Uint64()*scalar, minSizeTx.DAFootprint(scalar))
	largeFootprint := largeTx.RollupCostData().EstimatedDASize().Uint64() * scalar
	require.Greater(t, largeFootprint, minSizeTx.DAFootprint(scalar))
	require.Equal(t, largeFootprint, largeTx.DAFootprint(scalar))

	daFootprint, err := CalcDAFootprint([]*Transaction{deposit, minSizeTx, largeTx})
	require.NoError(t, err)
	require.Equal(t, minSizeTx.DAFootprint(scalar)+largeFootprint, daFootprint)

	daFootprint, err = CalcDAFootprint([]*Transaction{deposit, NewTx(&DepositTx{})})
	require.NoError(t, err)
	require.Zero(t, daFootprint, "deposit-only blocks have no DA footprint")

	daFootprint, err = CalcDAFootprint(nil)
	require.NoError(t, err)
	require.Zero(t, daFootprint)
}
