// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package waddrmgr

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/ltcutil/hdkeychain"
	"github.com/ltcsuite/ltcwallet/walletdb"
	_ "github.com/ltcsuite/ltcwallet/walletdb/bdb"
)

var (
	// seed is the master seed used throughout the tests.
	seed = []byte{
		0x2a, 0x64, 0xdf, 0x08, 0x5e, 0xef, 0xed, 0xd8, 0xbf,
		0xdb, 0xb3, 0x31, 0x76, 0xb5, 0xba, 0x2e, 0x62, 0xe8,
		0xbe, 0x8b, 0x56, 0xc8, 0x83, 0x77, 0x95, 0x59, 0x8b,
		0xb6, 0xc4, 0x40, 0xc0, 0x64,
	}

	rootKey, _ = hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)

	pubPassphrase   = []byte("_DJr{fL4H0O}*-0\n:V1izc)(6BomK")
	privPassphrase  = []byte("81lUHXnOMZ@?XXd7O9xyDIWIbXX-lj")
	pubPassphrase2  = []byte("-0NV4P~VSJBWbunw}%<Z]fuGpbN[ZI")
	privPassphrase2 = []byte("~{<]08%6!-?2s<$(8$8:f(5[4/!/{Y")

	// fastScrypt are parameters used throughout the tests to speed up the
	// scrypt operations.
	fastScrypt = &FastScryptOptions

	// waddrmgrNamespaceKey is the namespace key for the waddrmgr package.
	waddrmgrNamespaceKey = []byte("waddrmgrNamespace")

	// expectedAddrs is the list of all expected addresses generated from the
	// seed.
	expectedAddrs = []expectedAddr{
		{
			address:     "LPAqss8BTNUeFi4Giwrop9fhRNFtL1Dwiw",
			addressHash: hexToBytes("2b49ecd0cf72006173e6e95acf416b6735b5f889"),
			internal:    false,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("02d8f88468c5a2e8e1815faf555f59cbd1979e3dbdf823f80c271b6fb70d2d519b"),
			privKey:     hexToBytes("c27d6581b92785834b381fa697c4b0ffc4574b495743722e0acb7601b1b68b99"),
			privKeyWIF:  "T9a3GiNEnz5x7N6EaQGzjwQV8Ae31QbN5yb3PLdDqZfb9bfE6SyP",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          0,
				Index:           0,
			},
		},
		{
			address:     "LgGAPxGrf59Y87t2PpYPqQXFdMryPK9KCs",
			addressHash: hexToBytes("e6c59a1542138d1bf08f45cd18899557cf56b356"),
			internal:    false,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("02b9c175b908624f8a8eaac227d0e8c77c0eec327b8c512ad1b8b7a4b5b676971f"),
			privKey:     hexToBytes("18f3b191019e83878a81557abebb2afda199e31d22e150d8bf4df4561671be6c"),
			privKeyWIF:  "T3tUpTvBYt7UWCtEbYLhcaerVdrA3Kq1iz7BVZHZupfdmiGLocoq",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          0,
				Index:           1,
			},
		},
		{
			address:     "LKiQw9Wtnx6hHQ41bHpZzKRi1vvYYvZ4iu",
			addressHash: hexToBytes("0561e9373986965b647a57a09718e9c050215cfe"),
			internal:    false,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("0329faddf1254d490d6add49e2b08cf52b561038c72baec0edb3cfacff71ff1021"),
			privKey:     hexToBytes("ccb8f6305b73136b363644b647f6efc0fd27b6b7d9c11c7e560662ed38db7b34"),
			privKeyWIF:  "T9uvwzPj2V1hS4SaSp5z9HxoHV3iNw4T68W4cYnTAgdH6GTohbqf",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          0,
				Index:           2,
			},
		},
		{
			address:     "LMgaVwNUn99bWX8jMC6wcWFhZEtyGciuqM",
			addressHash: hexToBytes("1af950be02584ca230b7078cec0cfd38dd71b468"),
			internal:    false,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("03d738324e2f0ce42e46975d7f8c7117c1670e3d7912b0291aea452add99674774"),
			privKey:     hexToBytes("d6bc8ff768814fede2adcdb74826bd846924341b3862e3b6e31cdc084e992940"),
			privKeyWIF:  "TAFPyjFipMN3bJabga5y6hfUWxNMUFAdQdHyuDnenTdf2dcQM4G7",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          0,
				Index:           3,
			},
		},
		{
			address:     "Legghu1aGJrWKxBtQTjwK454Ai17iMTBUc",
			addressHash: hexToBytes("d578a267a7174c6ba7f76b0ab2397ce0ba0c5c3c"),
			internal:    false,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("03a917acd5cd5b6f544b43f1921a35677e4d5320e5d2add2056039b4b44fdf905e"),
			privKey:     hexToBytes("8563ade061110e03aee50695ffc5cb1c06c8310bde0a3674257c853c966968c0"),
			privKeyWIF:  "T7XGY3CyNBkYhttjNcMFF4o9TEKaBZ2nrnr4xJFbS15SB1T6Na4A",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          0,
				Index:           4,
			},
		},
		{
			address:     "LPWKz9J9nXadcfhzbmPfHjjZWkr4TtJ61D",
			addressHash: hexToBytes("2ef94abb9ee8f785d087c3ec8d6ee467e92d0d0a"),
			internal:    true,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("020a1290b997c0a234a95213962e7edcb761c7360f0230f698a1a3e71c37047bb0"),
			privKey:     hexToBytes("fe4f855fcf059ec6ddf7b25f63b19aa49c771d1fcb9850b68ae3d65e20657a60"),
			privKeyWIF:  "TBaKjUE1wJnJj1zhTa9uM524exYy1kgqNzsPsN16EKDJwQpH1dY4",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          1,
				Index:           0,
			},
		},
		{
			address:     "LeXmY4UD6akMAjMT1uUDARu1hkyrBvVKUq",
			addressHash: hexToBytes("d3c8ec46891f599bfeaa4c25918bfb3d46ea334c"),
			internal:    true,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("03f79bbde32af42dde98195f011d95982602fcd0dab657fe4a1f49f9d5ada1e02d"),
			privKey:     hexToBytes("bfef521317c65b018ae7e6d7ecc3aa700d5d0f7ea84d567be9270382d0b5e3e6"),
			privKeyWIF:  "T9V5DDsvBqBwpEEzp6zz4C4rCkr3zurRvaSd1ZubWyjRF7Eh5tdc",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          1,
				Index:           1,
			},
		},
		{
			address:     "LMbeoBLcGzbzdiNAdh6DDbaZB2RBXEDUgT",
			addressHash: hexToBytes("1a0ad2a04fde3b2afe068057591e1871c289c4b8"),
			internal:    true,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("023ded84afe4fe91b52b45c3deb26fd263f749cbc27747dc964dae9e0739cbc579"),
			privKey:     hexToBytes("f506dffd4494c24006df7a35f3291f7ca0297a1a431557a1339bfed6f48738ca"),
			privKeyWIF:  "TBGH3EheoDPBnHXzaJHffBKe9bFhjiBJ3DiZpK4MPGcM9RvA8st4",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          1,
				Index:           2,
			},
		},
		{
			address:     "LUm4ENbktQAsWRM5LrbzcNr4tB3WKocS1B",
			addressHash: hexToBytes("689b0249c628265215fd1de6142d5d5594eb8dc2"),
			internal:    true,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("030f1e79f06824e10a259914ec310528bb2d5b8d6356341fe9dff55498591af6af"),
			privKey:     hexToBytes("b3629de8ef6a275b4ffae41aa2bbbc2952eb92282ea6402435abbb010ecc1fb8"),
			privKeyWIF:  "T94gK1wRCMWN6RYpchZ8GZx29J3XzMvp47sJMqWQdeq5GziH148W",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          1,
				Index:           3,
			},
		},
		{
			address:     "Lcq4RzM8vQEUfH2f2EVzKjcBKz5TFhDjp9",
			addressHash: hexToBytes("c11dd8a3577978807a0453febedee2994a6144d4"),
			internal:    true,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("0317d7182e26b6ca3e0f3db531c474b9cab7a763a75eabff2e14ac92f62a793238"),
			privKey:     hexToBytes("ca747a7ef815ea0dbe68655272cecbfbd65f2a109019a9ed28e0d3dcaffe05c3"),
			privKeyWIF:  "T9qXJKuHUoNCE4yCYZx6UYaD9cHEioWob2g8qKhB9979V5ybp7kT",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          1,
				Index:           4,
			},
		},
	}

	// expectedExternalAddrs is the list of expected external addresses
	// generated from the seed
	expectedExternalAddrs = expectedAddrs[:5]

	// expectedInternalAddrs is the list of expected internal addresses
	// generated from the seed
	expectedInternalAddrs = expectedAddrs[5:]

	// defaultDBTimeout specifies the timeout value when opening the wallet
	// database.
	defaultDBTimeout = 10 * time.Second
)

