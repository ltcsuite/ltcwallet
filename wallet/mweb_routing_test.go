package wallet

import (
	"testing"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/ltcutil/hdkeychain"
	"github.com/ltcsuite/ltcd/ltcutil/psbt"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
	"github.com/ltcsuite/ltcwallet/walletdb"
)

// testMwebWallet creates a wallet with a specific birthday. If unlock
// is true, it unlocks the wallet (which also triggers MWEB scope
// migration for legacy wallets).
func testMwebWallet(t *testing.T, birthday time.Time, unlock bool) (*Wallet, func()) {
	t.Helper()

	dir := t.TempDir()
	seed, err := hdkeychain.GenerateSeed(hdkeychain.MinSeedBytes)
	if err != nil {
		t.Fatalf("unable to create seed: %v", err)
	}

	pubPass := []byte("hello")
	privPass := []byte("world")

	loader := NewLoader(
		&chaincfg.TestNet4Params, dir, true, defaultDBTimeout, 250,
		WithWalletSyncRetryInterval(10*time.Millisecond),
	)
	w, err := loader.CreateNewWallet(pubPass, privPass, seed, birthday)
	if err != nil {
		t.Fatalf("unable to create wallet: %v", err)
	}
	w.chainClient = &mockChainClient{}

	if unlock {
		if err := w.Unlock(privPass, time.After(10*time.Minute)); err != nil {
			t.Fatalf("unable to unlock wallet: %v", err)
		}
	}

	cleanup := func() {
		w.db.Close()
	}
	return w, cleanup
}

func TestPreferredMwebScope(t *testing.T) {
	t.Parallel()

	t.Run("new wallet has standard scope", func(t *testing.T) {
		t.Parallel()
		w, cleanup := testMwebWallet(t, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), false)
		defer cleanup()

		scope := w.preferredMwebScope(0)
		if scope != waddrmgr.KeyScopeMweb {
			t.Errorf("expected KeyScopeMweb, got %v", scope)
		}
	})

	t.Run("legacy wallet before unlock has legacy scope", func(t *testing.T) {
		t.Parallel()
		// Pre-activation, not unlocked → no migration yet
		w, cleanup := testMwebWallet(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), false)
		defer cleanup()

		scope := w.preferredMwebScope(0)
		if scope != waddrmgr.KeyScopeMwebLegacy {
			t.Errorf("expected KeyScopeMwebLegacy before unlock, got %v", scope)
		}
	})

	t.Run("legacy wallet after unlock prefers standard", func(t *testing.T) {
		t.Parallel()
		// Pre-activation, unlocked → migration runs, standard scope added
		w, cleanup := testMwebWallet(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), true)
		defer cleanup()

		scope := w.preferredMwebScope(0)
		if scope != waddrmgr.KeyScopeMweb {
			t.Errorf("expected KeyScopeMweb after unlock/migration, got %v", scope)
		}
	})
}

func TestResolveMwebScopeAndAccount(t *testing.T) {
	t.Parallel()

	t.Run("new wallet resolves default", func(t *testing.T) {
		t.Parallel()
		w, cleanup := testMwebWallet(t, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), false)
		defer cleanup()

		scope, account, err := w.ResolveMwebScopeAndAccount("default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if scope != waddrmgr.KeyScopeMweb {
			t.Errorf("expected KeyScopeMweb, got %v", scope)
		}
		if account != 0 {
			t.Errorf("expected account 0, got %d", account)
		}
	})

	t.Run("legacy wallet after unlock routes to standard", func(t *testing.T) {
		t.Parallel()
		w, cleanup := testMwebWallet(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), true)
		defer cleanup()

		scope, account, err := w.ResolveMwebScopeAndAccount("default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if scope != waddrmgr.KeyScopeMweb {
			t.Errorf("expected KeyScopeMweb after migration, got %v", scope)
		}
		if account != 0 {
			t.Errorf("expected account 0, got %d", account)
		}
	})

	t.Run("nonexistent name returns error on new wallet", func(t *testing.T) {
		t.Parallel()
		w, cleanup := testMwebWallet(t, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), false)
		defer cleanup()

		_, _, err := w.ResolveMwebScopeAndAccount("nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent account name")
		}
	})

	t.Run("nonexistent name on legacy wallet does not fall through", func(t *testing.T) {
		t.Parallel()
		// After unlock (migration), both scopes exist. Looking up a
		// name that doesn't exist in the legacy scope must NOT fall
		// through to the standard scope.
		w, cleanup := testMwebWallet(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), true)
		defer cleanup()

		_, _, err := w.ResolveMwebScopeAndAccount("bogus")
		if err == nil {
			t.Fatal("expected error for nonexistent name in legacy scope, " +
				"should not fall through to standard")
		}
	})
}

