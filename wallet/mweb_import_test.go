package wallet

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/ltcutil/hdkeychain"
	"github.com/ltcsuite/ltcd/ltcutil/mweb/mw"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
	"github.com/ltcsuite/ltcwallet/walletdb"
)

// Test data: known scan secret and spend pubkey from the standard MWEB
// derivation path m/0'/100'. These match the test vectors in
// waddrmgr/mweb_compat_test.go and Jade firmware tests.
var (
	hwScanSecretHex  = "b3c91b7291c2e1e06d4a93f3dc32404aef9927db8e794c01a7b4de18a397c338"
	hwSpendPubKeyHex = "03e3908af70085b458020e64aaa5c9a4b8ff382d42af0875c8145db6a30db9cad2"
	hwFingerprint    = uint32(0xe66c70b2)
)

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// testMwebImportWallet creates a new post-activation wallet suitable for
// testing MWEB import. The wallet is unlocked and has the standard MWEB
// scope with seed-derived account 0.
func testMwebImportWallet(t *testing.T) (*Wallet, func()) {
	t.Helper()
	// Post-activation wallet → has KeyScopeMweb, not legacy.
	w, cleanup := testMwebWallet(t,
		time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), true)
	return w, cleanup
}

// TestImportMwebScanKey verifies the full import flow: DB account creation,
// keypool initialization, and accessibility via forEachMwebAccount.
func TestImportMwebScanKey(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

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

	// Verify returned properties.
	if props.AccountName != "hw-mweb" {
		t.Errorf("name: got %q, want %q", props.AccountName, "hw-mweb")
	}
	if !props.IsWatchOnly {
		t.Error("expected IsWatchOnly=true")
	}
	if props.KeyScope != waddrmgr.KeyScopeMweb {
		t.Errorf("scope: got %v, want %v", props.KeyScope, waddrmgr.KeyScopeMweb)
	}
	if props.MasterKeyFingerprint != hwFingerprint {
		t.Errorf("fingerprint: got %x, want %x",
			props.MasterKeyFingerprint, hwFingerprint)
	}

	// Verify the account is in the standard MWEB scope.
	// Account 0 is seed-derived, imported should be account 1.
	if props.AccountNumber != 1 {
		t.Errorf("account number: got %d, want 1", props.AccountNumber)
	}

	// Verify ExternalKeyCount matches the recovery window (addresses
	// persisted to DB via ExtendExternalAddresses during import).
	if props.ExternalKeyCount != 100 {
		t.Errorf("ExternalKeyCount: got %d, want 100", props.ExternalKeyCount)
	}

	// Verify keypool was initialized by checking it exists.
	scopedMgr, err := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb)
	if err != nil {
		t.Fatalf("FetchScopedKeyManager: %v", err)
	}
	key := skmAccount{skm: scopedMgr, account: props.AccountNumber}
	_, ok := w.getMwebKeyPool(key)
	if !ok {
		t.Fatal("keypool not initialized for imported account")
	}

	// Verify forEachMwebAccount picks up the imported account.
	var foundImported bool
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		return w.forEachMwebAccount(ns, func(ma *mwebAccount) error {
			if ma.account == props.AccountNumber {
				foundImported = true
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("forEachMwebAccount: %v", err)
	}
	if !foundImported {
		t.Error("forEachMwebAccount did not include imported account")
	}

	// Verify LoadMwebKeychain works on the imported account.
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		kc, err := scopedMgr.LoadMwebKeychain(ns, props.AccountNumber)
		if err != nil {
			return err
		}
		if kc == nil {
			t.Error("LoadMwebKeychain returned nil")
			return nil
		}
		gotScan := hex.EncodeToString(kc.Scan[:])
		if gotScan != hwScanSecretHex {
			t.Errorf("scan mismatch:\n  got:  %s\n  want: %s",
				gotScan, hwScanSecretHex)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("LoadMwebKeychain: %v", err)
	}
}

// TestImportMwebScanKeyOnLegacyMigratedWallet verifies that importing on
// a legacy wallet that has been unlocked (triggering migration to create
// KeyScopeMweb) works correctly alongside the seed-derived account 0.
func TestImportMwebScanKeyOnLegacyMigratedWallet(t *testing.T) {
	t.Parallel()

	// Legacy wallet (pre-activation), unlocked → migration creates
	// KeyScopeMweb with seed-derived account 0.
	w, cleanup := testMwebWallet(t,
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), true)
	defer cleanup()

	// Verify both scopes exist (legacy from creation, standard from migration).
	if _, err := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMwebLegacy); err != nil {
		t.Fatalf("legacy scope missing: %v", err)
	}
	if _, err := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb); err != nil {
		t.Fatalf("standard scope missing after migration: %v", err)
	}

	var scanSecret [32]byte
	copy(scanSecret[:], mustDecodeHex(hwScanSecretHex))
	var spendPubKey [33]byte
	copy(spendPubKey[:], mustDecodeHex(hwSpendPubKeyHex))

	props, err := w.ImportMwebScanKey(
		"hw-mweb", scanSecret, spendPubKey, hwFingerprint, 50,
	)
	if err != nil {
		t.Fatalf("ImportMwebScanKey: %v", err)
	}

	// Imported account should be in the standard scope.
	if props.KeyScope != waddrmgr.KeyScopeMweb {
		t.Errorf("scope: got %v, want %v", props.KeyScope, waddrmgr.KeyScopeMweb)
	}

	// Verify native account 0 still has seed-derived keys.
	scopedMgr, _ := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb)
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		nativeProps, err := scopedMgr.AccountProperties(ns, 0)
		if err != nil {
			return err
		}
		if nativeProps.IsWatchOnly {
			t.Error("native account 0 should not be watch-only")
		}
		if nativeProps.AccountName != "default" {
			t.Errorf("native account 0 name: got %q", nativeProps.AccountName)
		}

		// Verify imported account has the hw keys.
		importProps, err := scopedMgr.AccountProperties(ns, props.AccountNumber)
		if err != nil {
			return err
		}
		if !importProps.IsWatchOnly {
			t.Error("imported account should be watch-only")
		}
		if importProps.MasterKeyFingerprint != hwFingerprint {
			t.Errorf("imported fingerprint: got %x, want %x",
				importProps.MasterKeyFingerprint, hwFingerprint)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify accounts: %v", err)
	}
}

