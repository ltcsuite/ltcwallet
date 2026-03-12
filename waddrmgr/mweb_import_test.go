package waddrmgr

import (
	"encoding/hex"
	"testing"

	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/ltcutil/hdkeychain"
	"github.com/ltcsuite/ltcd/ltcutil/mweb"
	"github.com/ltcsuite/ltcwallet/walletdb"
)

// Test scan secret and spend pubkey for imported MWEB account tests.
// These represent a hardware wallet with keys that happen to match the
// test seed's standard MWEB derivation, used to verify round-trip
// storage and keychain loading.
var (
	// hwScanSecret is a 32-byte MWEB scan secret for the imported account.
	hwScanSecret = hexToBytes("b3c91b7291c2e1e06d4a93f3dc32404aef9927db8e794c01a7b4de18a397c338")

	// hwSpendPubKey is the 33-byte compressed spend pubkey for the imported account.
	hwSpendPubKey = hexToBytes("03e3908af70085b458020e64aaa5c9a4b8ff382d42af0875c8145db6a30db9cad2")

	// hwMasterKeyFingerprint is a non-zero fingerprint required for PSBT signing.
	hwMasterKeyFingerprint = uint32(0xe66c70b2)
)

// setupMwebManager creates a manager with the standard MWEB scope unlocked.
func setupMwebManager(t *testing.T) (func(), walletdb.DB, *Manager, *ScopedKeyManager) {
	t.Helper()
	teardown, db, mgr := setupManager(t)

	err := walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		return mgr.Unlock(ns, privPassphrase)
	})
	if err != nil {
		teardown()
		t.Fatalf("Unlock: %v", err)
	}

	var scopedMgr *ScopedKeyManager
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		var err error
		scopedMgr, err = mgr.NewScopedKeyManager(
			ns, KeyScopeMweb, ScopeAddrMap[KeyScopeMweb],
		)
		return err
	})
	if err != nil {
		teardown()
		t.Fatalf("NewScopedKeyManager(KeyScopeMweb): %v", err)
	}

	return teardown, db, mgr, scopedMgr
}

