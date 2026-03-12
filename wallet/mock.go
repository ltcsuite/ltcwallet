package wallet

import (
	"sync/atomic"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/chain"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
)

type mockChainClient struct {
}

var _ chain.Interface = (*mockChainClient)(nil)

// mockMwebChainClient extends mockChainClient with MwebReplayer and
// MwebUtxoChecker implementations for testing.
type mockMwebChainClient struct {
	mockChainClient

	// utxoExists maps output hash → exists. If a hash is not in the map
	// and errOnMissing is set, MwebUtxoExists returns that error.
	utxoExists    map[chainhash.Hash]bool
	errOnMissing  error
	replayCalled  atomic.Int32
	replayErr     error
	mwebSynced    atomic.Bool
}

var (
	_ chain.Interface      = (*mockMwebChainClient)(nil)
	_ chain.MwebReplayer   = (*mockMwebChainClient)(nil)
	_ chain.MwebUtxoChecker = (*mockMwebChainClient)(nil)
)

func (m *mockMwebChainClient) ReplayMwebUtxos() error {
	m.replayCalled.Add(1)
	return m.replayErr
}

func (m *mockMwebChainClient) MwebUtxoExists(hash *chainhash.Hash) (bool, error) {
	if exists, ok := m.utxoExists[*hash]; ok {
		return exists, nil
	}
	if m.errOnMissing != nil {
		return false, m.errOnMissing
	}
	return false, nil
}

func (m *mockMwebChainClient) IsMwebSynced() bool {
	return m.mwebSynced.Load()
}

// Override GetBlockHash and GetBlockHeader to return valid data for
// checkMwebUtxos which calls getBlockMeta even for height=0 UTXOs.
func (m *mockMwebChainClient) GetBlockHash(height int64) (*chainhash.Hash, error) {
	hash := chainhash.Hash{byte(height + 1)}
	return &hash, nil
}

func (m *mockMwebChainClient) GetBlockHeader(hash *chainhash.Hash) (
	*wire.BlockHeader, error) {
	return &wire.BlockHeader{Nonce: uint32(hash[0])}, nil
}

func (m *mockChainClient) Start() error {
	return nil
}

func (m *mockChainClient) Stop() {
}

func (m *mockChainClient) WaitForShutdown() {}

func (m *mockChainClient) GetBestBlock() (*chainhash.Hash, int32, error) {
	return nil, 0, nil
}

func (m *mockChainClient) GetBlock(*chainhash.Hash) (*wire.MsgBlock, error) {
	return nil, nil
}

func (m *mockChainClient) GetBlockHash(int64) (*chainhash.Hash, error) {
	return nil, nil
}

func (m *mockChainClient) GetBlockHeader(*chainhash.Hash) (*wire.BlockHeader,
	error) {
	return nil, nil
}

func (m *mockChainClient) IsCurrent() bool {
	return false
}

func (m *mockChainClient) FilterBlocks(*chain.FilterBlocksRequest) (
	*chain.FilterBlocksResponse, error) {
	return nil, nil
}

func (m *mockChainClient) BlockStamp() (*waddrmgr.BlockStamp, error) {
	return &waddrmgr.BlockStamp{
		Height:    500000,
		Hash:      chainhash.Hash{},
		Timestamp: time.Unix(1234, 0),
	}, nil
}

func (m *mockChainClient) SendRawTransaction(*wire.MsgTx, bool) (
	*chainhash.Hash, error) {
	return nil, nil
}

func (m *mockChainClient) Rescan(*chainhash.Hash, []ltcutil.Address,
	map[wire.OutPoint]ltcutil.Address) error {
	return nil
}

func (m *mockChainClient) NotifyReceived([]ltcutil.Address) error {
	return nil
}

func (m *mockChainClient) NotifyBlocks() error {
	return nil
}

func (m *mockChainClient) Notifications() <-chan interface{} {
	return nil
}

func (m *mockChainClient) BackEnd() string {
	return "mock"
}
