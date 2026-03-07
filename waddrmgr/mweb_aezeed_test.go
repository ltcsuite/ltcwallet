package waddrmgr

import (
	"encoding/hex"
	"testing"

	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/ltcutil/hdkeychain"
	"github.com/ltcsuite/ltcd/ltcutil/mweb"
	"github.com/ltcsuite/ltcd/ltcutil/mweb/mw"
)

// Test vectors using a 16-byte aezeed entropy (the minimum seed size for
// hdkeychain.NewMaster). Two aezeed mnemonics share this entropy but have
// different birthdays — day 6295 (pre-activation, legacy scope) and day 6296
// (post-activation, standard scope). This verifies that the same entropy
// produces the correct, different MWEB keys depending on the derivation path.
//
// Entropy: 81b637d86359e6960de795e41e0b4cfd
//
// Day 6295 mnemonic (salt1):
//
//	abstract come tower olive rough stove metal amazing hospital shrug
//	abuse uphold duty upset garage donate gaze phone success suggest
//	drink voice analyst fossil
//
// Day 6296 mnemonic (salt1):
//
//	absorb lens elbow toss enter truly drastic fetch couch organ also
//	argue sport tell fuel spike fitness logic success suggest drink
//	rent bundle primary
var (
	aezeedEntropy, _ = hex.DecodeString("81b637d86359e6960de795e41e0b4cfd")

	// Standard scope: m/0'/100'/0' (scan), m/0'/100'/1' (spend)
	aezeedStandardScanSecret  = "4cc5814d8e5988082bb8cbcf34ab39649b0a0227af914cbb0bd20490a82d9654"
	aezeedStandardSpendSecret = "84a815bd07d723681bf17afc9f6a24e3d68d25dbc775b8d480338af3f0138a20"
	aezeedStandardScanPubKey  = "022c52c49d9457634f4cc5b6ce61b104f4592d7144d0fccdd55cd18f2d77383ef8"
	aezeedStandardSpendPubKey = "032b5b6fe5dc5d9aa22e0c6936fbaceaea89e3efc419cdd1af3b5d0a32499bff51"

	aezeedStandardAddresses = []struct {
		index    uint32
		scanA    string
		spendB   string
		spendKey string
		encoded  string
	}{
		{
			index:    0,
			scanA:    "0234fe52cc3a6a54ebb21f09d71c213a0a40de2e4295e8d9eac93e076cbd04b3e4",
			spendB:   "0223f382a7d4cde5e2696da084ea8ca917b160370921cfa09ab2c464cbfa2d614f",
			spendKey: "cb79899b3b7b6117fa5dc07daafc21303df170c467b03eb6acd32a1fc2029038",
			encoded:  "ltcmweb1qqg60u5kv8f49f6ajruyaw8pp8g9yph3wg2273k02eylqwm9aqje7gq3r7wp204xduh3xjmdqsn4ge2ghk9srwzfpe7sf4vkyvn9l5ttpfu2aw9nx",
		},
		{
			index:    1,
			scanA:    "024fda8b496ddb142c45e0c55458dec1ea477aa0364aef8b1336da1ae98ac48469",
			spendB:   "03d75a7599e000153dc84dc76d2aea11105c769a1c0f2c4bd550102df7b06673f3",
			spendKey: "fb2ecb239c5b77e5596366992a2594598240c97024872f200309dd4ae792564b",
			encoded:  "ltcmweb1qqf8a4z6fdhd3gtz9urz4gkx7c84yw74qxe9wlzcnxmdp46v2cjzxjq7htf6encqqz57usnw8d54w5ygst3mf58q0939a25qs9hmmqenn7vqnvxva",
		},
		{
			index:    10,
			scanA:    "03bf62f23cf727e0c4bf3a609921b421beef637500f5ae649761512711369d83c4",
			spendB:   "02af7b34b3e84e8a8503b451db94543a8dfe7a5984aab022d6c224ad6c1617cdc7",
			spendKey: "5813ffd1a3d98e2ac25263125ebd7834188e777bbfdc22925af7aaafb2e0ba0a",
			encoded:  "ltcmweb1qqwlk9u3u7un7p39l8fsfjgd5yxlw7cm4qr66ueyhv9gjwyfknkpugq400v6t86zw32zs8dz3mw29gw5dlea9np92kq3dds3y44kpv97dcu6lh5eh",
		},
	}

	// Legacy scope: m/1000'/2'/0'/0' (scan), m/1000'/2'/0'/1' (spend)
	aezeedLegacyScanSecret  = "3b438a0ec1a35fc75af20cb5e3aa944431f4fc4d09daf01d2bc48a1650a1ceb8"
	aezeedLegacySpendSecret = "e3115237771a7d3a9b5d5e194ee349a3b10f07ccf8ed4f8d43a9321d5e373954"
	aezeedLegacyScanPubKey  = "035e9efb5ca5f579dfda4e35160cce6b41a9c0fe8cd5e2603f14170f8f01d5131d"
	aezeedLegacySpendPubKey = "02ffd022e22d3019bb0f2f10a2273eb2739238f0372683b783d91e34534b5ae676"

	aezeedLegacyAddresses = []struct {
		index    uint32
		scanA    string
		spendB   string
		spendKey string
		encoded  string
	}{
		{
			index:    0,
			scanA:    "02847d8d59f212d9f043590203716942d6b1adb207367cc340b7da43523caf1248",
			spendB:   "02d1540a81bbf474c2a8f253c72e485a3067c7c69b0033f48fb408bd7c626a2e3c",
			spendKey: "d80e09497f99ff93bd05cf79d682a215212515ceb0d8af8263671689cf558fec",
			encoded:  "ltcmweb1qq2z8mr2e7gfdnuzrtypqxutfgtttrtdjqum8es6qkldyx53u4ufysqk32s9grwl5wnp23ujncuhysk3svlrudxcqx06gldqgh47xy63w8sgdawts",
		},
		{
			index:    1,
			scanA:    "03ba8894c5ce085a777359a0a819460c54cb64d41afd9479ad537be714186f215a",
			spendB:   "03e56dc0af942c948771b945e685a81935e00abfb00221d3b9d23113a5fedda282",
			spendKey: "562b569c7dc73a5fb2e97fe6423c3ffa73c2fa0af966851e5282d9c06ca9847d",
			encoded:  "ltcmweb1qqwag39x9ecy95amntxs2sx2xp32vkex5rt7eg7dd2da7w9qcdus45ql9dhq2l9pvjjrhrw29u6z6sxf4uq9tlvqzy8fmn533zwjlahdzsgz53u7r",
		},
		{
			index:    10,
			scanA:    "03bc763ec6801ecfbdb5bb6613fc4a13dc92331e365ba884b5d1b2bcaf30b917f6",
			spendB:   "03825a353cac80e3abb14b9af358d871bad99a212be634fbbed07e0882486f1801",
			spendKey: "8945b5dd2c50ac9e94c1ee3c173db8dc38de31f92ceab980f0edba3f0ee5eb89",
			encoded:  "ltcmweb1qqw78v0kxsq0vl0d4hdnp8lz2z0wfyvc7xed63p946xeteteshytlvquztg6netyquw4mzju67dvdsud6mxdzz2lxxnama5r7pzpysmccqys9dzs0",
		},
	}
)