// TestPreferredMwebScopeFixed verifies that after removing the account==0
// guard, preferredMwebScope returns KeyScopeMweb for ALL accounts when
// the standard scope exists — including account > 0 (imported accounts).
func TestPreferredMwebScopeFixed(t *testing.T) {
	t.Parallel()

	t.Run("account 0 returns standard when exists", func(t *testing.T) {
		t.Parallel()
		w, cleanup := testMwebImportWallet(t)
		defer cleanup()

		scope := w.preferredMwebScope(0)
		if scope != waddrmgr.KeyScopeMweb {
			t.Errorf("got %v, want KeyScopeMweb", scope)
		}
	})

	t.Run("account 1 returns standard when exists", func(t *testing.T) {
		t.Parallel()
		w, cleanup := testMwebImportWallet(t)
		defer cleanup()

		scope := w.preferredMwebScope(1)
		if scope != waddrmgr.KeyScopeMweb {
			t.Errorf("got %v, want KeyScopeMweb for account 1", scope)
		}
	})

	t.Run("returns legacy when standard absent", func(t *testing.T) {
		t.Parallel()
		// Pre-activation, NOT unlocked → no migration → no standard scope.
		w, cleanup := testMwebWallet(t,
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), false)
		defer cleanup()

		scope := w.preferredMwebScope(0)
		if scope != waddrmgr.KeyScopeMwebLegacy {
			t.Errorf("got %v, want KeyScopeMwebLegacy", scope)
		}
	})
}

