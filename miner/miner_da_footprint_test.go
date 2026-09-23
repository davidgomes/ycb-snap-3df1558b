package miner

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"
)

func TestJovianDAFootprintBlockLimit(t *testing.T) {
	const gasLimit = 1_000_000

	t.Run("jovian", func(t *testing.T) {
		cfg := *params.OptimismTestConfig
		cfg.JovianTime = new(uint64)
		block, receipts, poolTxs := mineDAFootprintBlock(t, &cfg, gasLimit, 30)

		included := len(block.Transactions()) - 1 // excluding the deposit
		require.Positive(t, included)
		require.Less(t, included, len(poolTxs), "DA footprint limit should exclude some txs")

		var txGas, daFootprint uint64
		for i, tx := range block.Transactions() {
			txGas += receipts[i].GasUsed
			if !tx.IsDepositTx() {
				daFootprint += tx.RollupCostData().EstimatedDASize().Uint64() * params.DAFootprintGasScalar
			}
		}
		require.Less(t, txGas, daFootprint)
		require.Equal(t, daFootprint, block.GasUsed())
		require.LessOrEqual(t, daFootprint, block.GasLimit())

		next := poolTxs[included].RollupCostData().EstimatedDASize().Uint64() * params.DAFootprintGasScalar
		require.Greater(t, daFootprint+next, block.GasLimit(), "next tx would have fit")
	})

	t.Run("isthmus", func(t *testing.T) {
		block, receipts, poolTxs := mineDAFootprintBlock(t, params.OptimismTestConfig, gasLimit, 30)

		require.Len(t, block.Transactions(), len(poolTxs)+1)
		var txGas uint64
		for _, r := range receipts {
			txGas += r.GasUsed
		}
		require.Equal(t, txGas, block.GasUsed())
	})
}

// mineDAFootprintBlock builds a block on top of genesis from a deposit and numTxs
// DA-heavy pool transactions and imports it into the chain.
func mineDAFootprintBlock(t *testing.T, cfg *params.ChainConfig, gasLimit, numTxs uint64) (*types.Block, []*types.Receipt, types.Transactions) {
	engine := beacon.New(ethash.NewFaker())
	gspec := &core.Genesis{
		Config:    cfg,
		Alloc:     types.GenesisAlloc{testBankAddress: {Balance: testBankFunds}},
		ExtraData: eip1559.EncodeOptimismExtraData(cfg, 0, 250, 6, new(uint64)),
	}
	chain, err := core.NewBlockChain(rawdb.NewMemoryDatabase(), gspec, engine, &core.BlockChainConfig{ArchiveMode: true})
	require.NoError(t, err)
	defer chain.Stop()
	pool, err := txpool.New(testTxPoolConfig.PriceLimit, chain, []txpool.SubPool{legacypool.New(testTxPoolConfig, chain)}, nil)
	require.NoError(t, err)
	defer pool.Close()
	w := New(&testWorkerBackend{chain: chain, txPool: pool, genesis: gspec}, testConfig, engine)

	signer := types.LatestSigner(cfg)
	txs := make(types.Transactions, 0, numTxs)
	for nonce := uint64(0); nonce < numTxs; nonce++ {
		data := make([]byte, 100)
		_, err := rand.Read(data)
		require.NoError(t, err)
		txs = append(txs, types.MustSignNewTx(testBankKey, signer, &types.DynamicFeeTx{
			ChainID:   cfg.ChainID,
			Nonce:     nonce,
			To:        &testUserAddress,
			Gas:       params.TxGas + uint64(len(data))*params.TxTokenPerNonZeroByte*params.TxCostFloorPerToken,
			GasFeeCap: big.NewInt(params.InitialBaseFee),
			GasTipCap: big.NewInt(params.InitialBaseFee),
			Data:      data,
		}))
	}
	for _, err := range pool.Add(txs, true) {
		require.NoError(t, err)
	}

	parent := chain.CurrentBlock()
	genParams := &generateParams{
		parentHash:    parent.Hash(),
		timestamp:     parent.Time + 2,
		withdrawals:   types.Withdrawals{},
		beaconRoot:    new(common.Hash),
		gasLimit:      &gasLimit,
		txs:           types.Transactions{types.NewTx(&types.DepositTx{})},
		eip1559Params: eip1559.EncodeHolocene1559Params(250, 6),
	}
	if cfg.IsJovian(genParams.timestamp) {
		genParams.minBaseFee = new(uint64)
	}
	r := w.generateWork(genParams, false)
	require.NoError(t, r.err)

	_, err = chain.InsertChain(types.Blocks{r.block})
	require.NoError(t, err, "block import failed")
	return r.block, r.receipts, txs
}
