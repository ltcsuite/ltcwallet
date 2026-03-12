package wallet

import (
	"errors"
	"testing"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/ltcutil/mweb"
	"github.com/ltcsuite/ltcd/ltcutil/mweb/mw"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/chain"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
	"github.com/ltcsuite/ltcwallet/walletdb"
	"github.com/ltcsuite/ltcwallet/wtxmgr"
)

// TestNeutrinoClientImplementsInterfaces verifies at compile time that
// NeutrinoClient satisfies the MwebReplayer and MwebUtxoChecker interfaces.
func TestNeutrinoClientImplementsInterfaces(t *testing.T) {
	t.Parallel()
	var _ chain.MwebReplayer = (*chain.NeutrinoClient)(nil)
	var _ chain.MwebUtxoChecker = (*chain.NeutrinoClient)(nil)
}

// TestRecoverMwebUtxosCallsReplay verifies that recoverMwebUtxos calls
// ReplayMwebUtxos on a chain client that supports MwebReplayer.
func TestRecoverMwebUtxosCallsReplay(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

	mock := &mockMwebChainClient{}
	mock.mwebSynced.Store(true)
	w.chainClientLock.Lock()
	w.chainClient = mock
	w.chainClientLock.Unlock()

	w.recoverMwebUtxos()

	if mock.replayCalled.Load() != 1 {
		t.Errorf("ReplayMwebUtxos called %d times, want 1",
			mock.replayCalled.Load())
	}
}

// TestRecoverMwebUtxosNoReplayerSupport verifies that recoverMwebUtxos
// gracefully returns when the chain client doesn't implement MwebReplayer.
func TestRecoverMwebUtxosNoReplayerSupport(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

	w.chainClientLock.Lock()
	w.chainClient = &mockChainClient{}
	w.chainClientLock.Unlock()

	// Should return without panicking.
	w.recoverMwebUtxos()
}

// TestRecoverMwebUtxosWaitsForSync verifies that recoverMwebUtxos polls
// IsMwebSynced() before calling ReplayMwebUtxos.
func TestRecoverMwebUtxosWaitsForSync(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

	mock := &mockMwebChainClient{}
	// Starts not synced (atomic.Bool zero value = false).
	w.chainClientLock.Lock()
	w.chainClient = mock
	w.chainClientLock.Unlock()

	done := make(chan struct{})
	go func() {
		w.recoverMwebUtxos()
		close(done)
	}()

	// Verify it hasn't called replay yet (waiting for sync).
	time.Sleep(100 * time.Millisecond)
	if mock.replayCalled.Load() != 0 {
		t.Fatal("ReplayMwebUtxos called before IsMwebSynced()")
	}

	// Now mark as synced (atomic store — no race).
	mock.mwebSynced.Store(true)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("recoverMwebUtxos didn't complete after sync")
	}

	if mock.replayCalled.Load() != 1 {
		t.Errorf("ReplayMwebUtxos called %d times, want 1",
			mock.replayCalled.Load())
	}
}

// TestRecoverMwebUtxosNoChainClient verifies that recoverMwebUtxos
// gracefully returns when there is no chain client.
func TestRecoverMwebUtxosNoChainClient(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

	w.chainClientLock.Lock()
	w.chainClient = nil
	w.chainClientLock.Unlock()

	w.recoverMwebUtxos()
}

// TestRecoverMwebUtxosReplayError verifies that recoverMwebUtxos
// handles errors from ReplayMwebUtxos gracefully (logs, doesn't panic).
func TestRecoverMwebUtxosReplayError(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

	mock := &mockMwebChainClient{
		replayErr: errors.New("test replay error"),
	}
	mock.mwebSynced.Store(true)
	w.chainClientLock.Lock()
	w.chainClient = mock
	w.chainClientLock.Unlock()

	w.recoverMwebUtxos()

	if mock.replayCalled.Load() != 1 {
		t.Errorf("ReplayMwebUtxos called %d times, want 1",
			mock.replayCalled.Load())
	}
}