// TestAezeedStandardDerivation verifies the standard MWEB key derivation
// path (m/0'/100') using 16-byte aezeed entropy. This is the path used by
// wallets created on or after day 6296 (2026-03-31 18:15:05 UTC).
func TestAezeedStandardDerivation(t *testing.T) {
	t.Parallel()

	const H = hdkeychain.HardenedKeyStart
	root, err := hdkeychain.NewMaster(aezeedEntropy, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("NewMaster: %v", err)
	}

	acctKey, _ := root.DeriveNonStandard(H)          // m/0'
	mwebKey, _ := acctKey.DeriveNonStandard(H + 100) // m/0'/100'
	scanExt, _ := mwebKey.DeriveNonStandard(H)       // m/0'/100'/0'
	spendExt, _ := mwebKey.DeriveNonStandard(H + 1)  // m/0'/100'/1'

	scanPriv, _ := scanExt.ECPrivKey()
	spendPriv, _ := spendExt.ECPrivKey()
	scanBytes := scanPriv.Key.Bytes()
	spendBytes := spendPriv.Key.Bytes()

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"scan secret", hex.EncodeToString(scanBytes[:]), aezeedStandardScanSecret},
		{"spend secret", hex.EncodeToString(spendBytes[:]), aezeedStandardSpendSecret},
		{"scan pubkey", hex.EncodeToString(scanPriv.PubKey().SerializeCompressed()), aezeedStandardScanPubKey},
		{"spend pubkey", hex.EncodeToString(spendPriv.PubKey().SerializeCompressed()), aezeedStandardSpendPubKey},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s mismatch:\n  got:  %s\n  want: %s", c.name, c.got, c.want)
		}
	}
}