// TestCreateOutputInfoMwebPaths calls the production createOutputInfo
// function with real managed MWEB addresses and verifies the emitted
// BIP32 derivation paths.
func TestCreateOutputInfoMwebPaths(t *testing.T) {
	t.Parallel()

	const H = hdkeychain.HardenedKeyStart

	t.Run("standard scope output path", func(t *testing.T) {
		t.Parallel()

		// Post-activation wallet → has KeyScopeMweb (standard)
		w, cleanup := testMwebWallet(t,
			time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), true)
		defer cleanup()

		out := generateMwebOutputInfo(t, w, waddrmgr.KeyScopeMweb)
		path := out.Bip32Derivation[0].Bip32Path

		// Standard: [0', 100', branch, index] — 4 elements
		if len(path) != 4 {
			t.Fatalf("expected 4-element path, got %d: %v", len(path), path)
		}
		if path[0] != H {
			t.Errorf("path[0]: expected 0' (%d), got %d", H, path[0])
		}
		if path[1] != 100+H {
			t.Errorf("path[1]: expected 100' (%d), got %d", 100+H, path[1])
		}
		// path[2]=branch, path[3]=index — not hardened
		if path[2] >= H {
			t.Errorf("path[2] (branch) should not be hardened: %d", path[2])
		}
	})

	t.Run("legacy scope output path", func(t *testing.T) {
		t.Parallel()

		// Pre-activation wallet → has KeyScopeMwebLegacy
		w, cleanup := testMwebWallet(t,
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), false)
		defer cleanup()

		out := generateMwebOutputInfo(t, w, waddrmgr.KeyScopeMwebLegacy)
		path := out.Bip32Derivation[0].Bip32Path

		// Legacy: [1000', 2', account', branch, index] — 5 elements
		if len(path) != 5 {
			t.Fatalf("expected 5-element path, got %d: %v", len(path), path)
		}
		if path[0] != 1000+H {
			t.Errorf("path[0]: expected 1000' (%d), got %d", 1000+H, path[0])
		}
		if path[1] != 2+H {
			t.Errorf("path[1]: expected 2' (%d), got %d", 2+H, path[1])
		}
		if path[2] != H { // account 0, hardened
			t.Errorf("path[2]: expected 0' (%d), got %d", H, path[2])
		}
	})
}

// generateMwebOutputInfo generates an MWEB address from the given scope,
// creates a TxOut for it, and calls the production createOutputInfo.
func generateMwebOutputInfo(t *testing.T, w *Wallet,
	scope waddrmgr.KeyScope) *psbt.POutput {

	t.Helper()

	var managedAddr waddrmgr.ManagedPubKeyAddress
	err := walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		scopedMgr, err := w.Manager.FetchScopedKeyManager(scope)
		if err != nil {
			return err
		}
		addrs, err := scopedMgr.NextExternalAddresses(ns, 0, 1)
		if err != nil {
			return err
		}
		var ok bool
		managedAddr, ok = addrs[0].(waddrmgr.ManagedPubKeyAddress)
		if !ok {
			t.Fatal("expected ManagedPubKeyAddress")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NextExternalAddresses: %v", err)
	}

	script := managedAddr.Address().ScriptAddress()
	txOut := wire.NewTxOut(100000, script)
	out, err := createOutputInfo(txOut, managedAddr)
	if err != nil {
		t.Fatalf("createOutputInfo: %v", err)
	}
	if len(out.Bip32Derivation) == 0 {
		t.Fatal("no Bip32Derivation in output")
	}
	if out.StealthAddress == nil {
		t.Fatal("expected StealthAddress to be set for MWEB output")
	}
	return out
}