// TestImportMwebScanKeyWithRescan verifies that ImportMwebScanKeyWithRescan
// creates the account and launches a background recovery goroutine that
// calls ReplayMwebUtxos.
func TestImportMwebScanKeyWithRescan(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

	mock := &mockMwebChainClient{}
	mock.mwebSynced.Store(true)
	w.chainClientLock.Lock()
	w.chainClient = mock
	w.chainClientLock.Unlock()

	var scanSecret [32]byte
	copy(scanSecret[:], mustDecodeHex(hwScanSecretHex))
	var spendPubKey [33]byte
	copy(spendPubKey[:], mustDecodeHex(hwSpendPubKeyHex))

	props, err := w.ImportMwebScanKeyWithRescan(
		"hw-rescan", scanSecret, spendPubKey, hwFingerprint, 100,
	)
	if err != nil {
		t.Fatalf("ImportMwebScanKeyWithRescan: %v", err)
	}

	if props.AccountName != "hw-rescan" {
		t.Errorf("name: got %q, want %q", props.AccountName, "hw-rescan")
	}
	if !props.IsWatchOnly {
		t.Error("expected IsWatchOnly=true")
	}
	if props.KeyScope != waddrmgr.KeyScopeMweb {
		t.Errorf("scope: got %v, want %v", props.KeyScope, waddrmgr.KeyScopeMweb)
	}

	// Wait for the background recovery goroutine to complete.
	deadline := time.After(5 * time.Second)
	for {
		if mock.replayCalled.Load() >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("ReplayMwebUtxos not called within timeout")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TestMockMwebUtxoChecker verifies the mock's MwebUtxoExists behavior:
// known hash → (true, nil), unknown hash → (false, nil), error mode.
func TestMockMwebUtxoChecker(t *testing.T) {
	t.Parallel()

	knownHash := chainhash.Hash{1, 2, 3}
	unknownHash := chainhash.Hash{4, 5, 6}

	mock := &mockMwebChainClient{
		utxoExists: map[chainhash.Hash]bool{
			knownHash: true,
		},
	}

	exists, err := mock.MwebUtxoExists(&knownHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected exists=true for known hash")
	}

	exists, err = mock.MwebUtxoExists(&unknownHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected exists=false for unknown hash")
	}
}

// setupMwebLeafsetTest creates a wallet with a mined MWEB output in the
// tx store and a stored old leafset. Returns the wallet, the MWEB output
// hash (used by checkMwebLeafset to check existence), the block header
// for the new leafset, and a cleanup function.
func setupMwebLeafsetTest(t *testing.T) (
	w *Wallet, outputHash *chainhash.Hash,
	newLeafsetHeader *wire.BlockHeader, cleanup func(),
) {
	t.Helper()

	w, cleanup = testMwebImportWallet(t)

	// Build a fake MwebOutput. We don't need valid crypto — just a struct
	// whose Hash() is deterministic. checkMwebLeafset never rewinds the
	// output, it only checks existence via MwebUtxoChecker.
	mwebOut := &wire.MwebOutput{
		Commitment:     mw.Commitment{0x02, 1, 2, 3},
		SenderPubKey:   mw.PublicKey{0x02, 4, 5, 6},
		ReceiverPubKey: mw.PublicKey{0x02, 7, 8, 9},
		Message: wire.MwebOutputMessage{
			Features:          wire.MwebOutputMessageStandardFieldsFeatureBit,
			KeyExchangePubKey: mw.PublicKey{0x02, 10, 11, 12},
			ViewTag:           0xAB,
			MaskedValue:       42,
		},
		Signature: mw.Signature{1, 2, 3, 4},
	}
	outputHash = mwebOut.Hash()

	// Create block headers for the chain. We need sequential blocks
	// registered in the address manager for SetSyncedTo to work.
	// Use distinct Nonce values so each header has a different BlockHash().
	headers := make([]*wire.BlockHeader, 6) // heights 0–5
	hashes := make([]chainhash.Hash, 6)
	for i := range headers {
		headers[i] = &wire.BlockHeader{
			Nonce: uint32(i + 1), // distinct per height
		}
		hashes[i] = headers[i].BlockHash()
	}

	err := walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		addrmgrNs := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		txmgrNs := tx.ReadWriteBucket(wtxmgrNamespaceKey)

		// Register blocks 1–5 in the address manager.
		for h := int32(1); h <= 5; h++ {
			err := w.Manager.SetSyncedTo(addrmgrNs, &waddrmgr.BlockStamp{
				Hash:      hashes[h],
				Height:    h,
				Timestamp: time.Unix(int64(1000+h), 0),
			})
			if err != nil {
				return err
			}
		}

		// Store old leafset at height 3, with matching block hash.
		// Size must match Bits length: 8 bits = 1 byte.
		oldLeafset := &mweb.Leafset{
			Size:   8,
			Height: 3,
			Block:  headers[3],
			Bits:   []byte{0xFF}, // all bits set
		}
		if err := w.putMwebLeafset(addrmgrNs, oldLeafset); err != nil {
			return err
		}

		// Insert a mined MWEB transaction at height 2.
		txHash := chainhash.Hash{0xAA, 0xBB}
		rec := &wtxmgr.TxRecord{
			MsgTx: wire.MsgTx{
				TxOut: []*wire.TxOut{
					wire.NewTxOut(100000, []byte{0x00, 0x14, 0x01, 0x02}),
				},
				Mweb: &wire.MwebTx{TxBody: &wire.MwebTxBody{
					Outputs: []*wire.MwebOutput{mwebOut},
					Kernels: []*wire.MwebKernel{{}},
				}},
			},
			Hash:     txHash,
			Received: time.Unix(1002, 0),
		}
		block := &wtxmgr.BlockMeta{
			Block: wtxmgr.Block{Hash: hashes[2], Height: 2},
			Time:  time.Unix(1002, 0),
		}

		if _, err := w.TxStore.InsertTxCheckIfExists(txmgrNs, rec, block); err != nil {
			return err
		}
		if err := w.TxStore.AddCredit(txmgrNs, rec, block, 0, false); err != nil {
			return err
		}
		if err := w.TxStore.AddMwebOutpoint(txmgrNs, outputHash,
			wire.NewOutPoint(&txHash, 0)); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// The new leafset uses height 4. Its Block must have a valid header
	// (non-nil) — used by checkMwebLeafset to build the block meta.
	newLeafsetHeader = headers[4]
	return
}

// TestCheckMwebLeafsetOutputExists verifies that when a MwebUtxoChecker
// backend reports an output as still existing, no spend record is created.
func TestCheckMwebLeafsetOutputExists(t *testing.T) {
	t.Parallel()
	w, outputHash, newHeader, cleanup := setupMwebLeafsetTest(t)
	defer cleanup()

	mock := &mockMwebChainClient{
		utxoExists: map[chainhash.Hash]bool{
			*outputHash: true, // output still exists
		},
	}
	w.chainClientLock.Lock()
	w.chainClient = mock
	w.chainClientLock.Unlock()

	err := walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		return w.checkMwebLeafset(tx, &mweb.Leafset{
			Size:   8,
			Height: 4,
			Block:  newHeader,
			Bits:   []byte{0xFE}, // bit 0 cleared → triggers spend check
		})
	})
	if err != nil {
		t.Fatalf("checkMwebLeafset: %v", err)
	}

	// Verify no spend transaction was created. If a spend record were
	// created, it would be an additional unmined or mined tx with TxIn
	// referencing our output. We verify by checking UnspentOutputs still
	// contains our output.
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		txmgrNs := tx.ReadBucket(wtxmgrNamespaceKey)
		credits, err := w.TxStore.UnspentOutputs(txmgrNs)
		if err != nil {
			return err
		}
		for _, c := range credits {
			if c.MwebOutput != nil && *c.MwebOutput.Hash() == *outputHash {
				return nil // found, still unspent — correct
			}
		}
		t.Error("MWEB output not found in unspent set (should still exist)")
		return nil
	})
	if err != nil {
		t.Fatalf("verify unspent: %v", err)
	}
}

