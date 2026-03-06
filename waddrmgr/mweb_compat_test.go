package waddrmgr

import (
	"encoding/hex"
	"testing"

	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/ltcutil/hdkeychain"
	"github.com/ltcsuite/ltcd/ltcutil/mweb"
	"github.com/ltcsuite/ltcd/ltcutil/mweb/mw"
	"github.com/ltcsuite/ltcwallet/walletdb"
)

// Test vectors generated from Litecoin Core's MWEB key derivation
// (scriptpubkeyman.cpp:1749-1764) using the same test seed as common_test.go.
//
// Derivation path:
//
//	m/0'/100'/0' => scan key
//	m/0'/100'/1' => spend key
var (
	expectedScanSecret  = "b3c91b7291c2e1e06d4a93f3dc32404aef9927db8e794c01a7b4de18a397c338"
	expectedScanPubKey  = "02cd7e29e31bf0c07281d3c591fe3dbe4375b911cc6038ec5d1be82099d6c482f5"
	expectedSpendSecret = "2fe1982b98c0b68c0839421c8a0a0a67ef3198c746ab8e6d09101eb7396a44d8"
	expectedSpendPubKey = "03e3908af70085b458020e64aaa5c9a4b8ff382d42af0875c8145db6a30db9cad2"

	// Legacy scan secret from the old (broken) derivation path
	// m/1000'/2'/0'/0'. Used to verify the legacy path hasn't regressed.
	expectedLegacyScanSecret = "880543ec7faf7434b79f11a568d3c388a6374976aefc943a237f5bc8b3860a24"

	// Stealth addresses derived using BLAKE3-based subaddress formula:
	//
	//	m_i = BLAKE3('A' || i_le32 || scan_secret)
	//	B_i = spend_pubkey + m_i*G
	//	A_i = B_i * scan_secret
	//	spend_key_i = spend_secret + m_i
	expectedStealthAddresses = []struct {
		index    uint32
		scanA    string // A_i (33-byte compressed hex)
		spendB   string // B_i (33-byte compressed hex)
		spendKey string // spend_key_i (32-byte scalar hex)
		encoded  string // bech32m ltcmweb1... address
	}{
		{
			index:    0,
			scanA:    "03acdfb78943f3330437760e37731828f9abd626a72df16fc7cd968df13b7465ab",
			spendB:   "039ed000ed69ca7d593f09ad4a373200bc9711261aab56efc05b92a5eab434f864",
			spendKey: "4076801c591afd06d2823c79858e4c93a6a69ad31ddca673e457437229c74b18",
			encoded:  "ltcmweb1qqwkdldufg0enxpphwc8rwucc9ru6h43x5uklzm78ektgmufmw3j6kqu76qqw66w204vn7zddfgmnyq9ujugjvx4t2mhuqkuj5h4tgd8cvs6gg076",
		},
		{
			index:    1,
			scanA:    "02516a92f3bc6025bce2911e67140dded34ac1f938df0148c9b478e577b5054e42",
			spendB:   "035dad4451e4f2bfd56bb0266a12d92af4749d43a452471e52a437b9d7bbb157c1",
			spendKey: "edf509d17a9ebe744dfb77650a4cc39fa90dc6a758c9d33107b2c4a501fa98ab",
			encoded:  "ltcmweb1qqfgk4yhnh3szt08zjy0xw9qdmmf54s0e8r0szjxfk3uw2aa4q48yyq6a44z9re8jhl2khvpxdgfdj2h5wjw58fzjgu099fphh8tmhv2hcygfr2nl",
		},
		{
			index:    10,
			scanA:    "03f864dcaa67a74542ff9b5adc27ad2f9002626baa91372e9aee7737ecfec18cca",
			spendB:   "027223f04b94617ec15d7d5c135c42242af64b2129f17080e6b10756bb6ec10073",
			spendKey: "bb33118206a8f8ec35874f78ae5676365bc4e9480d4600947ebb5e049ca4d3e4",
			encoded:  "ltcmweb1qq0uxfh92v7n52shlndddcfad97gqycnt42gnwt56aemn0m87cxxv5qnjy0cyh9rp0mq46l2uzdwyyfp27e9jz203wzqwdvg826akasgqwvgs2kze",
		},
	}
)