// TestMwebMigrationIdempotency verifies that calling ensureMwebStandardScope
// multiple times on an already-migrated wallet produces no errors and
// does not create duplicate scopes.
func TestMwebMigrationIdempotency(t *testing.T) {
	t.Parallel()

	// Legacy wallet, unlocked → migration runs during unlock
	w, cleanup := testMwebWallet(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), true)
	defer cleanup()

	// Verify standard scope exists after first migration
	if _, err := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb); err != nil {
		t.Fatalf("standard scope missing after first migration: %v", err)
	}

	// Run migration again — must be idempotent
	if err := w.ensureMwebStandardScope(); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}

	// Standard scope still present
	if _, err := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb); err != nil {
		t.Fatalf("standard scope missing after second migration: %v", err)
	}

	// Third time for good measure
	if err := w.ensureMwebStandardScope(); err != nil {
		t.Fatalf("third migration failed: %v", err)
	}
}

// TestMwebDualScopeAddresses verifies that after migration, both MWEB
// scopes exist and produce different valid MWEB addresses.
func TestMwebDualScopeAddresses(t *testing.T) {
	t.Parallel()

	// Legacy wallet, unlocked → both scopes exist
	w, cleanup := testMwebWallet(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), true)
	defer cleanup()

	legacyMgr, err := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMwebLegacy)
	if err != nil {
		t.Fatalf("legacy scope missing: %v", err)
	}
	stdMgr, err := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb)
	if err != nil {
		t.Fatalf("standard scope missing: %v", err)
	}

	var legacyAddr, stdAddr ltcutil.Address
	err = walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		addrs, err := legacyMgr.NextExternalAddresses(ns, 0, 1)
		if err != nil {
			return err
		}
		legacyAddr = addrs[0].Address()

		addrs, err = stdMgr.NextExternalAddresses(ns, 0, 1)
		if err != nil {
			return err
		}
		stdAddr = addrs[0].Address()
		return nil
	})
	if err != nil {
		t.Fatalf("address derivation: %v", err)
	}

	// Both must be MWEB addresses
	if _, ok := legacyAddr.(*ltcutil.AddressMweb); !ok {
		t.Error("legacy address is not AddressMweb")
	}
	if _, ok := stdAddr.(*ltcutil.AddressMweb); !ok {
		t.Error("standard address is not AddressMweb")
	}

	// Must be different (different derivation paths → different keys)
	if legacyAddr.EncodeAddress() == stdAddr.EncodeAddress() {
		t.Error("legacy and standard addresses must differ")
	}
}

// TestMwebMigrationCreatesKeyPool verifies that after migration,
// the standard MWEB scope has an initialized keypool. The legacy
// scope's keypool is created during syncWithChain, not migration,
// so we only check the standard keypool here.
func TestMwebMigrationCreatesKeyPool(t *testing.T) {
	t.Parallel()

	w, cleanup := testMwebWallet(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), true)
	defer cleanup()

	var stdFound bool
	err := w.forEachMwebKeyPool(func(key skmAccount, kp *mwebKeyPool) error {
		if key.skm.Scope() == waddrmgr.KeyScopeMweb {
			stdFound = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("forEachMwebKeyPool: %v", err)
	}
	if !stdFound {
		t.Error("no keypool for standard MWEB scope after migration")
	}
}

// TestMwebNewWalletHasNoLegacyScope verifies that a wallet created
// after the activation date has only the standard scope, not legacy.
func TestMwebNewWalletHasNoLegacyScope(t *testing.T) {
	t.Parallel()

	w, cleanup := testMwebWallet(t, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), true)
	defer cleanup()

	if _, err := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMweb); err != nil {
		t.Fatalf("standard scope should exist: %v", err)
	}
	if _, err := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeMwebLegacy); err == nil {
		t.Error("legacy scope should NOT exist for post-activation wallet")
	}

	// Migration should be a no-op (no legacy scope to migrate from)
	if err := w.ensureMwebStandardScope(); err != nil {
		t.Fatalf("ensureMwebStandardScope on new wallet failed: %v", err)
	}
}
