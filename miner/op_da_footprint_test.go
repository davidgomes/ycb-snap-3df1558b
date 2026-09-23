package miner

import (
	"crypto/ecdsa"
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

const jovianDAFootprintTestGasLimit = uint64(1e6)

func jovianDAFootprintTestConfig() *params.ChainConfig {
	cfg := *params.OptimismTestConfig
	zero := uint64(0)
	cfg.JovianTime = &zero
	return &cfg
}

func jovianDAFootprintTestParams(b *testWorkerBackend, txs types.Transactions, noTxs bool) *generateParams {
	gasLimit := jovianDAFootprintTestGasLimit
	return &generateParams{
		parentHash:    b.chain.CurrentBlock().Hash(),
		timestamp:     b.chain.CurrentBlock().Time + 12,
		withdrawals:   types.Withdrawals{},
		beaconRoot:    new(common.Hash),
		gasLimit:      &gasLimit,
		txs:           append(types.Transactions{types.NewTx(&types.DepositTx{})}, txs...),
		noTxs:         noTxs,
		eip1559Params: eip1559.EncodeHolocene1559Params(250, 6),
		minBaseFee:    new(uint64),
	}
}

// requireJovianBlockImports checks that the block's gasUsed matches the DA footprint as computed
// by the state processor, and that the block can be imported.
func requireJovianBlockImports(t *testing.T, b *testWorkerBackend, block *types.Block) {
	t.Helper()
	var daFootprint uint64
	for _, tx := range block.Transactions() {
		if !tx.IsDepositTx() {
			daFootprint += tx.RollupCostData().EstimatedDASize().Uint64() * params.DAFootprintGasScalar
		}
	}
	require.Equal(t, daFootprint, block.GasUsed(), "block gas used should be equal to DA footprint")
	require.LessOrEqual(t, block.GasUsed(), block.GasLimit(), "block gas used must not exceed gas limit")

	_, err := b.chain.InsertChain(types.Blocks{block})
	require.NoError(t, err, "block import/execution failed")
}

// TestJovianDAFootprintForcedTxs tests that non-deposit transactions that are force-included via the
// payload attributes, like when a verifier builds blocks from derived batches, count towards the DA footprint.
func TestJovianDAFootprintForcedTxs(t *testing.T) {
	t.Run("forced-only", func(t *testing.T) {
		w, b := newTestWorker(t, jovianDAFootprintTestConfig(), beacon.New(ethash.NewFaker()), rawdb.NewMemoryDatabase(), 0)

		r := w.generateWork(jovianDAFootprintTestParams(b, genTxs(0, 10), true), false)
		require.NoError(t, r.err, "block generation failed")
		require.Len(t, r.block.Transactions(), 11)
		requireJovianBlockImports(t, b, r.block)
	})
	t.Run("forced-and-pool", func(t *testing.T) {
		w, b := newTestWorker(t, jovianDAFootprintTestConfig(), beacon.New(ethash.NewFaker()), rawdb.NewMemoryDatabase(), 0)
		// The forced txs take nonces 0-4, so the miner skips the pool txs with those nonces.
		for _, err := range b.txPool.Add(genTxs(1, 20), false) {
			require.NoError(t, err, "failed adding tx to pool")
		}

		r := w.generateWork(jovianDAFootprintTestParams(b, genTxs(0, 5), false), false)
		require.NoError(t, r.err, "block generation failed")
		require.Greater(t, len(r.block.Transactions()), 6, "pool txs should have been included")
		requireJovianBlockImports(t, b, r.block)
	})
}

// TestJovianDAFootprintPrioritySenders tests that the DA footprint limit is shared between the
// priority and the normal senders' transactions.
func TestJovianDAFootprintPrioritySenders(t *testing.T) {
	cfg := jovianDAFootprintTestConfig()
	engine := beacon.New(ethash.NewFaker())
	db := rawdb.NewMemoryDatabase()
	minBaseFee := uint64(0)
	gspec := &core.Genesis{
		Config: cfg,
		Alloc: types.GenesisAlloc{
			testBankAddress: {Balance: testBankFunds},
			testUserAddress: {Balance: testBankFunds},
		},
		ExtraData: eip1559.EncodeOptimismExtraData(cfg, 0, 250, 6, &minBaseFee),
	}
	chain, err := core.NewBlockChain(db, gspec, engine, &core.BlockChainConfig{ArchiveMode: true})
	require.NoError(t, err)
	pool, err := txpool.New(testTxPoolConfig.PriceLimit, chain, []txpool.SubPool{legacypool.New(testTxPoolConfig, chain)}, nil)
	require.NoError(t, err)
	b := &testWorkerBackend{db: db, chain: chain, txPool: pool, genesis: gspec}
	w := New(b, testConfig, engine)
	w.SetPrioAddresses([]common.Address{testBankAddress})

	// Each sender alone has enough txs to exceed the DA footprint limit.
	signer := types.LatestSigner(cfg)
	var txs types.Transactions
	for _, key := range []*ecdsa.PrivateKey{testBankKey, testUserKey} {
		for nonce := uint64(0); nonce < 20; nonce++ {
			data := make([]byte, 100)
			_, err := rand.Read(data)
			require.NoError(t, err)
			txs = append(txs, types.MustSignNewTx(key, signer, &types.AccessListTx{
				ChainID:  cfg.ChainID,
				Nonce:    nonce,
				To:       &testRecipient,
				Gas:      params.TxGas + uint64(len(data))*40,
				GasPrice: big.NewInt(params.InitialBaseFee),
				Data:     data,
			}))
		}
	}
	for _, err := range b.txPool.Add(txs, false) {
		require.NoError(t, err, "failed adding tx to pool")
	}

	r := w.generateWork(jovianDAFootprintTestParams(b, nil, false), false)
	require.NoError(t, r.err, "block generation failed")
	requireJovianBlockImports(t, b, r.block)
}