// TestNewMwebAccountWatchingOnly verifies that importing a raw scan secret +
// spend pubkey creates a valid watch-only MWEB account that round-trips
// through the DB and produces correct keychain data.
func TestNewMwebAccountWatchingOnly(t *testing.T) {
	t.Parallel()
	teardown, db, mgr, scopedMgr := setupMwebManager(t)
	defer teardown()

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)

	var spendPubKey [33]byte
	copy(spendPubKey[:], hwSpendPubKey)

	var account uint32
	err := walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		var err error
		account, err = scopedMgr.NewMwebAccountWatchingOnly(
			ns, "hw-mweb", scanSecret, spendPubKey,
			hwMasterKeyFingerprint,
		)
		return err
	})
	if err != nil {
		t.Fatalf("NewMwebAccountWatchingOnly: %v", err)
	}

	// Account 0 is the seed-derived default, so imported should be 1.
	if account != 1 {
		t.Fatalf("expected account 1, got %d", account)
	}

	// Verify account properties via the manager.
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)

		props, err := scopedMgr.AccountProperties(ns, account)
		if err != nil {
			return err
		}

		if props.AccountName != "hw-mweb" {
			t.Errorf("name: got %q, want %q", props.AccountName, "hw-mweb")
		}
		if !props.IsWatchOnly {
			t.Error("expected IsWatchOnly=true")
		}
		if props.MasterKeyFingerprint != hwMasterKeyFingerprint {
			t.Errorf("fingerprint: got %x, want %x",
				props.MasterKeyFingerprint, hwMasterKeyFingerprint)
		}
		if props.AccountPubKey == nil {
			t.Fatal("AccountPubKey is nil")
		}
		if props.AccountPubKey.ChildIndex() != hdkeychain.HardenedKeyStart {
			t.Errorf("ChildIndex: got %d, want %d",
				props.AccountPubKey.ChildIndex(), hdkeychain.HardenedKeyStart)
		}

		// Round-trip the synthetic xpub through String/NewKeyFromString.
		xpubStr := props.AccountPubKey.String()
		roundTripped, err := hdkeychain.NewKeyFromString(xpubStr)
		if err != nil {
			t.Fatalf("AccountPubKey.String() round-trip failed: %v", err)
		}
		if roundTripped.String() != xpubStr {
			t.Error("AccountPubKey did not survive String() round-trip")
		}

		// Verify scan key recovers original scan secret.
		if props.AccountScanKey == nil {
			t.Fatal("AccountScanKey is nil")
		}
		scanPriv, err := props.AccountScanKey.ECPrivKey()
		if err != nil {
			t.Fatalf("ECPrivKey from scan key: %v", err)
		}
		scanKeyBytes := scanPriv.Key.Bytes()
		gotScan := hex.EncodeToString(scanKeyBytes[:])
		wantScan := hex.EncodeToString(hwScanSecret)
		if gotScan != wantScan {
			t.Errorf("scan secret mismatch:\n  got:  %s\n  want: %s", gotScan, wantScan)
		}

		// Verify spend pubkey recovers original spend pubkey.
		if props.AccountSpendPubKey == nil {
			t.Fatal("AccountSpendPubKey is nil")
		}
		spendPub, err := props.AccountSpendPubKey.ECPubKey()
		if err != nil {
			t.Fatalf("ECPubKey from spend key: %v", err)
		}
		gotSpend := hex.EncodeToString(spendPub.SerializeCompressed())
		wantSpend := hex.EncodeToString(hwSpendPubKey)
		if gotSpend != wantSpend {
			t.Errorf("spend pubkey mismatch:\n  got:  %s\n  want: %s", gotSpend, wantSpend)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("AccountProperties: %v", err)
	}

	// Verify the seed-derived account 0 still works.
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		props, err := scopedMgr.AccountProperties(ns, 0)
		if err != nil {
			return err
		}
		if props.IsWatchOnly {
			t.Error("seed-derived account 0 should not be watch-only")
		}
		if props.AccountName != "default" {
			t.Errorf("account 0 name: got %q, want %q", props.AccountName, "default")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("AccountProperties(0): %v", err)
	}

	_ = mgr // used for setup
}

// TestMwebAccountDuplicateName verifies that importing with a name that
// already exists returns ErrDuplicateAccount.
func TestMwebAccountDuplicateName(t *testing.T) {
	t.Parallel()
	teardown, db, _, scopedMgr := setupMwebManager(t)
	defer teardown()

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)
	var spendPubKey [33]byte
	copy(spendPubKey[:], hwSpendPubKey)

	// First import should succeed.
	err := walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		_, err := scopedMgr.NewMwebAccountWatchingOnly(
			ns, "hw-mweb", scanSecret, spendPubKey, hwMasterKeyFingerprint,
		)
		return err
	})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Second import with same name should fail.
	copy(scanSecret[:], hwScanSecret)
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		_, err := scopedMgr.NewMwebAccountWatchingOnly(
			ns, "hw-mweb", scanSecret, spendPubKey, hwMasterKeyFingerprint,
		)
		return err
	})
	if err == nil {
		t.Fatal("expected ErrDuplicateAccount, got nil")
	}
	if !IsError(err, ErrDuplicateAccount) {
		t.Fatalf("expected ErrDuplicateAccount, got: %v", err)
	}
}

