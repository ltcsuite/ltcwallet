package wallet

import (
	"testing"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/txscript"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
	"github.com/ltcsuite/ltcwallet/walletdb"
	"github.com/ltcsuite/ltcwallet/wtxmgr"
	"github.com/stretchr/testify/require"
)

// accountFilterTestWallet creates an unlocked test wallet with two BIP84
// accounts ("default" at account 0 and "savings" at account 1), returning
// p2wpkh pkScripts for each. The caller must defer cleanup().
func accountFilterTestWallet(t *testing.T) (
	w *Wallet, defaultPkScript, savingsPkScript []byte, cleanup func(),
) {
	t.Helper()
	w, cleanup = testWallet(t)

	// Unlock so we can derive addresses and create accounts.
	err := walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		return w.Manager.Unlock(ns, []byte("world"))
	})
	require.NoError(t, err)

	err = walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		ns := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		smgr, err := w.Manager.FetchScopedKeyManager(
			waddrmgr.KeyScopeBIP0084,
		)
		if err != nil {
			return err
		}

		// Default account (0) address.
		addrs, err := smgr.NextExternalAddresses(ns, 0, 1)
		if err != nil {
			return err
		}
		defaultPkScript, err = txscript.PayToAddrScript(
			addrs[0].Address(),
		)
		if err != nil {
			return err
		}

		// Create "savings" account (will be account 1).
		_, err = smgr.NewAccount(ns, "savings")
		if err != nil {
			return err
		}

		addrs, err = smgr.NextExternalAddresses(ns, 1, 1)
		if err != nil {
			return err
		}
		savingsPkScript, err = txscript.PayToAddrScript(
			addrs[0].Address(),
		)
		return err
	})
	require.NoError(t, err)
	return
}

// countMinedTxs counts the total number of mined transactions.
func countMinedTxs(res *GetTransactionsResult) int {
	n := 0
	for _, b := range res.MinedTransactions {
		n += len(b.Transactions)
	}
	return n
}

// TestGetTransactionsAccountFilter verifies that GetTransactions with an
// account name filter returns account-scoped transaction summaries for
// a simple credit-only transaction.
func TestGetTransactionsAccountFilter(t *testing.T) {
	t.Parallel()
	w, defaultPkScript, _, cleanup := accountFilterTestWallet(t)
	defer cleanup()

	// Build a transaction with one output paying to the default address.
	msgTx := wire.NewMsgTx(2)
	msgTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0x01},
			Index: 0,
		},
	})
	msgTx.AddTxOut(&wire.TxOut{
		Value:    1_000_000,
		PkScript: defaultPkScript,
	})

	rec, err := wtxmgr.NewTxRecordFromMsgTx(msgTx, time.Now())
	require.NoError(t, err)

	block := &wtxmgr.BlockMeta{
		Block: wtxmgr.Block{
			Hash:   chainhash.Hash{0x02},
			Height: 100,
		},
		Time: time.Now(),
	}

	err = walletdb.Update(w.db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wtxmgrNamespaceKey)
		if err := w.TxStore.InsertTx(ns, rec, block); err != nil {
			return err
		}
		return w.TxStore.AddCredit(ns, rec, block, 0, false)
	})
	require.NoError(t, err)

	// Unfiltered — should include the transaction.
	res, err := w.GetTransactions(
		NewBlockIdentifierFromHeight(0),
		NewBlockIdentifierFromHeight(-1),
		"", nil,
	)
	require.NoError(t, err)
	require.Equal(t, 1, countMinedTxs(res), "unfiltered should return 1 tx")

	tx0 := res.MinedTransactions[0].Transactions[0]
	require.Len(t, tx0.MyOutputs, 1)
	require.Equal(t, uint32(0), tx0.MyOutputs[0].Index)

	// Filter by "default" — should include with scoped summary.
	res, err = w.GetTransactions(
		NewBlockIdentifierFromHeight(0),
		NewBlockIdentifierFromHeight(-1),
		"default", nil,
	)
	require.NoError(t, err)
	require.Equal(t, 1, countMinedTxs(res), "default filter should return 1 tx")

	tx0 = res.MinedTransactions[0].Transactions[0]
	require.Len(t, tx0.MyOutputs, 1, "credit should be in MyOutputs")
	require.Empty(t, tx0.MyInputs, "no wallet debits expected")

	// Filter by non-existent account — should exclude.
	res, err = w.GetTransactions(
		NewBlockIdentifierFromHeight(0),
		NewBlockIdentifierFromHeight(-1),
		"hw-mweb", nil,
	)
	require.NoError(t, err)
	require.Equal(t, 0, countMinedTxs(res))
	require.Empty(t, res.UnminedTransactions)
}