// checkManagerError ensures the passed error is a ManagerError with an error
// code that matches the passed  error code.
func checkManagerError(t *testing.T, testName string, gotErr error,
	wantErrCode ErrorCode) bool {

	merr, ok := gotErr.(ManagerError)
	if !ok {
		t.Errorf("%s: unexpected error type - got %T, want %T",
			testName, gotErr, ManagerError{})
		return false
	}
	if merr.ErrorCode != wantErrCode {
		t.Errorf("%s: unexpected error code - got %s (%s), want %s",
			testName, merr.ErrorCode, merr.Description, wantErrCode)
		return false
	}

	return true
}

// hexToBytes is a wrapper around hex.DecodeString that panics if there is an
// error.  It MUST only be used with hard coded values in the tests.
func hexToBytes(origHex string) []byte {
	buf, err := hex.DecodeString(origHex)
	if err != nil {
		panic(err)
	}
	return buf
}

func emptyDB(t *testing.T) (tearDownFunc func(), db walletdb.DB) {
	dirName, err := os.MkdirTemp("", "mgrtest")
	if err != nil {
		t.Fatalf("Failed to create db temp dir: %v", err)
	}
	dbPath := filepath.Join(dirName, "mgrtest.db")
	db, err = walletdb.Create("bdb", dbPath, true, defaultDBTimeout)
	if err != nil {
		_ = os.RemoveAll(dirName)
		t.Fatalf("createDbNamespace: unexpected error: %v", err)
	}
	tearDownFunc = func() {
		db.Close()
		_ = os.RemoveAll(dirName)
	}
	return
}