// TestMwebStandardDerivationMatchesCore verifies that KeyScopeMweb produces
// the same scan/spend keys as Litecoin Core's m/0'/100'/0' and m/0'/100'/1'
// derivation path.
func TestMwebStandardDerivationMatchesCore(t *testing.T) {
	t.Parallel()

	// Verify test vectors are self-consistent with hdkeychain.
	const H = hdkeychain.HardenedKeyStart
	acctKey, err := rootKey.DeriveNonStandard(H) // m/0'
	if err != nil {
		t.Fatalf("derive m/0': %v", err)
	}
	mwebChainKey, err := acctKey.DeriveNonStandard(H + 100) // m/0'/100'
	if err != nil {
		t.Fatalf("derive m/0'/100': %v", err)
	}
	scanExt, err := mwebChainKey.DeriveNonStandard(H) // m/0'/100'/0'
	if err != nil {
		t.Fatalf("derive m/0'/100'/0': %v", err)
	}
	spendExt, err := mwebChainKey.DeriveNonStandard(H + 1) // m/0'/100'/1'
	if err != nil {
		t.Fatalf("derive m/0'/100'/1': %v", err)
	}

	scanPriv, _ := scanExt.ECPrivKey()
	spendPriv, _ := spendExt.ECPrivKey()

	scanKeyBytes := scanPriv.Key.Bytes()
	spendKeyBytes := spendPriv.Key.Bytes()
	if got := hex.EncodeToString(scanKeyBytes[:]); got != expectedScanSecret {
		t.Fatalf("hdkeychain scan secret mismatch:\n  got:  %s\n  want: %s", got, expectedScanSecret)
	}
	if got := hex.EncodeToString(spendKeyBytes[:]); got != expectedSpendSecret {
		t.Fatalf("hdkeychain spend secret mismatch:\n  got:  %s\n  want: %s", got, expectedSpendSecret)
	}

	// Create a wallet, add the standard MWEB scope, and verify
	// the ScopedKeyManager produces the same keys.
	teardown, db, mgr := setupManager(t)
	defer teardown()

	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		return mgr.Unlock(ns, privPassphrase)
	})
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	mwebSchema := ScopeAddrSchema{
		ExternalAddrType: Mweb,
		InternalAddrType: Mweb,
	}

	var scopedMgr *ScopedKeyManager
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		var err error
		scopedMgr, err = mgr.NewScopedKeyManager(ns, KeyScopeMweb, mwebSchema)
		return err
	})
	if err != nil {
		t.Fatalf("NewScopedKeyManager: %v", err)
	}

	var keychain *mweb.Keychain
	err = walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		var err error
		keychain, err = scopedMgr.LoadMwebKeychain(ns, 0)
		return err
	})
	if err != nil {
		t.Fatalf("LoadMwebKeychain: %v", err)
	}
	if keychain == nil {
		t.Fatal("LoadMwebKeychain returned nil")
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"scan secret", hex.EncodeToString(keychain.Scan[:]), expectedScanSecret},
		{"spend secret", hex.EncodeToString(keychain.Spend[:]), expectedSpendSecret},
		{"scan pubkey", hex.EncodeToString(keychain.Scan.PubKey()[:]), expectedScanPubKey},
		{"spend pubkey", hex.EncodeToString(keychain.Spend.PubKey()[:]), expectedSpendPubKey},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("ScopedKeyManager %s mismatch:\n  got:  %s\n  want: %s",
				c.name, c.got, c.want)
		}
	}
}