// TestGetTransactionsMixedAccountTransfer verifies account-scoped summaries
// for an internal transfer: default sends to savings. The same tx should
// appear in both filtered views with correct debits, credits, and fees.
func TestGetTransactionsMixedAccountTransfer(t *testing.T) {
	t.Parallel()
	w, defaultPkScript, savingsPkScript, cleanup := accountFilterTestWallet(t)
	defer cleanup()

	// --- tx1: external deposit into default account ---
	const depositAmount = 1_000_000
	tx1 := wire.NewMsgTx(2)
	tx1.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{0xaa},
			Index: 0,
		},
	})
	tx1.AddTxOut(&wire.TxOut{
		Value:    depositAmount,
		PkScript: defaultPkScript,
	})

	rec1, err := wtxmgr.NewTxRecordFromMsgTx(tx1, time.Now())
	require.NoError(t, err)

	block1 := &wtxmgr.BlockMeta{
		Block: wtxmgr.Block{
			Hash:   chainhash.Hash{0xb1},
			Height: 100,
		},
		Time: time.Now(),
	}

	err = walletdb.Update(w.db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wtxmgrNamespaceKey)
		if err := w.TxStore.InsertTx(ns, rec1, block1); err != nil {
			return err
		}
		return w.TxStore.AddCredit(ns, rec1, block1, 0, false)
	})
	require.NoError(t, err)

	// --- tx2: default → savings (internal transfer) ---
	// Spends tx1:0 (1,000,000), sends 900,000 to savings.
	// Fee = 1,000,000 - 900,000 = 100,000.
	const transferAmount = 900_000
	const expectedFee = depositAmount - transferAmount

	tx2 := wire.NewMsgTx(2)
	tx2.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  rec1.Hash,
			Index: 0,
		},
	})
	tx2.AddTxOut(&wire.TxOut{
		Value:    transferAmount,
		PkScript: savingsPkScript,
	})

	rec2, err := wtxmgr.NewTxRecordFromMsgTx(tx2, time.Now())
	require.NoError(t, err)

	block2 := &wtxmgr.BlockMeta{
		Block: wtxmgr.Block{
			Hash:   chainhash.Hash{0xb2},
			Height: 101,
		},
		Time: time.Now(),
	}

	err = walletdb.Update(w.db, func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wtxmgrNamespaceKey)
		if err := w.TxStore.InsertTx(ns, rec2, block2); err != nil {
			return err
		}
		return w.TxStore.AddCredit(ns, rec2, block2, 0, false)
	})
	require.NoError(t, err)

	// Sanity: unfiltered should have 2 mined txs.
	res, err := w.GetTransactions(
		NewBlockIdentifierFromHeight(0),
		NewBlockIdentifierFromHeight(-1),
		"", nil,
	)
	require.NoError(t, err)
	require.Equal(t, 2, countMinedTxs(res), "unfiltered should have 2 txs")

	// --- Filter by "default" ---
	// tx1: credit to default (receive).
	// tx2: debit from default (send), no credit to default.
	res, err = w.GetTransactions(
		NewBlockIdentifierFromHeight(0),
		NewBlockIdentifierFromHeight(-1),
		"default", nil,
	)
	require.NoError(t, err)
	require.Equal(t, 2, countMinedTxs(res),
		"default filter should include both txs")

	// Find tx2 in the results (block height 101).
	var tx2Summary *TransactionSummary
	for _, b := range res.MinedTransactions {
		for i, s := range b.Transactions {
			if *s.Hash == rec2.Hash {
				tx2Summary = &b.Transactions[i]
			}
		}
	}
	require.NotNil(t, tx2Summary, "tx2 should be in default results")

	// tx2 from default's perspective: debit (spent tx1:0), no credit.
	require.Len(t, tx2Summary.MyInputs, 1,
		"default should see 1 debit (spent tx1:0)")
	require.Equal(t, ltcutil.Amount(depositAmount),
		tx2Summary.MyInputs[0].PreviousAmount)
	require.Empty(t, tx2Summary.MyOutputs,
		"default should see no credits (output goes to savings)")

	// Fee should be attributed since all debits are from default.
	require.Equal(t, ltcutil.Amount(expectedFee), tx2Summary.Fee,
		"fee should be attributed when all debits from filtered account")

	// --- Filter by "savings" ---
	// Only tx2 should appear (credit to savings).
	// tx1 should NOT appear (it only touches default).
	res, err = w.GetTransactions(
		NewBlockIdentifierFromHeight(0),
		NewBlockIdentifierFromHeight(-1),
		"savings", nil,
	)
	require.NoError(t, err)
	require.Equal(t, 1, countMinedTxs(res),
		"savings filter should include only tx2")

	tx2Summary = nil
	for _, b := range res.MinedTransactions {
		for i, s := range b.Transactions {
			if *s.Hash == rec2.Hash {
				tx2Summary = &b.Transactions[i]
			}
		}
	}
	require.NotNil(t, tx2Summary, "tx2 should be in savings results")

	// tx2 from savings' perspective: credit (received), no debit.
	require.Len(t, tx2Summary.MyOutputs, 1,
		"savings should see 1 credit")
	require.Empty(t, tx2Summary.MyInputs,
		"savings should see no debits (input is from default)")

	// Fee should NOT be attributed (debits are from default, not savings).
	require.Equal(t, ltcutil.Amount(0), tx2Summary.Fee,
		"fee should not be attributed when debits are from another account")
}