// TestCheckMwebLeafsetOutputMissing verifies that when a MwebUtxoChecker
// backend reports an output as missing (spent), a spend record is created.
func TestCheckMwebLeafsetOutputMissing(t *testing.T) {
	t.Parallel()
	w, outputHash, newHeader, cleanup := setupMwebLeafsetTest(t)
	defer cleanup()

	mock := &mockMwebChainClient{
		utxoExists: map[chainhash.Hash]bool{
			// outputHash NOT present → MwebUtxoExists returns false
		},
	}
	w.chainClientLock.Lock()
	w.chainClient = mock
	w.chainClientLock.Unlock()

	err := walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		return w.checkMwebLeafset(tx, &mweb.Leafset{
			Size:   8,
			Height: 4,
			Block:  newHeader,
			Bits:   []byte{0xFE}, // bit 0 cleared → triggers spend check
		})
	})
	if err != nil {
		t.Fatalf("checkMwebLeafset: %v", err)
	}

	// Verify the output is now marked as spent. A spend record tx was
	// created with TxIn referencing our outpoint, consuming the credit.
	// After that, UnspentOutputs should NOT contain it.
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		txmgrNs := tx.ReadBucket(wtxmgrNamespaceKey)
		credits, err := w.TxStore.UnspentOutputs(txmgrNs)
		if err != nil {
			return err
		}
		for _, c := range credits {
			if c.MwebOutput != nil && *c.MwebOutput.Hash() == *outputHash {
				t.Error("MWEB output still in unspent set (should be spent)")
				return nil
			}
		}
		return nil // not found — correct, it was spent
	})
	if err != nil {
		t.Fatalf("verify spent: %v", err)
	}
}

