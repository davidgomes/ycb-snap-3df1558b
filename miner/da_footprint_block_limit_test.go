package miner

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"
)

const daFootprintTestBlockGasLimit = 1_000_000

var (
	daFootprintTestKey, _   = crypto.GenerateKey()
	daFootprintTestAddress  = crypto.PubkeyToAddress(daFootprintTestKey.PublicKey)
	daFootprintTestTo       = common.HexToAddress("0xda")
	daFootprintTestCoinbase = common.HexToAddress("0xc0ffee")
)

type daFootprintTestBackend struct {
	chain  *core.BlockChain
	txPool *txpool.TxPool
}

func (b *daFootprintTestBackend) BlockChain() *core.BlockChain { return b.chain }
func (b *daFootprintTestBackend) TxPool() *txpool.TxPool       { return b.txPool }

// daFootprintTestChainConfig returns an OP Stack chain config with all forks up to Isthmus
// active at genesis, and additionally Jovian if requested.
func daFootprintTestChainConfig(jovian bool) *params.ChainConfig {
	cfg := *params.OptimismTestConfig
	if jovian {
		cfg.JovianTime = new(uint64)
	}
	return &cfg
}

func newDAFootprintTestMiner(t *testing.T, cfg *params.ChainConfig) (*Miner, *daFootprintTestBackend) {
	t.Helper()
	minBaseFee := uint64(0)
	gspec := &core.Genesis{
		Config:    cfg,
		GasLimit:  daFootprintTestBlockGasLimit,
		BaseFee:   big.NewInt(params.InitialBaseFee),
		ExtraData: eip1559.EncodeOptimismExtraData(cfg, 0, 250, 6, &minBaseFee),
		Alloc:     types.GenesisAlloc{daFootprintTestAddress: {Balance: big.NewInt(params.Ether)}},
	}
	engine := beacon.New(ethash.NewFaker())
	chain, err := core.NewBlockChain(rawdb.NewMemoryDatabase(), gspec, engine, &core.BlockChainConfig{ArchiveMode: true})
	require.NoError(t, err)
	t.Cleanup(chain.Stop)

	poolConfig := legacypool.DefaultConfig
	poolConfig.Journal = ""
	pool := legacypool.New(poolConfig, chain)
	txPool, err := txpool.New(poolConfig.PriceLimit, chain, []txpool.SubPool{pool}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { txPool.Close() })

	backend := &daFootprintTestBackend{chain: chain, txPool: txPool}
	w := New(backend, Config{Recommit: time.Second}, engine)
	t.Cleanup(w.Close)
	return w, backend
}

// daFootprintTestTxs creates count transactions with 100 bytes of incompressible calldata each.
func daFootprintTestTxs(t *testing.T, cfg *params.ChainConfig, startNonce, count uint64) types.Transactions {
	t.Helper()
	signer := types.LatestSigner(cfg)
	txs := make(types.Transactions, 0, count)
	for nonce := startNonce; nonce < startNonce+count; nonce++ {
		data := make([]byte, 100)
		_, err := rand.Read(data)
		require.NoError(t, err)
		txs = append(txs, types.MustSignNewTx(daFootprintTestKey, signer, &types.AccessListTx{
			ChainID:  cfg.ChainID,
			Nonce:    nonce,
			To:       &daFootprintTestTo,
			Value:    big.NewInt(1),
			Gas:      params.TxGas + uint64(len(data))*params.TxCostFloorPerToken*params.TxTokenPerNonZeroByte,
			GasPrice: big.NewInt(2 * params.InitialBaseFee),
			Data:     data,
		}))
	}
	return txs
}

// daFootprintTestL1InfoDeposit returns an L1 attributes deposit transaction. If scalar is nil,
// the deposit uses the Isthmus format, otherwise the Jovian format with the given scalar.
func daFootprintTestL1InfoDeposit(scalar *uint16) *types.Transaction {
	data := make([]byte, types.IsthmusL1AttributesLen)
	copy(data, types.IsthmusL1AttributesSelector)
	if scalar != nil {
		data = make([]byte, types.JovianL1AttributesLen)
		copy(data, types.JovianL1AttributesSelector)
		binary.BigEndian.PutUint16(data[types.IsthmusL1AttributesLen:], *scalar)
	}
	return types.NewTx(&types.DepositTx{
		From: common.HexToAddress("0xdeaddeaddeaddeaddeaddeaddeaddeaddead0001"),
		To:   &types.L1BlockAddr,
		Data: data,
	})
}