// TestImportMwebAddressDerivation verifies that importing the known test
// scan secret + spend pubkey produces stealth addresses that match the
// Jade/Core test vectors from mweb_compat_test.go.
func TestImportMwebAddressDerivation(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

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

	// Verify that the imported account's keychain produces stealth addresses
	// matching the known test vectors from waddrmgr/mweb_compat_test.go.
	scopedMgr, _ := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb)

	type expectedAddr struct {
		index  uint32
		scanA  string
		spendB string
	}
	expected := []expectedAddr{
		{0, "03acdfb78943f3330437760e37731828f9abd626a72df16fc7cd968df13b7465ab",
			"039ed000ed69ca7d593f09ad4a373200bc9711261aab56efc05b92a5eab434f864"},
		{1, "02516a92f3bc6025bce2911e67140dded34ac1f938df0148c9b478e577b5054e42",
			"035dad4451e4f2bfd56bb0266a12d92af4749d43a452471e52a437b9d7bbb157c1"},
	}

	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		kc, err := scopedMgr.LoadMwebKeychain(ns, props.AccountNumber)
		if err != nil {
			return err
		}

		for _, tc := range expected {
			addr := kc.Address(tc.index)
			gotA := hex.EncodeToString(addr.Scan[:])
			gotB := hex.EncodeToString(addr.Spend[:])
			if gotA != tc.scanA {
				t.Errorf("index %d scan A mismatch:\n  got:  %s\n  want: %s",
					tc.index, gotA, tc.scanA)
			}
			if gotB != tc.spendB {
				t.Errorf("index %d spend B mismatch:\n  got:  %s\n  want: %s",
					tc.index, gotB, tc.spendB)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("address verification: %v", err)
	}
}

// TestImportMwebScanKeyRejectsZeroFingerprint verifies that a zero
// master key fingerprint is rejected.
func TestImportMwebScanKeyRejectsZeroFingerprint(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

	var scanSecret [32]byte
	copy(scanSecret[:], mustDecodeHex(hwScanSecretHex))
	var spendPubKey [33]byte
	copy(spendPubKey[:], mustDecodeHex(hwSpendPubKeyHex))

	_, err := w.ImportMwebScanKey(
		"hw-mweb", scanSecret, spendPubKey, 0, 100,
	)
	if err == nil {
		t.Fatal("expected error for zero fingerprint, got nil")
	}
}

// TestImportMwebScanKeyDuplicateName verifies that importing with a
// duplicate account name fails.
func TestImportMwebScanKeyDuplicateName(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

	var scanSecret [32]byte
	copy(scanSecret[:], mustDecodeHex(hwScanSecretHex))
	var spendPubKey [33]byte
	copy(spendPubKey[:], mustDecodeHex(hwSpendPubKeyHex))

	// First import should succeed.
	_, err := w.ImportMwebScanKey(
		"hw-mweb", scanSecret, spendPubKey, hwFingerprint, 100,
	)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Second import with same name should fail.
	copy(scanSecret[:], mustDecodeHex(hwScanSecretHex))
	_, err = w.ImportMwebScanKey(
		"hw-mweb", scanSecret, spendPubKey, hwFingerprint, 100,
	)
	if err == nil {
		t.Fatal("expected duplicate name error, got nil")
	}
}

// TestImportMwebScanKeyRecoveryWindowSurvivesRestart verifies that the
// recovery window is made durable: ExternalKeyCount is persisted to the
// DB via ExtendExternalAddresses during import, and a rebuilt keypool
// (simulating restart) starts from that persisted index.
func TestImportMwebScanKeyRecoveryWindowSurvivesRestart(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

	var scanSecret [32]byte
	copy(scanSecret[:], mustDecodeHex(hwScanSecretHex))
	var spendPubKey [33]byte
	copy(spendPubKey[:], mustDecodeHex(hwSpendPubKeyHex))

	props, err := w.ImportMwebScanKey(
		"hw-mweb", scanSecret, spendPubKey, hwFingerprint, 200,
	)
	if err != nil {
		t.Fatalf("ImportMwebScanKey: %v", err)
	}

	// Verify ExternalKeyCount matches the recovery window.
	if props.ExternalKeyCount != 200 {
		t.Errorf("ExternalKeyCount: got %d, want 200",
			props.ExternalKeyCount)
	}

	scopedMgr, _ := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb)
	key := skmAccount{skm: scopedMgr, account: props.AccountNumber}

	// Simulate restart: delete the in-memory keypool, invalidate account
	// cache, then rebuild via getOrInitMwebKeyPool (the path that runs
	// on syncWithChain after restart).
	w.mwebKeyPoolsMu.Lock()
	delete(w.mwebKeyPools, key)
	w.mwebKeyPoolsMu.Unlock()
	scopedMgr.InvalidateAccountCache(props.AccountNumber)

	// Rebuild the pool from DB state (no custom pool size — this is
	// the restart path with default 1000 lookahead).
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)

		// Re-read account properties to get the scan secret.
		reloadedProps, err := scopedMgr.AccountProperties(
			ns, props.AccountNumber,
		)
		if err != nil {
			return err
		}

		// Verify ExternalKeyCount survived the cache invalidation.
		if reloadedProps.ExternalKeyCount != 200 {
			t.Errorf("ExternalKeyCount after restart: got %d, want 200",
				reloadedProps.ExternalKeyCount)
		}

		scanPriv, err := reloadedProps.AccountScanKey.ECPrivKey()
		if err != nil {
			return err
		}
		scanKeyBytes := scanPriv.Key.Bytes()

		ma := &mwebAccount{
			skmAccount: key,
			scanSecret: (*mw.SecretKey)(&scanKeyBytes),
		}
		kp, err := w.getOrInitMwebKeyPool(key, ns, ma)
		if err != nil {
			return err
		}

		// The rebuilt pool should start from the persisted index (200),
		// not from 0. This proves the recovery window is durable.
		if kp.index != 200 {
			t.Errorf("rebuilt pool index: got %d, want 200", kp.index)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("rebuild pool: %v", err)
	}
}