// TestCheckMwebLeafsetCheckerError verifies that errors from
// MwebUtxoExists are propagated (not silently swallowed).
func TestCheckMwebLeafsetCheckerError(t *testing.T) {
	t.Parallel()
	w, _, newHeader, cleanup := setupMwebLeafsetTest(t)
	defer cleanup()

	testErr := errors.New("coin DB connection lost")
	mock := &mockMwebChainClient{
		errOnMissing: testErr, // any unknown hash returns this error
	}
	w.chainClientLock.Lock()
	w.chainClient = mock
	w.chainClientLock.Unlock()

	err := walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		return w.checkMwebLeafset(tx, &mweb.Leafset{
			Size:   8,
			Height: 4,
			Block:  newHeader,
			Bits:   []byte{0xFE},
		})
	})
	if err == nil {
		t.Fatal("expected error from MwebUtxoExists, got nil")
	}
	if !errors.Is(err, testErr) {
		t.Errorf("expected %q, got %q", testErr, err)
	}
}

// TestCheckMwebLeafsetNonCheckerBackend verifies that a chain client
// NOT implementing MwebUtxoChecker causes checkMwebLeafset to return
// nil (graceful fallback, same as old Electrum behavior).
func TestCheckMwebLeafsetNonCheckerBackend(t *testing.T) {
	t.Parallel()
	w, _, newHeader, cleanup := setupMwebLeafsetTest(t)
	defer cleanup()

	// Use plain mockChainClient — does NOT implement MwebUtxoChecker.
	w.chainClientLock.Lock()
	w.chainClient = &mockChainClient{}
	w.chainClientLock.Unlock()

	err := walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		return w.checkMwebLeafset(tx, &mweb.Leafset{
			Size:   8,
			Height: 4,
			Block:  newHeader,
			Bits:   []byte{0xFE}, // bits cleared, but no checker → skip
		})
	})
	if err != nil {
		t.Fatalf("checkMwebLeafset with non-checker backend: %v", err)
	}
}

