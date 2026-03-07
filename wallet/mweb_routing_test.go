package wallet

import (
	"testing"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/ltcutil/hdkeychain"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
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