// TestAezeedLegacyDerivation verifies the legacy MWEB key derivation
// path (m/1000'/2'/0') using the same 16-byte aezeed entropy. This is the
// path used by wallets created before day 6296.
func TestAezeedLegacyDerivation(t *testing.T) {
	t.Parallel()

	const H = hdkeychain.HardenedKeyStart
	root, err := hdkeychain.NewMaster(aezeedEntropy, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("NewMaster: %v", err)
	}

	purposeKey, _ := root.DeriveNonStandard(H + 1000)     // m/1000'
	coinTypeKey, _ := purposeKey.DeriveNonStandard(H + 2) // m/1000'/2'
	acctKey, _ := deriveAccountKey(coinTypeKey, 0)        // m/1000'/2'/0'
	scanExt, _ := acctKey.Derive(H)                       // m/1000'/2'/0'/0'
	spendExt, _ := acctKey.Derive(H + 1)                  // m/1000'/2'/0'/1'

	scanPriv, _ := scanExt.ECPrivKey()
	spendPriv, _ := spendExt.ECPrivKey()
	scanBytes := scanPriv.Key.Bytes()
	spendBytes := spendPriv.Key.Bytes()

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"scan secret", hex.EncodeToString(scanBytes[:]), aezeedLegacyScanSecret},
		{"spend secret", hex.EncodeToString(spendBytes[:]), aezeedLegacySpendSecret},
		{"scan pubkey", hex.EncodeToString(scanPriv.PubKey().SerializeCompressed()), aezeedLegacyScanPubKey},
		{"spend pubkey", hex.EncodeToString(spendPriv.PubKey().SerializeCompressed()), aezeedLegacySpendPubKey},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s mismatch:\n  got:  %s\n  want: %s", c.name, c.got, c.want)
		}
	}

	// Must differ from standard scope
	if aezeedLegacyScanSecret == aezeedStandardScanSecret {
		t.Error("legacy and standard scan secrets must differ")
	}
}

// TestAezeedStandardSubaddresses verifies stealth addresses at multiple
// indices using the standard scope keys derived from aezeed entropy.
func TestAezeedStandardSubaddresses(t *testing.T) {
	t.Parallel()

	scanBytes, _ := hex.DecodeString(aezeedStandardScanSecret)
	spendBytes, _ := hex.DecodeString(aezeedStandardSpendSecret)
	kc := &mweb.Keychain{
		Scan:  (*mw.SecretKey)(scanBytes),
		Spend: (*mw.SecretKey)(spendBytes),
	}

	for _, tc := range aezeedStandardAddresses {
		addr := kc.Address(tc.index)
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

		sk := kc.SpendKey(tc.index)
		gotSK := hex.EncodeToString(sk[:])
		if gotSK != tc.spendKey {
			t.Errorf("index %d: spend_key mismatch:\n  got:  %s\n  want: %s",
				tc.index, gotSK, tc.spendKey)
		}

		encoded := ltcutil.NewAddressMweb(addr, &chaincfg.MainNetParams)
		if got := encoded.EncodeAddress(); got != tc.encoded {
			t.Errorf("index %d: encoded address mismatch:\n  got:  %s\n  want: %s",
				tc.index, got, tc.encoded)
		}
	}
}

// TestAezeedLegacySubaddresses verifies stealth addresses at multiple
// indices using the legacy scope keys derived from aezeed entropy.
func TestAezeedLegacySubaddresses(t *testing.T) {
	t.Parallel()

	scanBytes, _ := hex.DecodeString(aezeedLegacyScanSecret)
	spendBytes, _ := hex.DecodeString(aezeedLegacySpendSecret)
	kc := &mweb.Keychain{
		Scan:  (*mw.SecretKey)(scanBytes),
		Spend: (*mw.SecretKey)(spendBytes),
	}

	for _, tc := range aezeedLegacyAddresses {
		addr := kc.Address(tc.index)
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

		sk := kc.SpendKey(tc.index)
		gotSK := hex.EncodeToString(sk[:])
		if gotSK != tc.spendKey {
			t.Errorf("index %d: spend_key mismatch:\n  got:  %s\n  want: %s",
				tc.index, gotSK, tc.spendKey)
		}

		encoded := ltcutil.NewAddressMweb(addr, &chaincfg.MainNetParams)
		if got := encoded.EncodeAddress(); got != tc.encoded {
			t.Errorf("index %d: encoded address mismatch:\n  got:  %s\n  want: %s",
				tc.index, got, tc.encoded)
		}
	}
}

// TestAezeedStandardVsLegacyDifference verifies that the same aezeed entropy
// produces completely different stealth addresses when derived via the standard
// scope vs the legacy scope.
func TestAezeedStandardVsLegacyDifference(t *testing.T) {
	t.Parallel()

	if len(aezeedStandardAddresses) != len(aezeedLegacyAddresses) {
		t.Fatal("test vector arrays have different lengths")
	}

	for i := range aezeedStandardAddresses {
		std := aezeedStandardAddresses[i]
		leg := aezeedLegacyAddresses[i]
		if std.index != leg.index {
			t.Fatalf("index mismatch at position %d", i)
		}

		if std.scanA == leg.scanA {
			t.Errorf("index %d: standard and legacy A_i should differ", std.index)
		}
		if std.spendB == leg.spendB {
			t.Errorf("index %d: standard and legacy B_i should differ", std.index)
		}
		if std.spendKey == leg.spendKey {
			t.Errorf("index %d: standard and legacy spend_key should differ", std.index)
		}
		if std.encoded == leg.encoded {
			t.Errorf("index %d: standard and legacy encoded addresses should differ", std.index)
		}
	}
}