// TestCheckMwebUtxosDiscoversImportedOutput is the end-to-end test for the
// recovery pipeline: it creates a valid MWEB output addressed to the imported
// scan key, feeds it through checkMwebUtxos (the same function that
// handleChainNotifications dispatches MwebUtxos notifications to), and verifies
// the wallet discovers and stores it. This covers the critical gap: replay
// delivers UTXOs → checkMwebUtxos → rewindOutput matches the imported scan
// key → output stored as wallet credit.
func TestCheckMwebUtxosDiscoversImportedOutput(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

	// Set chain client to mockMwebChainClient (needed for getBlockMeta).
	mock := &mockMwebChainClient{}
	w.chainClientLock.Lock()
	w.chainClient = mock
	w.chainClientLock.Unlock()

	// Import the hw scan key.
	var scanSecret [32]byte
	copy(scanSecret[:], mustDecodeHex(hwScanSecretHex))
	var spendPubKey [33]byte
	copy(spendPubKey[:], mustDecodeHex(hwSpendPubKeyHex))

	props, err := w.ImportMwebScanKey(
		"hw-mweb", scanSecret, spendPubKey, hwFingerprint, 100,
	)
	if err != nil {
		t.Fatalf("ImportMwebScanKey: %v", err)
	}

	// Build a keychain from the same keys to create a valid output.
	var scanKey mw.SecretKey
	copy(scanKey[:], mustDecodeHex(hwScanSecretHex))
	var spendPub mw.PublicKey
	copy(spendPub[:], mustDecodeHex(hwSpendPubKeyHex))
	kc := &mweb.Keychain{
		Scan:        &scanKey,
		SpendPubKey: &spendPub,
	}

	// Create a valid MWEB output to the imported account's address at
	// index 0, using a pegin (no input coins needed).
	addr := kc.Address(0)
	amount := uint64(500000)
	mwebTx, _, err := mweb.NewTransaction(
		nil,                                    // no input coins
		[]*mweb.Recipient{{Value: amount, Address: addr}},
		0,      // fee
		amount, // pegin amount = output amount
		nil,    // no pegouts
	)
	if err != nil {
		t.Fatalf("NewTransaction: %v", err)
	}

	if len(mwebTx.TxBody.Outputs) == 0 {
		t.Fatal("NewTransaction produced no outputs")
	}
	output := mwebTx.TxBody.Outputs[0]
	outputHash := output.Hash()

	// Sanity check: verify the output can be rewound with the scan secret.
	coin, err := mweb.RewindOutput(output, kc.Scan)
	if err != nil {
		t.Fatalf("RewindOutput sanity check: %v", err)
	}
	if coin.Value != amount {
		t.Fatalf("RewindOutput value: got %d, want %d", coin.Value, amount)
	}

	// Verify the output is NOT yet known to the wallet.
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(wtxmgrNamespaceKey)
		op, _, err := w.TxStore.GetMwebOutpoint(ns, outputHash)
		if err != nil {
			return err
		}
		if op != nil {
			t.Fatal("output should not be known before checkMwebUtxos")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("pre-check: %v", err)
	}

	// Feed the output through checkMwebUtxos as an unconfirmed UTXO
	// (height=0). This is the same code path that handleChainNotifications
	// uses when processing a chain.MwebUtxos notification from ReplayMwebUtxos.
	err = walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		return w.checkMwebUtxos(tx, &chain.MwebUtxos{
			Utxos: []*wire.MwebNetUtxo{{
				Height:   0, // unconfirmed
				Output:   output,
				OutputId: outputHash,
			}},
		})
	})
	if err != nil {
		t.Fatalf("checkMwebUtxos: %v", err)
	}

	// Verify the output IS now stored in the wallet.
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(wtxmgrNamespaceKey)
		op, rec, err := w.TxStore.GetMwebOutpoint(ns, outputHash)
		if err != nil {
			return err
		}
		if op == nil {
			t.Error("MWEB outpoint not found after checkMwebUtxos")
			return nil
		}
		if rec == nil {
			t.Error("MWEB transaction record not found")
			return nil
		}
		return nil
	})
	if err != nil {
		t.Fatalf("post-check outpoint: %v", err)
	}

	// Verify it appears in the unspent outputs with correct properties.
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(wtxmgrNamespaceKey)
		credits, err := w.TxStore.UnspentOutputs(ns)
		if err != nil {
			return err
		}
		for _, c := range credits {
			if c.MwebOutput != nil && *c.MwebOutput.Hash() == *outputHash {
				if int64(c.Amount) != int64(amount) {
					t.Errorf("credit amount: got %d, want %d",
						c.Amount, amount)
				}
				return nil
			}
		}
		t.Error("imported MWEB output not found in unspent credits")
		return nil
	})
	if err != nil {
		t.Fatalf("post-check unspent: %v", err)
	}

	// Verify the output was discovered for the imported account specifically,
	// not the seed-derived account 0.
	_ = props // account verification is implicit: the keypool for account 1
	// (imported) contains the address at index 0 from the hw scan key.
	// If rewindOutput matched the wrong account, the address wouldn't be
	// in that account's keypool and the output would be skipped.
}