// buildDAFootprintTestBlock builds a block on top of the current head from the forced
// transactions and, unless noTxs is set, from the transaction pool.
func buildDAFootprintTestBlock(w *Miner, b *daFootprintTestBackend, forced types.Transactions, noTxs bool) *newPayloadResult {
	parent := b.chain.CurrentBlock()
	gasLimit := uint64(daFootprintTestBlockGasLimit)
	genParams := &generateParams{
		parentHash:    parent.Hash(),
		timestamp:     parent.Time + 2,
		forceTime:     true,
		coinbase:      daFootprintTestCoinbase,
		withdrawals:   types.Withdrawals{},
		beaconRoot:    new(common.Hash),
		gasLimit:      &gasLimit,
		txs:           forced,
		eip1559Params: eip1559.EncodeHolocene1559Params(250, 6),
		noTxs:         noTxs,
	}
	if w.chainConfig.IsMinBaseFee(genParams.timestamp) {
		genParams.minBaseFee = new(uint64)
	}
	return w.generateWork(genParams, false)
}

// maxFittingDAFootprintTxs returns how many of the given transactions, in order, fit into a block
// with the given gas limit when only accounting for their DA footprints.
func maxFittingDAFootprintTxs(txs types.Transactions, scalar uint16, gasLimit uint64) int {
	var total uint64
	for i, tx := range txs {
		total += tx.DAFootprint(scalar)
		if total > gasLimit {
			return i
		}
	}
	return len(txs)
}

func sumReceiptsGasUsed(receipts []*types.Receipt) uint64 {
	var gasUsed uint64
	for _, r := range receipts {
		gasUsed += r.GasUsed
	}
	return gasUsed
}