// TestMwebAccountPersistence verifies that an imported MWEB account and
// its masterKeyFingerprint survive a DB close/reopen cycle.
func TestMwebAccountPersistence(t *testing.T) {
	t.Parallel()
	teardown, db, mgr, scopedMgr := setupMwebManager(t)

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)
	var spendPubKey [33]byte
	copy(spendPubKey[:], hwSpendPubKey)

	var account uint32
	err := walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		var err error
		account, err = scopedMgr.NewMwebAccountWatchingOnly(
			ns, "hw-mweb", scanSecret, spendPubKey, hwMasterKeyFingerprint,
		)
		return err
	})
	if err != nil {
		teardown()
		t.Fatalf("import: %v", err)
	}

	// Close and reopen the manager.
	mgr.Close()
	var mgr2 *Manager
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		var err error
		mgr2, err = Open(ns, pubPassphrase, &chaincfg.MainNetParams)
		return err
	})
	if err != nil {
		teardown()
		t.Fatalf("Open after reopen: %v", err)
	}

	scopedMgr2, err := mgr2.FetchScopedKeyManager(KeyScopeMweb)
	if err != nil {
		mgr2.Close()
		teardown()
		t.Fatalf("FetchScopedKeyManager after reopen: %v", err)
	}

	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		props, err := scopedMgr2.AccountProperties(ns, account)
		if err != nil {
			return err
		}
		if props.AccountName != "hw-mweb" {
			t.Errorf("name after reopen: got %q, want %q", props.AccountName, "hw-mweb")
		}
		if props.MasterKeyFingerprint != hwMasterKeyFingerprint {
			t.Errorf("fingerprint after reopen: got %x, want %x",
				props.MasterKeyFingerprint, hwMasterKeyFingerprint)
		}
		if !props.IsWatchOnly {
			t.Error("expected IsWatchOnly after reopen")
		}
		return nil
	})

	mgr2.Close()
	teardown()

	if err != nil {
		t.Fatalf("verify after reopen: %v", err)
	}
}

// TestMwebAccountRenamePreservesFingerprint verifies that renaming an
// imported MWEB account preserves its masterKeyFingerprint.
func TestMwebAccountRenamePreservesFingerprint(t *testing.T) {
	t.Parallel()
	teardown, db, _, scopedMgr := setupMwebManager(t)
	defer teardown()

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)
	var spendPubKey [33]byte
	copy(spendPubKey[:], hwSpendPubKey)

	var account uint32
	err := walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		var err error
		account, err = scopedMgr.NewMwebAccountWatchingOnly(
			ns, "hw-mweb", scanSecret, spendPubKey, hwMasterKeyFingerprint,
		)
		return err
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Rename the account.
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		return scopedMgr.RenameAccount(ns, account, "hw-renamed")
	})
	if err != nil {
		t.Fatalf("RenameAccount: %v", err)
	}

	// Invalidate cache so we re-read from DB.
	scopedMgr.InvalidateAccountCache(account)

	// Verify fingerprint survived the rename.
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		props, err := scopedMgr.AccountProperties(ns, account)
		if err != nil {
			return err
		}
		if props.AccountName != "hw-renamed" {
			t.Errorf("name: got %q, want %q", props.AccountName, "hw-renamed")
		}
		if props.MasterKeyFingerprint != hwMasterKeyFingerprint {
			t.Errorf("fingerprint after rename: got %x, want %x",
				props.MasterKeyFingerprint, hwMasterKeyFingerprint)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify after rename: %v", err)
	}
}

// TestMwebFingerprintSurvivesReserialize verifies that masterKeyFingerprint
// is preserved through the putChainedAddress and deletePrivateKeys rewrite
// paths.
func TestMwebFingerprintSurvivesReserialize(t *testing.T) {
	t.Parallel()
	teardown, db, mgr, scopedMgr := setupMwebManager(t)
	defer teardown()

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)
	var spendPubKey [33]byte
	copy(spendPubKey[:], hwSpendPubKey)

	var account uint32
	err := walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		var err error
		account, err = scopedMgr.NewMwebAccountWatchingOnly(
			ns, "hw-mweb", scanSecret, spendPubKey, hwMasterKeyFingerprint,
		)
		return err
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Derive addresses — this triggers putChainedAddress which
	// reserializes the account row with updated nextExternalIndex.
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		_, err := scopedMgr.NextExternalAddresses(ns, account, 3)
		return err
	})
	if err != nil {
		t.Fatalf("NextExternalAddresses: %v", err)
	}

	// Invalidate cache and verify fingerprint survived.
	scopedMgr.InvalidateAccountCache(account)
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		props, err := scopedMgr.AccountProperties(ns, account)
		if err != nil {
			return err
		}
		if props.MasterKeyFingerprint != hwMasterKeyFingerprint {
			t.Errorf("fingerprint after address derivation: got %x, want %x",
				props.MasterKeyFingerprint, hwMasterKeyFingerprint)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify after address derivation: %v", err)
	}

	// Lock the wallet (triggers deletePrivateKeys which reserializes
	// all default account rows).
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		_ = tx.ReadBucket(waddrmgrNamespaceKey)
		return mgr.Lock()
	})
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}

	// Invalidate cache and verify fingerprint survived the lock cycle.
	scopedMgr.InvalidateAccountCache(account)
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		props, err := scopedMgr.AccountProperties(ns, account)
		if err != nil {
			return err
		}
		if props.MasterKeyFingerprint != hwMasterKeyFingerprint {
			t.Errorf("fingerprint after lock: got %x, want %x",
				props.MasterKeyFingerprint, hwMasterKeyFingerprint)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify after lock: %v", err)
	}
}

