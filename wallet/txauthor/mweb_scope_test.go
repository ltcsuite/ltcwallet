package txauthor

import (
	"encoding/hex"
	"testing"

	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
)

// validMwebScript returns a 66-byte script containing two valid compressed
// secp256k1 public keys, which txscript.IsMweb recognizes as MWEB.
func validMwebScript() []byte {
	// secp256k1 generator point G (compressed)
	g, _ := hex.DecodeString(
		"0279BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798")
	script := make([]byte, 66)
	copy(script[:33], g)
	copy(script[33:], g)
	return script
}

// TestMwebScopePassedToChangeSource verifies that when MWEB inputs are
// present, NewUnsignedTransaction passes ChangeSource.MwebScope (not the
// hardcoded legacy scope) to NewScript.
func TestMwebScopePassedToChangeSource(t *testing.T) {
	t.Parallel()

	mwebScript := validMwebScript()

	inputSource := func(target ltcutil.Amount) (
		ltcutil.Amount, []*wire.TxIn, []ltcutil.Amount,
		[][]byte, []*wire.MwebOutput, error,
	) {
		return 1e8,
			[]*wire.TxIn{wire.NewTxIn(&wire.OutPoint{}, nil, nil)},
			[]ltcutil.Amount{1e8},
			[][]byte{mwebScript}, // MWEB script triggers isMweb
			nil, nil
	}

	var receivedScope *waddrmgr.KeyScope
	called := false
	standardScope := waddrmgr.KeyScopeMweb

	changeSource := &ChangeSource{
		ScriptSize: 66,
		NewScript: func(scope *waddrmgr.KeyScope) ([]byte, error) {
			called = true
			receivedScope = scope
			return mwebScript, nil
		},
		MwebScope: &standardScope,
	}

	outputs := p2pkhOutputs(1e6)
	_, err := NewUnsignedTransaction(outputs, 1000, inputSource, changeSource)
	if err != nil {
		t.Fatalf("NewUnsignedTransaction: %v", err)
	}

	if !called {
		t.Fatal("NewScript was never called")
	}
	if receivedScope == nil {
		t.Fatal("NewScript was called with nil scope")
	}
	if *receivedScope != waddrmgr.KeyScopeMweb {
		t.Errorf("expected scope %v, got %v",
			waddrmgr.KeyScopeMweb, *receivedScope)
	}
}

// TestMwebScopeNilFallsBackToLegacy verifies that when MwebScope is nil,
// NewUnsignedTransaction falls back to KeyScopeMwebLegacy.
func TestMwebScopeNilFallsBackToLegacy(t *testing.T) {
	t.Parallel()

	mwebScript := validMwebScript()

	inputSource := func(target ltcutil.Amount) (
		ltcutil.Amount, []*wire.TxIn, []ltcutil.Amount,
		[][]byte, []*wire.MwebOutput, error,
	) {
		return 1e8,
			[]*wire.TxIn{wire.NewTxIn(&wire.OutPoint{}, nil, nil)},
			[]ltcutil.Amount{1e8},
			[][]byte{mwebScript},
			nil, nil
	}

	var receivedScope *waddrmgr.KeyScope
	called := false

	changeSource := &ChangeSource{
		ScriptSize: 66,
		NewScript: func(scope *waddrmgr.KeyScope) ([]byte, error) {
			called = true
			receivedScope = scope
			return mwebScript, nil
		},
		MwebScope: nil, // nil → should fall back to legacy
	}

	outputs := p2pkhOutputs(1e6)
	_, err := NewUnsignedTransaction(outputs, 1000, inputSource, changeSource)
	if err != nil {
		t.Fatalf("NewUnsignedTransaction: %v", err)
	}

	if !called {
		t.Fatal("NewScript was never called")
	}
	if receivedScope == nil {
		t.Fatal("NewScript was called with nil scope")
	}
	if *receivedScope != waddrmgr.KeyScopeMwebLegacy {
		t.Errorf("expected fallback to %v, got %v",
			waddrmgr.KeyScopeMwebLegacy, *receivedScope)
	}
}
