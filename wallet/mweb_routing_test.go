package wallet

import (
	"testing"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg"
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