// TestMwebStandardSubaddresses verifies that stealth addresses derived from
// the standard MWEB scope match the expected values, including encoded
// ltcmweb1... addresses and per-index spend keys.
func TestMwebStandardSubaddresses(t *testing.T) {
	t.Parallel()

	scanBytes, _ := hex.DecodeString(expectedScanSecret)
	spendBytes, _ := hex.DecodeString(expectedSpendSecret)
	scanSecret := (*mw.SecretKey)(scanBytes)
	spendSecret := (*mw.SecretKey)(spendBytes)

	keychain := &mweb.Keychain{
		Scan:  scanSecret,
		Spend: spendSecret,
	}

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

		// Verify per-index spend key
		sk := keychain.SpendKey(tc.index)
		gotSK := hex.EncodeToString(sk[:])
		if gotSK != tc.spendKey {
			t.Errorf("index %d: spend_key mismatch:\n  got:  %s\n  want: %s",
				tc.index, gotSK, tc.spendKey)
		}

		// Verify encoded ltcmweb1... address
		encoded := ltcutil.NewAddressMweb(addr, &chaincfg.MainNetParams)
		if got := encoded.EncodeAddress(); got != tc.encoded {
			t.Errorf("index %d: encoded address mismatch:\n  got:  %s\n  want: %s",
				tc.index, got, tc.encoded)
		}
	}
}

// TestMwebStandardVsLegacyDifference verifies that the legacy MWEB scope
// produces a known, specific scan key that differs from the standard scope.
// This catches regressions in both directions.
func TestMwebStandardVsLegacyDifference(t *testing.T) {
	t.Parallel()

	const H = hdkeychain.HardenedKeyStart

	// Legacy path: m/1000'/2'/0'/0' (scan)
	purposeKey, _ := rootKey.DeriveNonStandard(H + 1000)
	coinTypeKey, _ := purposeKey.DeriveNonStandard(H + 2)
	acctKey, _ := deriveAccountKey(coinTypeKey, 0)
	legacyScan, _ := acctKey.Derive(H)

	legacyScanPriv, _ := legacyScan.ECPrivKey()
	legacyScanBytes := legacyScanPriv.Key.Bytes()
	legacyScanHex := hex.EncodeToString(legacyScanBytes[:])

	// Verify legacy produces its known expected value (not just "different")
	if legacyScanHex != expectedLegacyScanSecret {
		t.Errorf("legacy scan secret regressed:\n  got:  %s\n  want: %s",
			legacyScanHex, expectedLegacyScanSecret)
	}

	// Sanity: must differ from standard
	if legacyScanHex == expectedScanSecret {
		t.Error("legacy scan key must NOT match standard scan key")
	}
}

// TestMwebStandardNewAccountRestriction verifies that the standard MWEB scope
// rejects creation of accounts beyond account 0.
func TestMwebStandardNewAccountRestriction(t *testing.T) {
	t.Parallel()

	teardown, db, mgr := setupManager(t)
	defer teardown()

	err := walletdb.View(db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		return mgr.Unlock(ns, privPassphrase)
	})
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	mwebSchema := ScopeAddrSchema{
		ExternalAddrType: Mweb,
		InternalAddrType: Mweb,
	}

	var scopedMgr *ScopedKeyManager
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		var err error
		scopedMgr, err = mgr.NewScopedKeyManager(ns, KeyScopeMweb, mwebSchema)
		return err
	})
	if err != nil {
		t.Fatalf("NewScopedKeyManager: %v", err)
	}

	// Attempt to create account 1 — should fail
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		_, err := scopedMgr.NewAccount(ns, "test-account-1")
		return err
	})
	if err == nil {
		t.Fatal("expected error creating account > 0 on standard MWEB scope, got nil")
	}
	if !IsError(err, ErrAccountNumTooHigh) {
		t.Fatalf("expected ErrAccountNumTooHigh, got: %v", err)
	}
}