// setupManager creates a new address manager and returns a teardown function
// that should be invoked to ensure it is closed and removed upon completion.
func setupManager(t *testing.T) (tearDownFunc func(), db walletdb.DB, mgr *Manager) {
	// Create a new manager in a temp directory.
	dirName, err := os.MkdirTemp("", "mgrtest")
	if err != nil {
		t.Fatalf("Failed to create db temp dir: %v", err)
	}
	dbPath := filepath.Join(dirName, "mgrtest.db")
	db, err = walletdb.Create("bdb", dbPath, true, defaultDBTimeout)
	if err != nil {
		_ = os.RemoveAll(dirName)
		t.Fatalf("createDbNamespace: unexpected error: %v", err)
	}
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns, err := tx.CreateTopLevelBucket(waddrmgrNamespaceKey)
		if err != nil {
			return err
		}
		err = Create(
			ns, rootKey, pubPassphrase, privPassphrase,
			&chaincfg.MainNetParams, fastScrypt, time.Time{},
		)
		if err != nil {
			return err
		}
		mgr, err = Open(ns, pubPassphrase, &chaincfg.MainNetParams)
		return err
	})
	if err != nil {
		db.Close()
		_ = os.RemoveAll(dirName)
		t.Fatalf("Failed to create Manager: %v", err)
	}
	tearDownFunc = func() {
		mgr.Close()
		db.Close()
		_ = os.RemoveAll(dirName)
	}
	return tearDownFunc, db, mgr
}
