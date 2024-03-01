package wallet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/big"
	"slices"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/ltcutil/mweb"
	"github.com/ltcsuite/ltcd/ltcutil/mweb/mw"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/chain"
	"github.com/ltcsuite/ltcwallet/internal/zero"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
	"github.com/ltcsuite/ltcwallet/walletdb"
	"github.com/ltcsuite/ltcwallet/wtxmgr"
	"lukechampine.com/blake3"
)

type (
	skmAccount struct {
		skm     *waddrmgr.ScopedKeyManager
		account uint32
	}
	mwebAccount struct {
		skmAccount
		scanSecret *mw.SecretKey
	}
)

func (w *Wallet) forEachMwebAccount(addrmgrNs walletdb.ReadBucket,
	fn func(*mwebAccount) error) error {

	for _, scope := range w.Manager.ScopesForExternalAddrType(waddrmgr.Mweb) {
		s, err := w.Manager.FetchScopedKeyManager(scope)
		if err != nil {
			return err
		}
		err = s.ForEachAccount(addrmgrNs, func(account uint32) error {
			props, err := s.AccountProperties(addrmgrNs, account)
			if err != nil || props.AccountScanKey == nil {
				return err
			}
			scanKeyPriv, err := props.AccountScanKey.ECPrivKey()
			if err != nil {
				return err
			}
			defer scanKeyPriv.Zero()
			scanSecret := (*mw.SecretKey)(scanKeyPriv.Serialize())
			defer zero.Bytes(scanSecret[:])
			return fn(&mwebAccount{skmAccount{s, account}, scanSecret})
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *Wallet) extractCanonicalFromMweb(
	dbtx walletdb.ReadWriteTx, rec *wtxmgr.TxRecord) error {

	addrmgrNs := dbtx.ReadBucket(waddrmgrNamespaceKey)
	txmgrNs := dbtx.ReadWriteBucket(wtxmgrNamespaceKey)

	if rec.MsgTx.Mweb == nil {
		return nil
	}

	kernels := rec.MsgTx.Mweb.TxBody.Kernels
	if len(kernels) > 0 {
		// Check for sentinel kernel
		if kernels[len(kernels)-1].Excess[0] == 0 {
			return nil
		}
	}

	for _, input := range rec.MsgTx.Mweb.TxBody.Inputs {
		outpoint, _, err := w.TxStore.GetMwebOutpoint(txmgrNs, &input.OutputId)
		if err != nil {
			continue
		}
		rec.MsgTx.AddTxIn(&wire.TxIn{PreviousOutPoint: *outpoint})
	}

	err := w.forEachMwebAccount(addrmgrNs, func(ma *mwebAccount) error {
		for _, output := range rec.MsgTx.Mweb.TxBody.Outputs {
			coin, err := mweb.RewindOutput(output, ma.scanSecret)
			if err != nil {
				continue
			}
			addr := ltcutil.NewAddressMweb(coin.Address, w.chainParams)
			rec.MsgTx.AddTxOut(wire.NewTxOut(
				int64(coin.Value), addr.ScriptAddress()))
			w.TxStore.AddMwebOutpoint(txmgrNs, coin.OutputId,
				wire.NewOutPoint(&rec.Hash, uint32(len(rec.MsgTx.TxOut)-1)))
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, kernel := range kernels {
		for i, pegout := range kernel.Pegouts {
			h := blake3.New(32, nil)
			h.Write(kernel.Hash()[:])
			binary.Write(h, binary.LittleEndian, uint32(i))
			rec.MsgTx.AddTxOut(wire.NewTxOut(pegout.Value, pegout.PkScript))
			w.TxStore.AddMwebOutpoint(txmgrNs, (*chainhash.Hash)(h.Sum(nil)),
				wire.NewOutPoint(&rec.Hash, uint32(len(rec.MsgTx.TxOut)-1)))
		}
	}

	// Add a sentinel kernel so that we don't process this tx again.
	rec.MsgTx.Mweb.TxBody.Kernels = append(kernels, &wire.MwebKernel{})

	rec.SerializedTx = nil

	return nil
}

func (w *Wallet) getMwebPegouts(txmgrNs walletdb.ReadBucket,
	rec *wtxmgr.TxRecord) (map[uint32]bool, error) {

	pegouts := make(map[uint32]bool)
	if rec.MsgTx.Mweb == nil {
		return pegouts, nil
	}

	for _, kernel := range rec.MsgTx.Mweb.TxBody.Kernels {
		for i := range kernel.Pegouts {
			h := blake3.New(32, nil)
			h.Write(kernel.Hash()[:])
			binary.Write(h, binary.LittleEndian, uint32(i))
			op, _, err := w.TxStore.GetMwebOutpoint(
				txmgrNs, (*chainhash.Hash)(h.Sum(nil)))
			if err != nil {
				return nil, err
			}
			if op.Hash != rec.Hash {
				return nil, errors.New("unexpected outpoint for pegout")
			}
			pegouts[op.Index] = true
		}
	}

	return pegouts, nil
}

func (w *Wallet) getBlockMeta(height int32) (*wtxmgr.BlockMeta, error) {
	chainClient, err := w.requireChainClient()
	if err != nil {
		return nil, err
	}
	blockHash, err := chainClient.GetBlockHash(int64(height))
	if err != nil {
		return nil, err
	}
	blockHeader, err := chainClient.GetBlockHeader(blockHash)
	if err != nil {
		return nil, err
	}
	return &wtxmgr.BlockMeta{
		Block: wtxmgr.Block{Hash: *blockHash, Height: height},
		Time:  blockHeader.Timestamp,
	}, nil
}

func (w *Wallet) checkMwebUtxos(
	dbtx walletdb.ReadWriteTx, n *chain.MwebUtxos) error {

	addrmgrNs := dbtx.ReadWriteBucket(waddrmgrNamespaceKey)
	txmgrNs := dbtx.ReadBucket(wtxmgrNamespaceKey)

	type minedTx struct {
		rec    *wtxmgr.TxRecord
		height int32
	}
	minedTxns := make(map[chainhash.Hash]minedTx)
	var remainingUtxos []*wire.MwebNetUtxo

	for _, utxo := range n.Utxos {
		_, rec, err := w.TxStore.GetMwebOutpoint(txmgrNs, utxo.OutputId)
		switch err {
		case nil:
			minedTxns[rec.Hash] = minedTx{rec, utxo.Height}
		case wtxmgr.ErrUnknownOutput:
			remainingUtxos = append(remainingUtxos, utxo)
		default:
			return err
		}
	}

	for _, tx := range minedTxns {
		block, err := w.getBlockMeta(tx.height)
		if err != nil {
			return err
		}
		err = w.addRelevantTx(dbtx, tx.rec, block)
		if err != nil {
			return err
		}
		if block.Height <= w.Manager.SyncedTo().Height {
			w.NtfnServer.notifyAttachedBlock(dbtx, block)
		}
	}

	err := w.forEachMwebAccount(addrmgrNs, func(ma *mwebAccount) error {
		for _, utxo := range remainingUtxos {
			coin, err := mweb.RewindOutput(utxo.Output, ma.scanSecret)
			if err != nil {
				continue
			}
			addr := ltcutil.NewAddressMweb(coin.Address, w.chainParams)
			ok, err := w.mwebKeyPools[ma.skmAccount].contains(addrmgrNs, addr)
			if err != nil {
				return err
			} else if !ok {
				continue
			}
			block, err := w.getBlockMeta(utxo.Height)
			if err != nil {
				return err
			}
			rec := &wtxmgr.TxRecord{
				MsgTx: wire.MsgTx{
					Mweb: &wire.MwebTx{TxBody: &wire.MwebTxBody{
						Outputs: []*wire.MwebOutput{utxo.Output},
					}},
				},
				Hash:     *utxo.OutputId,
				Received: block.Time,
			}
			if utxo.Height == 0 {
				rec.Received = time.Now()
				block = nil
			}
			err = w.addRelevantTx(dbtx, rec, block)
			if err != nil {
				return err
			}
			if block != nil && block.Height <= w.Manager.SyncedTo().Height {
				w.NtfnServer.notifyAttachedBlock(dbtx, block)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return w.checkMwebLeafset(dbtx, n.Leafset)
}

func (w *Wallet) checkMwebLeafset(dbtx walletdb.ReadWriteTx,
	newLeafset *mweb.Leafset) error {

	addrmgrNs := dbtx.ReadWriteBucket(waddrmgrNamespaceKey)
	txmgrNs := dbtx.ReadBucket(wtxmgrNamespaceKey)

	if newLeafset == nil || newLeafset.Block == nil {
		return nil
	}

	oldLeafset, err := w.getMwebLeafset(addrmgrNs)
	if err != nil {
		return err
	}

	err = w.putMwebLeafset(addrmgrNs, newLeafset)
	if err != nil {
		return err
	}

	oldBits := oldLeafset.Bits
	newBits := newLeafset.Bits
	if len(oldBits) < len(newBits) {
		newBits = newBits[:len(oldBits)]
	} else {
		oldBits = oldBits[:len(newBits)]
	}

	old := new(big.Int).SetBytes(oldBits)
	new := new(big.Int).SetBytes(newBits)

	if new.And(old, new).Cmp(old) == 0 {
		return nil
	}

	// Leaves in the old leafset were spent, check if they were ours.
	chainClient, err := w.requireChainClient()
	if err != nil {
		return err
	}

	nc, ok := chainClient.(*chain.NeutrinoClient)
	if !ok {
		return nil
	}

	outputs, err := w.TxStore.UnspentOutputs(txmgrNs)
	if err != nil {
		return err
	}

	var rec wtxmgr.TxRecord
	for _, output := range outputs {
		if output.MwebOutput == nil || output.Height < 0 {
			continue
		}
		if !nc.CS.MwebUtxoExists(output.MwebOutput.Hash()) {
			rec.MsgTx.AddTxIn(&wire.TxIn{PreviousOutPoint: output.OutPoint})
		}
	}

	if len(rec.MsgTx.TxIn) == 0 {
		return nil
	}

	block := &wtxmgr.BlockMeta{
		Block: wtxmgr.Block{
			Hash:   newLeafset.Block.BlockHash(),
			Height: int32(newLeafset.Height),
		},
		Time: newLeafset.Block.Timestamp,
	}

	rec.Hash = rec.MsgTx.TxHash()
	rec.Received = block.Time

	err = w.addRelevantTx(dbtx, &rec, block)
	if err != nil {
		return err
	}

	if block.Height <= w.Manager.SyncedTo().Height {
		w.NtfnServer.notifyAttachedBlock(dbtx, block)
	}

	return nil
}

var mwebLeafsets = []byte("mwebLeafsets")

func (w *Wallet) getMwebLeafset(
	addrmgrNs walletdb.ReadBucket) (*mweb.Leafset, error) {

	leafset := &mweb.Leafset{}
	mwebLeafsets := addrmgrNs.NestedReadBucket(mwebLeafsets)
	if mwebLeafsets == nil {
		return leafset, nil
	}

	err := mwebLeafsets.ForEach(func(k, v []byte) error {
		lfs := &mweb.Leafset{}
		err := lfs.Deserialize(bytes.NewReader(v))
		if err != nil {
			return err
		}

		hash, err := w.Manager.BlockHash(addrmgrNs, int32(lfs.Height))

		switch {
		case lfs.Height < leafset.Height:
		case lfs.Height > uint32(w.Manager.SyncedTo().Height):
		case waddrmgr.IsError(err, waddrmgr.ErrBlockNotFound):
		case err != nil:
			return err
		case lfs.Block.BlockHash() == *hash:
			leafset = lfs
		}

		return nil
	})

	return leafset, err
}

func (w *Wallet) putMwebLeafset(addrmgrNs walletdb.ReadWriteBucket,
	leafset *mweb.Leafset) error {

	mwebLeafsets, err := addrmgrNs.CreateBucketIfNotExists(mwebLeafsets)
	if err != nil {
		return err
	}

	// Delete older and newer leafsets.
	err = mwebLeafsets.ForEach(func(k, v []byte) error {
		height := binary.LittleEndian.Uint32(k)
		if height < leafset.Height-10 || height > leafset.Height {
			return mwebLeafsets.Delete(k)
		}
		return nil
	})
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err = leafset.Serialize(&buf); err != nil {
		return err
	}
	k := binary.LittleEndian.AppendUint32(nil, leafset.Height)
	return mwebLeafsets.Put(k, buf.Bytes())
}

type mwebKeyPool struct {
	mwebAccount
	index    uint32
	keychain *mweb.Keychain
	addrs    []*mw.StealthAddress
}

func newMwebKeyPool(addrmgrNs walletdb.ReadBucket,
	ma *mwebAccount) (*mwebKeyPool, error) {

	props, err := ma.skm.AccountProperties(addrmgrNs, ma.account)
	if err != nil {
		return nil, err
	}

	spendPubKey, err := props.AccountSpendPubKey.ECPubKey()
	if err != nil {
		return nil, err
	}

	kp := &mwebKeyPool{
		mwebAccount: *ma,
		index:       props.ExternalKeyCount,
		keychain: &mweb.Keychain{
			Scan:        (*mw.SecretKey)(bytes.Clone(ma.scanSecret[:])),
			SpendPubKey: (*mw.PublicKey)(spendPubKey.SerializeCompressed()),
		},
	}
	kp.topUp()

	return kp, nil
}

func (kp *mwebKeyPool) topUp() {
	for len(kp.addrs) < 1000 {
		index := kp.index + uint32(len(kp.addrs))
		kp.addrs = append(kp.addrs, kp.keychain.Address(index))
	}
}

func (kp *mwebKeyPool) contains(addrmgrNs walletdb.ReadWriteBucket,
	addr *ltcutil.AddressMweb) (bool, error) {

	switch _, err := kp.skm.Address(addrmgrNs, addr); {
	case err == nil:
		return true, nil
	case !waddrmgr.IsError(err, waddrmgr.ErrAddressNotFound):
		return false, err
	}

	index := slices.IndexFunc(kp.addrs, addr.StealthAddress().Equal)
	if index < 0 {
		return false, nil
	}

	err := kp.skm.ExtendExternalAddresses(addrmgrNs,
		kp.account, kp.index+uint32(index))
	if err != nil {
		return false, err
	}

	kp.index += uint32(index + 1)
	kp.addrs = kp.addrs[index+1:]
	kp.topUp()

	return true, nil
}