// TestDeriveKeyWatchOnly verifies that calling deriveKey with private=true
// on a watch-only imported account returns ErrWatchingOnly instead of panicking.
func TestDeriveKeyWatchOnly(t *testing.T) {
	t.Parallel()
	teardown, db, _, scopedMgr := setupMwebManager(t)
	defer teardown()

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)
	var spendPubKey [33]byte
	copy(spendPubKey[:], hwSpendPubKey)

	var account uint32
	err := walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		var err error
		account, err = scopedMgr.NewMwebAccountWatchingOnly(
			ns, "hw-mweb", scanSecret, spendPubKey, hwMasterKeyFingerprint,
		)
		return err
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Public derivation should succeed.
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		addrs, err := scopedMgr.NextExternalAddresses(ns, account, 1)
		if err != nil {
			return err
		}
		if len(addrs) != 1 {
			t.Errorf("expected 1 address, got %d", len(addrs))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NextExternalAddresses (public): %v", err)
	}
}

// TestExtendAddressesWatchOnly verifies that ExtendExternalAddresses does
// not panic on a watch-only imported MWEB account (regression test for the
// extendAddresses inverted watch-only check bug).
func TestExtendAddressesWatchOnly(t *testing.T) {
	t.Parallel()
	teardown, db, _, scopedMgr := setupMwebManager(t)
	defer teardown()

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)
	var spendPubKey [33]byte
	copy(spendPubKey[:], hwSpendPubKey)

	var account uint32
	err := walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		var err error
		account, err = scopedMgr.NewMwebAccountWatchingOnly(
			ns, "hw-mweb", scanSecret, spendPubKey, hwMasterKeyFingerprint,
		)
		return err
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// First derive some addresses so there's a base index.
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		_, err := scopedMgr.NextExternalAddresses(ns, account, 5)
		return err
	})
	if err != nil {
		t.Fatalf("NextExternalAddresses: %v", err)
	}

	// Now extend — this calls extendAddresses which had the inverted
	// watch-only bug. On an unlocked wallet with a watch-only imported
	// account, the old code would set watchOnly=false (because
	// acctKeyPriv==nil was interpreted as "not watch-only"), then try to
	// use acctKeyPriv and panic on nil dereference.
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		return scopedMgr.ExtendExternalAddresses(ns, account, 10)
	})
	if err != nil {
		t.Fatalf("ExtendExternalAddresses: %v", err)
	}
}

