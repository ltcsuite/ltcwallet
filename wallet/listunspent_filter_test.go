package wallet

import (
	"sort"
	"testing"

	"github.com/ltcsuite/ltcd/txscript"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
	"github.com/ltcsuite/ltcwallet/walletdb"
	"github.com/stretchr/testify/require"
)

// TestListUnspentAccountFilterAcrossOverlappingScopes verifies that account
// filtering considers every scoped manager that owns an address, not just the
// first one returned by AddrAccount.
func TestListUnspentAccountFilterAcrossOverlappingScopes(t *testing.T) {
	t.Parallel()

	w, cleanup := testWallet(t)
	defer cleanup()

	defaultProps, err := w.AccountProperties(
		waddrmgr.KeyScopeBIP0084, waddrmgr.DefaultAccountNum,
	)
	require.NoError(t, err)
	require.NotNil(t, defaultProps.AccountPubKey)

	shadowScope := waddrmgr.KeyScope{
		Purpose: 1017,
		Coin:    waddrmgr.KeyScopeBIP0084.Coin,
	}
	shadowSchema := waddrmgr.ScopeAddrSchema{
		ExternalAddrType: waddrmgr.WitnessPubKey,
		InternalAddrType: waddrmgr.WitnessPubKey,
	}
	shadowProps, err := w.ImportAccountWithScope(
		"shadow", defaultProps.AccountPubKey,
		defaultProps.MasterKeyFingerprint, shadowScope, shadowSchema,
	)
	require.NoError(t, err)

	defaultAddr, err := w.CurrentAddress(
		waddrmgr.DefaultAccountNum, waddrmgr.KeyScopeBIP0084,
	)
	require.NoError(t, err)

	shadowAddr, err := w.NewAddress(shadowProps.AccountNumber, shadowScope)
	require.NoError(t, err)

	require.Equal(t, defaultAddr.String(), shadowAddr.String())

	var accountLookups []outputAccountLookup
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		addrmgrNs := tx.ReadBucket(waddrmgrNamespaceKey)

		var err error
		accountLookups, err = w.lookupAddressAccounts(
			addrmgrNs, defaultAddr,
		)
		return err
	})
	require.NoError(t, err)
	require.Len(t, accountLookups, 2)

	accountNames := make([]string, 0, len(accountLookups))
	for _, lookup := range accountLookups {
		accountNames = append(accountNames, lookup.name)
	}
	sort.Strings(accountNames)
	require.Equal(t, []string{"default", "shadow"}, accountNames)

	pkScript, err := txscript.PayToAddrScript(defaultAddr)
	require.NoError(t, err)

	incomingTx := &wire.MsgTx{
		TxIn: []*wire.TxIn{{}},
		TxOut: []*wire.TxOut{{
			Value:    100_000,
			PkScript: pkScript,
		}},
	}
	addUtxo(t, w, incomingTx)

	defaultUnspent, err := w.ListUnspent(0, testBlockHeight, "default")
	require.NoError(t, err)
	require.Len(t, defaultUnspent, 1)

	shadowUnspent, err := w.ListUnspent(0, testBlockHeight, "shadow")
	require.NoError(t, err)
	require.Len(t, shadowUnspent, 1)
}
