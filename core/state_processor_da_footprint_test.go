package core

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"
)

func daFootprintTestL1InfoDeposit(scalar uint16) *types.Transaction {
	data := make([]byte, types.JovianL1AttributesLen)
	copy(data, types.JovianL1AttributesSelector)
	binary.BigEndian.PutUint16(data[types.IsthmusL1AttributesLen:], scalar)
	return types.NewTx(&types.DepositTx{
		From: common.HexToAddress("0xdeaddeaddeaddeaddeaddeaddeaddeaddead0001"),
		To:   &types.L1BlockAddr,
		Data: data,
	})
}

func TestBlockGasUsedDAFootprint(t *testing.T) {
	jovian := *params.OptimismTestConfig
	jovian.JovianTime = new(uint64)

	key, _ := crypto.GenerateKey()
	signer := types.LatestSigner(&jovian)
	userTx := types.MustSignNewTx(key, signer, &types.DynamicFeeTx{ChainID: jovian.ChainID, Gas: params.TxGas})
	txs := []*types.Transaction{daFootprintTestL1InfoDeposit(1000), userTx}
	daFootprint := userTx.DAFootprint(1000)

	header := &types.Header{GasLimit: 10 * daFootprint}
	gasUsed, err := blockGasUsed(params.OptimismTestConfig, header, txs, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), gasUsed, "pre-Jovian gas used is the tx gas used")

	gasUsed, err = blockGasUsed(&jovian, header, txs, 1)
	require.NoError(t, err)
	require.Equal(t, daFootprint, gasUsed, "DA footprint should dominate")

	gasUsed, err = blockGasUsed(&jovian, header, txs, daFootprint+1)
	require.NoError(t, err)
	require.Equal(t, daFootprint+1, gasUsed, "tx gas used should dominate")

	header.GasLimit = daFootprint - 1
	_, err = blockGasUsed(&jovian, header, txs, 1)
	require.ErrorIs(t, err, ErrDAFootprintLimitExceeded)
}

func TestProcessJovianBlockDAFootprint(t *testing.T) {
	const scalar = 1000
	var (
		key, _     = crypto.GenerateKey()
		addr       = crypto.PubkeyToAddress(key.PublicKey)
		to         = common.HexToAddress("0xda")
		minBaseFee = uint64(0)
		cfg        = *params.OptimismTestConfig
	)
	cfg.JovianTime = new(uint64)
	gspec := &Genesis{
		Config:    &cfg,
		GasLimit:  30_000_000,
		BaseFee:   big.NewInt(params.InitialBaseFee),
		ExtraData: eip1559.EncodeOptimismExtraData(&cfg, 0, 250, 6, &minBaseFee),
		Alloc:     types.GenesisAlloc{addr: {Balance: big.NewInt(params.Ether)}},
	}
	engine := beacon.New(ethash.NewFaker())
	signer := types.LatestSigner(&cfg)
	_, blocks, receipts := GenerateChainWithGenesis(gspec, engine, 1, func(i int, b *BlockGen) {
		b.SetExtra(eip1559.EncodeOptimismExtraData(&cfg, b.header.Time, 250, 6, &minBaseFee))
		b.AddTx(daFootprintTestL1InfoDeposit(scalar))
		for nonce := uint64(0); nonce < 5; nonce++ {
			data := make([]byte, 200)
			_, err := rand.Read(data)
			require.NoError(t, err)
			b.AddTx(types.MustSignNewTx(key, signer, &types.DynamicFeeTx{
				ChainID:   cfg.ChainID,
				Nonce:     nonce,
				To:        &to,
				Gas:       params.TxGas + uint64(len(data))*params.TxCostFloorPerToken*params.TxTokenPerNonZeroByte,
				GasFeeCap: big.NewInt(2 * params.InitialBaseFee),
				GasTipCap: big.NewInt(1),
				Data:      data,
			}))
		}
	})

	block := blocks[0]
	daFootprint, err := types.CalcDAFootprint(block.Transactions())
	require.NoError(t, err)
	txGasUsed := receipts[0][len(receipts[0])-1].CumulativeGasUsed
	require.Greater(t, daFootprint, txGasUsed)
	require.Equal(t, daFootprint, block.GasUsed())

	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), gspec, engine, nil)
	require.NoError(t, err)
	defer chain.Stop()
	_, err = chain.InsertChain(blocks)
	require.NoError(t, err)

	// The base fee of the next block reacts to the DA footprint.
	expectedBaseFee := eip1559.CalcBaseFee(&cfg, block.Header(), block.Time()+2)
	withTxGasOnly := types.CopyHeader(block.Header())
	withTxGasOnly.GasUsed = txGasUsed
	require.Equal(t, 1, expectedBaseFee.Cmp(eip1559.CalcBaseFee(&cfg, withTxGasOnly, block.Time()+2)))
}