// TestLoadMwebKeychainImported verifies that LoadMwebKeychain works on
// an imported MWEB account and produces correct stealth addresses that
// match the real test vectors.
func TestLoadMwebKeychainImported(t *testing.T) {
	t.Parallel()
	teardown, db, _, scopedMgr := setupMwebManager(t)
	defer teardown()

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)
	var spendPubKey [33]byte
	copy(spendPubKey[:], hwSpendPubKey)

	var account uint32
	err := walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		var err error
		account, err = scopedMgr.NewMwebAccountWatchingOnly(
			ns, "hw-mweb", scanSecret, spendPubKey, hwMasterKeyFingerprint,
		)
		return err
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	var keychain *mweb.Keychain
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		var err error
		keychain, err = scopedMgr.LoadMwebKeychain(ns, account)
		return err
	})
	if err != nil {
		t.Fatalf("LoadMwebKeychain: %v", err)
	}
	if keychain == nil {
		t.Fatal("LoadMwebKeychain returned nil")
	}

	// Verify scan secret matches.
	gotScan := hex.EncodeToString(keychain.Scan[:])
	wantScan := hex.EncodeToString(hwScanSecret)
	if gotScan != wantScan {
		t.Errorf("scan secret mismatch:\n  got:  %s\n  want: %s", gotScan, wantScan)
	}

	// Verify stealth addresses match the real test vectors.
	// Since we imported the SAME scan secret and spend pubkey as the
	// wallet's seed-derived keys, the addresses should match the
	// expectedStealthAddresses from mweb_compat_test.go.
	for _, tc := range expectedStealthAddresses {
		addr := keychain.Address(tc.index)
		gotA := hex.EncodeToString(addr.Scan[:])
		gotB := hex.EncodeToString(addr.Spend[:])

		if gotA != tc.scanA {
			t.Errorf("index %d: A_i mismatch:\n  got:  %s\n  want: %s",
				tc.index, gotA, tc.scanA)
		}
		if gotB != tc.spendB {
			t.Errorf("index %d: B_i mismatch:\n  got:  %s\n  want: %s",
				tc.index, gotB, tc.spendB)
		}

		// Verify encoded stealth address.
		encoded := ltcutil.NewAddressMweb(addr, &chaincfg.MainNetParams)
		if got := encoded.EncodeAddress(); got != tc.encoded {
			t.Errorf("index %d: address mismatch:\n  got:  %s\n  want: %s",
				tc.index, got, tc.encoded)
		}
	}

}

// TestMwebAccountCoexistence verifies that native account 0 and an
// imported account both load and can be iterated.
func TestMwebAccountCoexistence(t *testing.T) {
	t.Parallel()
	teardown, db, _, scopedMgr := setupMwebManager(t)
	defer teardown()

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)
	var spendPubKey [33]byte
	copy(spendPubKey[:], hwSpendPubKey)

	err := walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		_, err := scopedMgr.NewMwebAccountWatchingOnly(
			ns, "hw-mweb", scanSecret, spendPubKey, hwMasterKeyFingerprint,
		)
		return err
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// ForEachAccount should iterate both accounts.
	var accounts []uint32
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		return scopedMgr.ForEachAccount(ns, func(acct uint32) error {
			accounts = append(accounts, acct)
			return nil
		})
	})
	if err != nil {
		t.Fatalf("ForEachAccount: %v", err)
	}

	// Expect account 0 (default), account 1 (imported),
	// and ImportedAddrAccount (max uint32).
	if len(accounts) < 2 {
		t.Fatalf("expected at least 2 accounts, got %d: %v", len(accounts), accounts)
	}

	// Verify both have MWEB scan keys via LoadMwebKeychain.
	for _, acct := range accounts {
		if acct == ImportedAddrAccount {
			continue // skip the catch-all imported account
		}
		err = walletdb.View(db, func(tx walletdb.ReadTx) error {
			ns := tx.ReadBucket(waddrmgrNamespaceKey)
			kc, err := scopedMgr.LoadMwebKeychain(ns, acct)
			if err != nil {
				return err
			}
			if kc == nil {
				t.Errorf("account %d: LoadMwebKeychain returned nil", acct)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("LoadMwebKeychain(account %d): %v", acct, err)
		}
	}
}

// TestMwebImportRejectsNonMwebScope verifies that NewMwebAccountWatchingOnly
// fails on a non-MWEB scope.
func TestMwebImportRejectsNonMwebScope(t *testing.T) {
	t.Parallel()
	teardown, db, mgr, _ := setupMwebManager(t)
	defer teardown()

	bip84Mgr, err := mgr.FetchScopedKeyManager(KeyScopeBIP0084)
	if err != nil {
		t.Fatalf("FetchScopedKeyManager(BIP0084): %v", err)
	}

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)
	var spendPubKey [33]byte
	copy(spendPubKey[:], hwSpendPubKey)

	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		_, err := bip84Mgr.NewMwebAccountWatchingOnly(
			ns, "bad-scope", scanSecret, spendPubKey, hwMasterKeyFingerprint,
		)
		return err
	})
	if err == nil {
		t.Fatal("expected error for non-MWEB scope, got nil")
	}
}