// TestImportMwebScanKeyAtomicPoolInit verifies that getOrInitMwebKeyPool's
// atomic behavior prevents a second import or concurrent init from
// overwriting an already-existing pool for the same account key.
func TestImportMwebScanKeyAtomicPoolInit(t *testing.T) {
	t.Parallel()
	w, cleanup := testMwebImportWallet(t)
	defer cleanup()

	var scanSecret [32]byte
	copy(scanSecret[:], mustDecodeHex(hwScanSecretHex))
	var spendPubKey [33]byte
	copy(spendPubKey[:], mustDecodeHex(hwSpendPubKeyHex))

	// Import the account — this creates a pool via getOrInitMwebKeyPool.
	props, err := w.ImportMwebScanKey(
		"hw-mweb", scanSecret, spendPubKey, hwFingerprint, 50,
	)
	if err != nil {
		t.Fatalf("ImportMwebScanKey: %v", err)
	}

	scopedMgr, _ := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb)
	key := skmAccount{skm: scopedMgr, account: props.AccountNumber}

	// Get the pool created by the import.
	kp1, ok := w.getMwebKeyPool(key)
	if !ok {
		t.Fatal("keypool not initialized after import")
	}

	// Simulate what a concurrent chain notification would do: call
	// getOrInitMwebKeyPool for the SAME account key. The atomic helper
	// should return the existing pool, not create a new one.
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		scanPriv, _ := props.AccountScanKey.ECPrivKey()
		scanKeyBytes := scanPriv.Key.Bytes()
		ma := &mwebAccount{
			skmAccount: key,
			scanSecret: (*mw.SecretKey)(&scanKeyBytes),
		}
		kp2, err := w.getOrInitMwebKeyPool(key, ns, ma)
		if err != nil {
			return err
		}
		// Must be the exact same pool pointer — not a new allocation.
		if kp2 != kp1 {
			t.Error("getOrInitMwebKeyPool created a new pool instead " +
				"of returning the existing one")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("getOrInitMwebKeyPool: %v", err)
	}
}

// TestImportMwebScanKeyOnLockedWallet verifies that importing on a
// locked wallet that already has KeyScopeMweb (from creation) succeeds
// for the DB write but the import overall works since
// NewMwebAccountWatchingOnly doesn't need the wallet to be unlocked
// (it encrypts with cryptoKeyPub, not cryptoKeyPriv).
func TestImportMwebScanKeyOnLockedWallet(t *testing.T) {
	t.Parallel()

	// Create a post-activation wallet (has KeyScopeMweb).
	dir := t.TempDir()
	seed, err := hdkeychain.GenerateSeed(hdkeychain.MinSeedBytes)
	if err != nil {
		t.Fatalf("GenerateSeed: %v", err)
	}

	pubPass := []byte("hello")
	privPass := []byte("world")

	loader := NewLoader(
		&chaincfg.TestNet4Params, dir, true, defaultDBTimeout, 250,
		WithWalletSyncRetryInterval(10*time.Millisecond),
	)
	w, err := loader.CreateNewWallet(pubPass, privPass, seed,
		time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreateNewWallet: %v", err)
	}
	w.chainClient = &mockChainClient{}
	defer w.db.Close()

	// Wallet is locked (never unlocked). KeyScopeMweb exists from creation.
	// NewMwebAccountWatchingOnly encrypts with cryptoKeyPub which is
	// available without unlock, so this should succeed.
	var scanSecret [32]byte
	copy(scanSecret[:], mustDecodeHex(hwScanSecretHex))
	var spendPubKey [33]byte
	copy(spendPubKey[:], mustDecodeHex(hwSpendPubKeyHex))

	props, err := w.ImportMwebScanKey(
		"hw-locked", scanSecret, spendPubKey, hwFingerprint, 100,
	)
	if err != nil {
		t.Fatalf("ImportMwebScanKey on locked wallet: %v", err)
	}
	if !props.IsWatchOnly {
		t.Error("expected watch-only")
	}
	if props.KeyScope != waddrmgr.KeyScopeMweb {
		t.Errorf("scope: got %v, want %v", props.KeyScope, waddrmgr.KeyScopeMweb)
	}
}

