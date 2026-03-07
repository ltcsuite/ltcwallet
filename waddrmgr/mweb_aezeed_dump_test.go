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

// Test vectors extracted from a Litecoin Core v0.21.4 wallet dump (dumpwallet RPC).
// The wallet was freshly created on mainnet and the dump contains the extended
// private master key and all derived MWEB stealth addresses with their subaddress
// indices (hdkeypath=x/N).
//
// This test verifies that ltcsuite's standard MWEB derivation (m/0'/100')
// produces identical stealth addresses to Litecoin Core for the same master key.
const coreDumpMasterKey = "xprv9s21ZrQH143K3hn24DjqcqBXYUipDgAUowud2eZ4Y8sHR5CwQL71oo6igoVE5J2mwMjyVuEFKSkL9y8isyUwuVVudcbgSgyzSsQHdL1wLV9"

var coreDumpAddresses = []struct {
	index   uint32
	encoded string // ltcmweb1... bech32 address from Litecoin Core
}{
	{0, "ltcmweb1qqfg3chwxy50a3mtm798627uqvj5j75ljqkuh9tn9y23j9verqq6rxq47gyru7wzrha8zg54yc80lumg79lvamddyxg7u6m2lyyw2lvrt6uvqtdtq"},
	{1, "ltcmweb1qqgddjct8ve9wr0d4mh6038gjzgqttrzwuykhcjp055yrhhmsxk2tgq3d2epdm6t90v2zgm8l65efkqnhmjmtcggampw9mezly99s0ljsqsaz7qhu"},
	{2, "ltcmweb1qq020drnwa2rv4hyn2lkcw9p8m8nxme8vd993uq9d5vuymrknvrftxqe7z6yfzcu5hwssqh08mylmrkv42n47a5as0ee2m2vac9e2rh2kjqpxtjkk"},
	{3, "ltcmweb1qqdhnrg83waj9shuu4kqua56jh4e7qvsxndsuljm8yvw6f6puy637kq5zxf5p508z6q6nhzr4xlz9s2cqu70pve9cudp0r92ftfu0m7k98urkrv8m"},
	{4, "ltcmweb1qqdh8utmlx59wr0f6luhkz5rnyp0mw3yluspzx0nhv9qt026v3xtjcqcr2pn27ggtpt4z90yng4tl3tpded0z5t25qfksd9tmw3trxuvnescqtgeq"},
	{10, "ltcmweb1qqty708vy6vf7u6d5h0zegls8vful9ammg93asutzkjgt469rgrsvyqmv8z970vxwcpkuwqxhzfnjazy56dcpqs44yfe5kujpwasy247s2q75949w"},
	{50, "ltcmweb1qqgnhcx46enrnmsya2qpqwv6uaak4m489487mcfaslpg548h44533jq3ngeqzgpt5wxhmyh5ygl23y22rt3334v5cm56r2tc3v5jvcfq96cp6lex0"},
	{100, "ltcmweb1qqfeytpjrfjr5wnd6jv6r9luhhsz980vz6gawfwfe05k3gf9j90dgyqm9km772ydau8d877amwqltj6nfgttfgdashh6pmj8ynspduhzanvu8fazv"},
	{200, "ltcmweb1qqddmt2awpcknrpu2fp9ew0l43k9aqps75qm7dcx6h22nzy02c0m7sqezn8kew2eq7kjvfng5qdmj9v59hadzzrh0uf6dlfm9f95t7h4q2c8s79kx"},
	{500, "ltcmweb1qqddz5jawhwrahf6yn728g9gmtte3n3z6teflj4wxhez5rnazkxnckqjw8q7txnng9yhtnxdaufvawzqa7303qjjddm9z2uxsug5dp6cegye9rf5p"},
}

// TestCoreDumpMwebAddresses parses the extended master key from a Litecoin Core
// wallet dump and verifies that ltcsuite derives identical MWEB stealth addresses
// at every subaddress index present in the dump.
func TestCoreDumpMwebAddresses(t *testing.T) {
	t.Parallel()

	root, err := hdkeychain.NewKeyFromString(coreDumpMasterKey)
	if err != nil {
		t.Fatalf("parse master key: %v", err)
	}

	const H = hdkeychain.HardenedKeyStart

	// Standard MWEB derivation: m/0'/100'
	acctKey, err := root.DeriveNonStandard(H) // m/0'
	if err != nil {
		t.Fatalf("derive m/0': %v", err)
	}
	mwebKey, err := acctKey.DeriveNonStandard(H + 100) // m/0'/100'
	if err != nil {
		t.Fatalf("derive m/0'/100': %v", err)
	}

	scanExt, err := mwebKey.DeriveNonStandard(H) // m/0'/100'/0'
	if err != nil {
		t.Fatalf("derive scan key: %v", err)
	}
	spendExt, err := mwebKey.DeriveNonStandard(H + 1) // m/0'/100'/1'
	if err != nil {
		t.Fatalf("derive spend key: %v", err)
	}

	scanPriv, err := scanExt.ECPrivKey()
	if err != nil {
		t.Fatalf("scan ECPrivKey: %v", err)
	}
	spendPriv, err := spendExt.ECPrivKey()
	if err != nil {
		t.Fatalf("spend ECPrivKey: %v", err)
	}

	scanBytes := scanPriv.Key.Bytes()
	spendBytes := spendPriv.Key.Bytes()

	t.Logf("scan secret:  %s", hex.EncodeToString(scanBytes[:]))
	t.Logf("spend secret: %s", hex.EncodeToString(spendBytes[:]))

	kc := &mweb.Keychain{
		Scan:  (*mw.SecretKey)(&scanBytes),
		Spend: (*mw.SecretKey)(&spendBytes),
	}

	for _, tc := range coreDumpAddresses {
		addr := kc.Address(tc.index)
		encoded := ltcutil.NewAddressMweb(addr, &chaincfg.MainNetParams)
		got := encoded.EncodeAddress()

		if got != tc.encoded {
			t.Errorf("index %d: address mismatch\n  got:  %s\n  want: %s",
				tc.index, got, tc.encoded)
		}
	}
}