func TestMinerJovianDAFootprintBlockLimit(t *testing.T) {
	scalar400 := uint16(400)

	t.Run("jovian-min-size-tx", func(t *testing.T) {
		cfg := daFootprintTestChainConfig(true)
		w, b := newDAFootprintTestMiner(t, cfg)
		tx := types.MustSignNewTx(daFootprintTestKey, types.LatestSigner(cfg), &types.DynamicFeeTx{
			ChainID:   cfg.ChainID,
			To:        &daFootprintTestTo,
			Gas:       params.TxGas,
			GasFeeCap: big.NewInt(2 * params.InitialBaseFee),
			GasTipCap: big.NewInt(1),
		})
		require.NoError(t, b.txPool.Add(types.Transactions{tx}, true)[0])

		r := buildDAFootprintTestBlock(w, b, types.Transactions{daFootprintTestL1InfoDeposit(&scalar400)}, false)
		require.NoError(t, r.err)
		require.Len(t, r.block.Transactions(), 2)

		// The tx is below the minimum DA size, so its DA footprint dominates its gas usage.
		daFootprint := types.MinTransactionSize.Uint64() * uint64(scalar400)
		require.Equal(t, daFootprint, tx.DAFootprint(scalar400))
		require.Less(t, sumReceiptsGasUsed(r.receipts), daFootprint)
		require.Equal(t, daFootprint, r.block.GasUsed())

		_, err := b.chain.InsertChain(types.Blocks{r.block})
		require.NoError(t, err)
	})

	for _, tt := range []struct {
		name   string
		scalar uint16
	}{
		{"jovian-scalar-400", 400},
		{"jovian-scalar-800", 800},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := daFootprintTestChainConfig(true)
			w, b := newDAFootprintTestMiner(t, cfg)
			txs := daFootprintTestTxs(t, cfg, 0, 40)
			for _, err := range b.txPool.Add(txs, true) {
				require.NoError(t, err)
			}
			expectedTxs := maxFittingDAFootprintTxs(txs, tt.scalar, daFootprintTestBlockGasLimit)
			require.Less(t, expectedTxs, len(txs), "DA footprint limit should be hit")

			deposit := daFootprintTestL1InfoDeposit(&tt.scalar)
			r := buildDAFootprintTestBlock(w, b, types.Transactions{deposit}, false)
			require.NoError(t, r.err)

			blockTxs := r.block.Transactions()
			require.Len(t, blockTxs, 1+expectedTxs, "block should contain the deposit and all txs that fit the DA footprint limit")

			daFootprint, err := types.CalcDAFootprint(blockTxs)
			require.NoError(t, err)
			txGasUsed := sumReceiptsGasUsed(r.receipts)
			require.Less(t, txGasUsed, daFootprint)
			require.Equal(t, daFootprint, r.block.GasUsed(), "block gas used should be the DA footprint")
			require.LessOrEqual(t, daFootprint, r.block.GasLimit())
			require.Greater(t, daFootprint+txs[expectedTxs].DAFootprint(tt.scalar), r.block.GasLimit(),
				"next tx should not have fit the DA footprint limit")
			require.Equal(t, txGasUsed, r.receipts[len(r.receipts)-1].CumulativeGasUsed,
				"receipts should only account for tx gas")

			_, err = b.chain.InsertChain(types.Blocks{r.block})
			require.NoError(t, err)
		})
	}

	t.Run("jovian-default-scalar", func(t *testing.T) {
		cfg := daFootprintTestChainConfig(true)
		w, b := newDAFootprintTestMiner(t, cfg)
		txs := daFootprintTestTxs(t, cfg, 0, 40)
		for _, err := range b.txPool.Add(txs, true) {
			require.NoError(t, err)
		}
		// A zero scalar in the L1 attributes means that the default scalar applies.
		zero := uint16(0)
		r := buildDAFootprintTestBlock(w, b, types.Transactions{daFootprintTestL1InfoDeposit(&zero)}, false)
		require.NoError(t, r.err)
		expectedTxs := maxFittingDAFootprintTxs(txs, types.DAFootprintGasScalarDefault, daFootprintTestBlockGasLimit)
		require.Len(t, r.block.Transactions(), 1+expectedTxs)

		_, err := b.chain.InsertChain(types.Blocks{r.block})
		require.NoError(t, err)
	})

	t.Run("jovian-forced-txs-exceed-limit", func(t *testing.T) {
		cfg := daFootprintTestChainConfig(true)
		w, b := newDAFootprintTestMiner(t, cfg)
		txs := daFootprintTestTxs(t, cfg, 0, 40)
		fitting := maxFittingDAFootprintTxs(txs, scalar400, daFootprintTestBlockGasLimit)

		forced := append(types.Transactions{daFootprintTestL1InfoDeposit(&scalar400)}, txs[:fitting+1]...)
		r := buildDAFootprintTestBlock(w, b, forced, true)
		require.ErrorIs(t, r.err, errDAFootprintLimitReached)

		forced = forced[:len(forced)-1]
		r = buildDAFootprintTestBlock(w, b, forced, true)
		require.NoError(t, r.err)
		require.Len(t, r.block.Transactions(), 1+fitting)
	})

	t.Run("isthmus", func(t *testing.T) {
		cfg := daFootprintTestChainConfig(false)
		w, b := newDAFootprintTestMiner(t, cfg)
		txs := daFootprintTestTxs(t, cfg, 0, 60)
		for _, err := range b.txPool.Add(txs, true) {
			require.NoError(t, err)
		}
		r := buildDAFootprintTestBlock(w, b, types.Transactions{daFootprintTestL1InfoDeposit(nil)}, false)
		require.NoError(t, r.err)

		// Only the gas limit applies, so more txs fit than with the DA footprint limit.
		txGasUsed := sumReceiptsGasUsed(r.receipts)
		require.Equal(t, txGasUsed, r.block.GasUsed())
		require.Less(t, r.block.GasLimit()-txGasUsed, txs[0].Gas())
		require.Greater(t, len(r.block.Transactions())-1, maxFittingDAFootprintTxs(txs, scalar400, daFootprintTestBlockGasLimit))

		_, err := b.chain.InsertChain(types.Blocks{r.block})
		require.NoError(t, err)
	})
}

func TestMinerJovianRequiresMinBaseFee(t *testing.T) {
	cfg := daFootprintTestChainConfig(true)
	w, b := newDAFootprintTestMiner(t, cfg)
	parent := b.chain.CurrentBlock()
	args := &BuildPayloadArgs{
		Parent:        parent.Hash(),
		Timestamp:     parent.Time + 2,
		FeeRecipient:  daFootprintTestCoinbase,
		Withdrawals:   types.Withdrawals{},
		BeaconRoot:    new(common.Hash),
		EIP1559Params: eip1559.EncodeHolocene1559Params(250, 6),
	}
	for _, noTxPool := range []bool{true, false} {
		args.NoTxPool = noTxPool
		_, err := w.buildPayload(args, false)
		require.ErrorIs(t, err, errMissingMinBaseFee)
	}

	// Zero denominators are only allowed together with a zero elasticity.
	args.MinBaseFee = new(uint64)
	args.EIP1559Params = eip1559.EncodeHolocene1559Params(0, 6)
	for _, noTxPool := range []bool{true, false} {
		args.NoTxPool = noTxPool
		_, err := w.buildPayload(args, false)
		require.Error(t, err)
	}
}