// TestImportMwebScanKeyOnWatchOnlyWallet verifies the scope-absent
// auto-create path: a watch-only wallet has NO scoped managers at all,
// so FetchScopedKeyManager(KeyScopeMweb) returns ErrScopeNotFound.
// The import must create the scope via NewScopedKeyManager (which on a
// watch-only wallet creates the scope bucket but no accounts), then
// create the imported account as account 0 (fetchLastAccount returns
// 0xFFFFFFFF, +1 wraps to 0).
func TestImportMwebScanKeyOnWatchOnlyWallet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pubPass := []byte("hello")

	loader := NewLoader(
		&chaincfg.TestNet4Params, dir, true, defaultDBTimeout, 250,
		WithWalletSyncRetryInterval(10*time.Millisecond),
	)
	w, err := loader.CreateNewWatchingOnlyWallet(pubPass,
		time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreateNewWatchingOnlyWallet: %v", err)
	}
	w.chainClient = &mockChainClient{}
	defer w.db.Close()

	// Verify KeyScopeMweb does NOT exist yet.
	_, err = w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb)
	if err == nil {
		t.Fatal("expected KeyScopeMweb to NOT exist on watch-only wallet")
	}

	var scanSecret [32]byte
	copy(scanSecret[:], mustDecodeHex(hwScanSecretHex))
	var spendPubKey [33]byte
	copy(spendPubKey[:], mustDecodeHex(hwSpendPubKeyHex))

	props, err := w.ImportMwebScanKey(
		"hw-watchonly", scanSecret, spendPubKey, hwFingerprint, 50,
	)
	if err != nil {
		t.Fatalf("ImportMwebScanKey on watch-only wallet: %v", err)
	}

	// On watch-only wallet: no seed-derived account 0 exists, so the
	// imported account wraps to account 0 (fetchLastAccount returns
	// 0xFFFFFFFF, +1 = 0).
	if props.AccountNumber != 0 {
		t.Errorf("account number: got %d, want 0 (wrapped)", props.AccountNumber)
	}
	if !props.IsWatchOnly {
		t.Error("expected watch-only")
	}
	if props.KeyScope != waddrmgr.KeyScopeMweb {
		t.Errorf("scope: got %v, want %v", props.KeyScope, waddrmgr.KeyScopeMweb)
	}
	if props.MasterKeyFingerprint != hwFingerprint {
		t.Errorf("fingerprint: got %x, want %x",
			props.MasterKeyFingerprint, hwFingerprint)
	}

	// Verify the scope was created.
	_, err = w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb)
	if err != nil {
		t.Fatalf("KeyScopeMweb should exist after import: %v", err)
	}

	// Verify the keychain loads and produces correct addresses.
	scopedMgr, _ := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb)
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		kc, err := scopedMgr.LoadMwebKeychain(ns, props.AccountNumber)
		if err != nil {
			return err
		}
		gotScan := hex.EncodeToString(kc.Scan[:])
		if gotScan != hwScanSecretHex {
			t.Errorf("scan mismatch:\n  got:  %s\n  want: %s",
				gotScan, hwScanSecretHex)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("LoadMwebKeychain: %v", err)
	}
}

// Suppress unused import warnings.
var _ = hdkeychain.HardenedKeyStart
var _ = chaincfg.MainNetParams