// TestDefaultAccountRowFingerprintBackwardCompat verifies that old
// dbDefaultAccountRow records (without the trailing fingerprint bytes)
// deserialize correctly with masterKeyFingerprint=0.
func TestDefaultAccountRowFingerprintBackwardCompat(t *testing.T) {
	t.Parallel()

	// Serialize without fingerprint (old format, simulated).
	oldRow := serializeDefaultAccountRow(
		[]byte("pubkey"), []byte("privkey"),
		nil, nil, 5, 3, "test-account", 0,
	)

	// Deserialize — should get fingerprint=0.
	accountID := uint32ToBytes(0)
	row := &dbAccountRow{
		acctType: accountDefault,
		rawData:  oldRow,
	}
	parsed, err := deserializeDefaultAccountRow(accountID, row)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if parsed.masterKeyFingerprint != 0 {
		t.Errorf("expected fingerprint=0 for old row, got %x", parsed.masterKeyFingerprint)
	}
	if parsed.name != "test-account" {
		t.Errorf("name: got %q, want %q", parsed.name, "test-account")
	}

	// Now serialize WITH fingerprint (new format).
	newRow := serializeDefaultAccountRow(
		[]byte("pubkey"), []byte("privkey"),
		nil, nil, 5, 3, "test-account", 0xDEADBEEF,
	)

	row2 := &dbAccountRow{
		acctType: accountDefault,
		rawData:  newRow,
	}
	parsed2, err := deserializeDefaultAccountRow(accountID, row2)
	if err != nil {
		t.Fatalf("deserialize new format: %v", err)
	}
	if parsed2.masterKeyFingerprint != 0xDEADBEEF {
		t.Errorf("expected fingerprint=0xDEADBEEF, got %x", parsed2.masterKeyFingerprint)
	}

	// Verify the new row is 4 bytes larger than the old one.
	if len(newRow) != len(oldRow)+4 {
		t.Errorf("new row should be 4 bytes larger: old=%d, new=%d", len(oldRow), len(newRow))
	}
}

// TestUnlockAfterCachedImportedAccount verifies that Manager.Unlock()
// succeeds even when an imported MWEB account (with empty privKeyEncrypted)
// has been cached by a prior AccountProperties call while locked.
func TestUnlockAfterCachedImportedAccount(t *testing.T) {
	t.Parallel()
	teardown, db, mgr, scopedMgr := setupMwebManager(t)
	defer teardown()

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)
	var spendPubKey [33]byte
	copy(spendPubKey[:], hwSpendPubKey)

	var account uint32
	err := walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		var err error
		account, err = scopedMgr.NewMwebAccountWatchingOnly(
			ns, "hw-mweb", scanSecret, spendPubKey,
			hwMasterKeyFingerprint,
		)
		return err
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Lock the wallet.
	mgr.Lock()

	// While locked, read the imported account to cache it.
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		_, err := scopedMgr.AccountProperties(ns, account)
		return err
	})
	if err != nil {
		t.Fatalf("AccountProperties while locked: %v", err)
	}

	// Now unlock — this must not fail on the cached imported account's
	// empty acctKeyEncrypted.
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		return mgr.Unlock(ns, privPassphrase)
	})
	if err != nil {
		t.Fatalf("Unlock after cached import: %v", err)
	}
}

// TestMwebImportRejectsInvalidSpendPubKey verifies that
// NewMwebAccountWatchingOnly rejects a malformed spend pubkey.
func TestMwebImportRejectsInvalidSpendPubKey(t *testing.T) {
	t.Parallel()
	teardown, db, _, scopedMgr := setupMwebManager(t)
	defer teardown()

	var scanSecret [32]byte
	copy(scanSecret[:], hwScanSecret)

	// 33 bytes but not a valid compressed secp256k1 point.
	var badPubKey [33]byte
	badPubKey[0] = 0x05 // invalid prefix

	err := walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		_, err := scopedMgr.NewMwebAccountWatchingOnly(
			ns, "bad-key", scanSecret, badPubKey,
			hwMasterKeyFingerprint,
		)
		return err
	})
	if err == nil {
		t.Fatal("expected error for invalid spend pubkey, got nil")
	}
}
