// Copyright (c) 2020 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/ltcutil/hdkeychain"
	"github.com/ltcsuite/ltcd/ltcutil/mweb"
	"github.com/ltcsuite/ltcd/ltcutil/mweb/mw"
	"github.com/ltcsuite/ltcd/ltcutil/psbt"
	"github.com/ltcsuite/ltcd/txscript"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/internal/zero"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
	"github.com/ltcsuite/ltcwallet/wallet/txauthor"
	"github.com/ltcsuite/ltcwallet/wallet/txrules"
	"github.com/ltcsuite/ltcwallet/walletdb"
	"github.com/ltcsuite/ltcwallet/wtxmgr"
)

// FundPsbt creates a fully populated PSBT packet that contains enough inputs to
// fund the outputs specified in the passed in packet with the specified fee
// rate. If there is change left, a change output from the wallet is added and
// the index of the change output is returned. If no custom change scope is
// specified, we will use the coin selection scope (if not nil) or the BIP0086
// scope by default. Otherwise, no additional output is created and the
// index -1 is returned.
//
// NOTE: If the packet doesn't contain any inputs, coin selection is performed
// automatically, only selecting inputs from the account based on the given key
// scope and account number. If a key scope is not specified, then inputs from
// accounts matching the account number provided across all key scopes may be
// selected. This is done to handle the default account case, where a user wants
// to fund a PSBT with inputs regardless of their type (NP2WKH, P2WKH, etc.). If
// the packet does contain any inputs, it is assumed that full coin selection
// happened externally and no additional inputs are added. If the specified
// inputs aren't enough to fund the outputs with the given fee rate, an error is
// returned.
//
// NOTE: A caller of the method should hold the global coin selection lock of
// the wallet. However, no UTXO specific lock lease is acquired for any of the
// selected/validated inputs by this method. It is in the caller's
// responsibility to lock the inputs before handing the partial transaction out.
func (w *Wallet) FundPsbt(packet *psbt.Packet, keyScope *waddrmgr.KeyScope,
	minConfs int32, account uint32, feeSatPerKB ltcutil.Amount,
	coinSelectionStrategy CoinSelectionStrategy,
	optFuncs ...TxCreateOption) (int32, error) {

	// Make sure the packet is well formed. We only require there to be at
	// least one input or output.
	err := psbt.VerifyInputOutputLen(packet, false, false)
	if err != nil {
		return 0, err
	}

	if len(packet.Inputs) == 0 && len(packet.Outputs) == 0 {
		return 0, fmt.Errorf("PSBT packet must contain at least one " +
			"input or output")
	}

	txouts := packet.BuildTxOuts()

	// Make sure none of the outputs are dust.
	for _, txout := range txouts {
		// TODO: Skip for MWEB?

		// When checking an output for things like dusty-ness, we'll
		// use the default mempool relay fee rather than the target
		// effective fee rate to ensure accuracy. Otherwise, we may
		// mistakenly mark small-ish, but not quite dust output as
		// dust.
		err := txrules.CheckOutput(txout, txrules.DefaultRelayFeePerKb)
		if err != nil {
			return 0, err
		}
	}

	// Let's find out the amount to fund first.
	amt := int64(0)
	for _, output := range packet.Outputs {
		amt += int64(output.Amount)
	}

	var tx *txauthor.AuthoredTx
	switch {
	// We need to do coin selection.
	case len(packet.Inputs) == 0:
		// We ask the underlying wallet to fund a TX for us. This
		// includes everything we need, specifically fee estimation and
		// change address creation.
		tx, err = w.CreateSimpleTx(
			keyScope, account, txouts, minConfs,
			feeSatPerKB, coinSelectionStrategy, false,
			optFuncs...,
		)
		if err != nil {
			return 0, fmt.Errorf("error creating funding TX: %v",
				err)
		}

		// Copy over the inputs now then collect all UTXO information
		// that we can and attach them to the PSBT as well. We don't
		// include the witness as the resulting PSBT isn't expected not
		// should be signed yet.
		for _, txin := range tx.Tx.TxIn {
			input := psbt.PInput{
				PrevoutHash:  &txin.PreviousOutPoint.Hash,
				PrevoutIndex: &txin.PreviousOutPoint.Index,
				Sequence:     &txin.Sequence,
			}
			packet.Inputs = append(packet.Inputs, input)
		}
		err = addInputInfo(w, packet)
		if err != nil {
			return 0, err
		}

	// If there are inputs, we need to check if they're sufficient and add
	// a change output if necessary.
	default:
		// Make sure all inputs provided are actually ours.
		err = addInputInfo(w, packet)
		if err != nil {
			return 0, err
		}

		// We can leverage the fee calculation of the txauthor package
		// if we provide the selected UTXOs as a coin source. We just
		// need to make sure we always return the full list of user-
		// selected UTXOs rather than a subset, otherwise our change
		// amount will be off (in case the user selected multiple UTXOs
		// that are large enough on their own). That's why we use our
		// own static input source creator instead of the more generic
		// makeInputSource() that selects a subset that is "large
		// enough".
		credits := make([]wtxmgr.Credit, len(packet.Inputs))
		for idx, in := range packet.Inputs {
			utxo := in.WitnessUtxo
			credits[idx] = wtxmgr.Credit{
				OutPoint: wire.OutPoint{Hash: *in.PrevoutHash, Index: *in.PrevoutIndex},
				Amount:   ltcutil.Amount(utxo.Value),
				PkScript: utxo.PkScript,
			}
		}
		inputSource := constantInputSource(credits)

		// Build the TxCreateOption to retrieve the change scope.
		opts := defaultTxCreateOptions()
		for _, optFunc := range optFuncs {
			optFunc(opts)
		}

		if opts.changeKeyScope == nil {
			opts.changeKeyScope = keyScope
		}

		// We also need a change source which needs to be able to insert
		// a new change address into the database.
		err = walletdb.Update(w.db, func(dbtx walletdb.ReadWriteTx) error {
			_, changeSource, err := w.addrMgrWithChangeSource(
				dbtx, opts.changeKeyScope, account,
			)
			if err != nil {
				return err
			}

			// Ask the txauthor to create a transaction with our
			// selected coins. This will perform fee estimation and
			// add a change output if necessary.
			tx, err = txauthor.NewUnsignedTransaction(
				txouts, feeSatPerKB, inputSource, changeSource,
			)
			if err != nil {
				return fmt.Errorf("fee estimation not "+
					"successful: %v", err)
			}

			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("could not add change address to "+
				"database: %v", err)
		}
	}

	if tx.Tx.Mweb != nil {
		for _, kernel := range tx.Tx.Mweb.TxBody.Kernels {
			packet.Kernels = append(packet.Kernels, createKernelInfo(kernel))
		}
	}

	// If there is a change output, we need to copy it over to the PSBT now.
	var changeTxOut *wire.TxOut
	if tx.ChangeIndex >= 0 {
		changeTxOut = tx.Tx.TxOut[tx.ChangeIndex]
		addr, _, _, err := w.ScriptForOutput(changeTxOut)
		if err != nil {
			return 0, fmt.Errorf("error querying wallet for "+
				"change addr: %w", err)
		}

		changeOutputInfo, err := createOutputInfo(changeTxOut, addr)
		if err != nil {
			return 0, fmt.Errorf("error adding output info to "+
				"change output: %w", err)
		}

		packet.Outputs = append(packet.Outputs, *changeOutputInfo)
	}

	// Now that we have the final PSBT ready, we can sort it according to
	// BIP 69. This will sort the wire inputs and outputs and move the
	// partial inputs and outputs accordingly.
	err = psbt.InPlaceSort(packet)
	if err != nil {
		return 0, fmt.Errorf("could not sort PSBT: %v", err)
	}

	// The change output index might have changed after the sorting. We need
	// to find our index again.
	changeIndex := int32(-1)
	if changeTxOut != nil {
		for idx, output := range packet.Outputs {
			// TODO: Handle MWEB change
			if int64(output.Amount) == changeTxOut.Value && bytes.Equal(output.PKScript, changeTxOut.PkScript) {
				changeIndex = int32(idx)
				break
			}
		}
	}

	err = addKernelInfo(packet, feeSatPerKB)
	if err != nil {
		return 0, err
	}

	return changeIndex, nil
}

// addInputInfo is a helper function that fetches the UTXO information
// of an input and attaches it to the PSBT packet.
func addInputInfo(w *Wallet, packet *psbt.Packet) error {
	for idx, _ := range packet.Inputs {
		in := &packet.Inputs[idx]
		if in.PrevoutHash == nil || in.PrevoutIndex == nil {
			if in.MwebOutputId == nil {
				continue // TODO: Handle MWEB
			} else {
				return errors.New("invalid previous outpoint")
			}
		}

		prevOutPoint := wire.OutPoint{Hash: *in.PrevoutHash, Index: *in.PrevoutIndex}
		tx, utxo, derivationPath, _, err := w.FetchInputInfo(
			&prevOutPoint,
		)
		if err != nil {
			return fmt.Errorf("error fetching UTXO: %v",
				err)
		}

		addr, witnessProgram, _, err := w.ScriptForOutput(utxo)
		if err != nil {
			return fmt.Errorf("error fetching UTXO "+
				"script: %v", err)
		}

		switch {
		case txscript.IsPayToTaproot(utxo.PkScript):
			addInputInfoSegWitV1(in, utxo, derivationPath)
		case txscript.IsMweb(utxo.PkScript):
			mwebErr := addInputInfoMweb(w, in, tx, utxo)
			if mwebErr != nil {
				return mwebErr
			}
		default:
			addInputInfoSegWitV0(in, tx, utxo, derivationPath, addr, witnessProgram)
		}
	}

	return nil
}

// addInputInfoSegWitV0 adds the UTXO and BIP32 derivation info for a SegWit v0
// PSBT input (p2wkh, np2wkh) from the given wallet information.
func addInputInfoSegWitV0(in *psbt.PInput, prevTx *wire.MsgTx, utxo *wire.TxOut,
	derivationInfo *psbt.Bip32Derivation, addr waddrmgr.ManagedAddress,
	witnessProgram []byte) {

	// As a fix for CVE-2020-14199 we have to always include the full
	// non-witness UTXO in the PSBT for segwit v0.
	in.NonWitnessUtxo = prevTx

	// To make it more obvious that this is actually a witness output being
	// spent, we also add the same information as the witness UTXO.
	in.WitnessUtxo = &wire.TxOut{
		Value:    utxo.Value,
		PkScript: utxo.PkScript,
	}
	in.SighashType = txscript.SigHashAll

	// Include the derivation path for each input.
	in.Bip32Derivation = []*psbt.Bip32Derivation{
		derivationInfo,
	}

	// For nested P2WKH we need to add the redeem script to the input,
	// otherwise an offline wallet won't be able to sign for it. For normal
	// P2WKH this will be nil.
	if addr.AddrType() == waddrmgr.NestedWitnessPubKey {
		in.RedeemScript = witnessProgram
	}
}

// addInputInfoSegWitV0 adds the UTXO and BIP32 derivation info for a SegWit v1
// PSBT input (p2tr) from the given wallet information.
func addInputInfoSegWitV1(in *psbt.PInput, utxo *wire.TxOut,
	derivationInfo *psbt.Bip32Derivation) {

	// For SegWit v1 we only need the witness UTXO information.
	in.WitnessUtxo = &wire.TxOut{
		Value:    utxo.Value,
		PkScript: utxo.PkScript,
	}
	in.SighashType = txscript.SigHashDefault

	// Include the derivation path for each input in addition to the
	// taproot specific info we have below.
	in.Bip32Derivation = []*psbt.Bip32Derivation{
		derivationInfo,
	}

	// Include the derivation path for each input.
	in.TaprootBip32Derivation = []*psbt.TaprootBip32Derivation{{
		XOnlyPubKey:          derivationInfo.PubKey[1:],
		MasterKeyFingerprint: derivationInfo.MasterKeyFingerprint,
		Bip32Path:            derivationInfo.Bip32Path,
	}}
}

func addInputInfoMweb(w *Wallet, in *psbt.PInput, prevTx *wire.MsgTx, utxo *wire.TxOut) error {
	var mwebOutput *wire.MwebOutput
	err := walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		txmgrNs := tx.ReadBucket(wtxmgrNamespaceKey)
		prevOutPoint := wire.OutPoint{Hash: *in.PrevoutHash, Index: *in.PrevoutIndex}
		var err error
		mwebOutput, err = w.TxStore.GetMwebOutput(txmgrNs, &prevOutPoint, prevTx)
		return err
	})
	if err != nil {
		return err
	}

	in.WitnessUtxo = &wire.TxOut{
		Value:    utxo.Value,
		PkScript: utxo.PkScript,
	}

	value := ltcutil.Amount(utxo.Value)
	in.MwebAmount = &value

	in.MwebOutputId = mwebOutput.Hash()
	in.MwebKeyExchangePubkey = &mwebOutput.Message.KeyExchangePubKey
	in.MwebCommit = &mwebOutput.Commitment
	in.MwebOutputPubkey = &mwebOutput.ReceiverPubKey

	// TODO: MwebMasterScanKey and MwebMasterSpendKey Bip32Derivation's? MwebAddressIndex?
	return nil
}

func addKernelInfo(packet *psbt.Packet, feeRatePerKb ltcutil.Amount) error {
	ltcIn := ltcutil.Amount(0)
	mwebIn := ltcutil.Amount(0)
	for _, in := range packet.Inputs {
		if in.MwebAmount != nil {
			mwebIn += *in.MwebAmount
		} else {
			ltcIn += ltcutil.Amount(in.WitnessUtxo.Value)
		}
	}

	ltcOut := ltcutil.Amount(0)
	mwebOut := ltcutil.Amount(0)
	for _, out := range packet.Outputs {
		if out.StealthAddress != nil {
			mwebOut += out.Amount
		} else {
			ltcOut += out.Amount
		}
	}

	if mwebIn == 0 && mwebOut == 0 {
		return nil
	}

	kernel := psbt.PKernel{}

	// If there are *any* MWEB inputs, all LTC outputs become pegouts.
	pegoutSum := ltcutil.Amount(0)
	if ltcOut > 0 && mwebIn > 0 {
		mwebOutputIdx := 0
		for _, out := range packet.Outputs {
			if out.StealthAddress == nil {
				pegoutSum += out.Amount
				kernel.PegOuts = append(kernel.PegOuts, &wire.TxOut{
					Value:    int64(out.Amount),
					PkScript: out.PKScript,
				})
			} else {
				packet.Outputs[mwebOutputIdx] = out
				mwebOutputIdx++
			}
		}

		// Non-MWEB outputs have become pegouts, and packet.Outputs will now contain only MWEB outputs.
		packet.Outputs = packet.Outputs[:mwebOutputIdx]
	}

	// Add Pegin
	if ltcIn > 0 {
		mwebFee := ltcutil.Amount(mweb.EstimateFee(packet.BuildTxOuts(), feeRatePerKb, false))
		peginAmount := (mwebOut + pegoutSum) + mwebFee - mwebIn

		kernel.PeginAmount = &peginAmount
		kernel.Fee = &mwebFee
	} else {
		mwebFee := mwebIn - (mwebOut + pegoutSum)
		kernel.Fee = &mwebFee
	}

	packet.Kernels = append(packet.Kernels, kernel)
	return nil
}

func createKernelInfo(kernel *wire.MwebKernel) psbt.PKernel {
	var pKern psbt.PKernel
	fee := ltcutil.Amount(kernel.Fee)
	pKern.Fee = &fee

	if kernel.Pegin > 0 {
		pegin := ltcutil.Amount(kernel.Pegin)
		pKern.PeginAmount = &pegin
	}

	if len(kernel.Pegouts) > 0 {
		pKern.PegOuts = make([]*wire.TxOut, len(kernel.Pegouts))
		copy(pKern.PegOuts, kernel.Pegouts)
	}

	if kernel.LockHeight > 0 {
		pKern.LockHeight = &kernel.LockHeight
	}

	if len(kernel.ExtraData) > 0 {
		pKern.ExtraData = make([]byte, len(kernel.ExtraData))
		copy(pKern.ExtraData, kernel.ExtraData)
	}

	return pKern
}

// createOutputInfo creates the BIP32 derivation info for an output from our
// internal wallet.
func createOutputInfo(txOut *wire.TxOut,
	addr waddrmgr.ManagedPubKeyAddress) (*psbt.POutput, error) {

	// We don't know the derivation path for imported keys. Those shouldn't
	// be selected as change outputs in the first place, but just to make
	// sure we don't run into an issue, we return early for imported keys.
	keyScope, derivationPath, isKnown := addr.DerivationInfo()
	if !isKnown {
		return nil, fmt.Errorf("error adding output info to PSBT, " +
			"change addr is an imported addr with unknown " +
			"derivation path")
	}

	// Include the derivation path for this output.
	derivation := &psbt.Bip32Derivation{
		PubKey:               addr.PubKey().SerializeCompressed(),
		MasterKeyFingerprint: derivationPath.MasterKeyFingerprint,
		Bip32Path: []uint32{
			keyScope.Purpose + hdkeychain.HardenedKeyStart,
			keyScope.Coin + hdkeychain.HardenedKeyStart,
			derivationPath.Account,
			derivationPath.Branch,
			derivationPath.Index,
		},
	}
	out := &psbt.POutput{
		Amount: ltcutil.Amount(txOut.Value),
		Bip32Derivation: []*psbt.Bip32Derivation{
			derivation,
		},
	}

	// Include the Taproot derivation path as well if this is a P2TR output.
	if txscript.IsPayToTaproot(txOut.PkScript) {
		schnorrPubKey := derivation.PubKey[1:]
		out.TaprootBip32Derivation = []*psbt.TaprootBip32Derivation{{
			XOnlyPubKey:          schnorrPubKey,
			MasterKeyFingerprint: derivation.MasterKeyFingerprint,
			Bip32Path:            derivation.Bip32Path,
		}}
		out.TaprootInternalKey = schnorrPubKey
	}

	if txscript.IsMweb(txOut.PkScript) {
		out.StealthAddress = &mw.StealthAddress{
			Scan:  (*mw.PublicKey)(txOut.PkScript[:33]),
			Spend: (*mw.PublicKey)(txOut.PkScript[33:]),
		}
	}

	return out, nil
}

// FinalizePsbt expects a partial transaction with all inputs and outputs fully
// declared and tries to sign all inputs that belong to the wallet. Our wallet
// must be the last signer of the transaction. That means, if there are any
// unsigned non-witness inputs or inputs without UTXO information attached or
// inputs without witness data that do not belong to the wallet, this method
// will fail. If no error is returned, the PSBT is ready to be extracted and the
// final TX within to be broadcast.
//
// NOTE: This method does NOT publish the transaction after it's been finalized
// successfully.
func (w *Wallet) FinalizePsbt(keyScope *waddrmgr.KeyScope, account uint32, packet *psbt.Packet) error {

	// Let's check that this is actually something we can and want to sign.
	// We need at least one input and one output. In addition each
	// input needs nonWitness Utxo or witness Utxo data specified.
	err := psbt.InputsReadyToSign(packet)
	if err != nil {
		return err
	}

	mwebInputSigner := psbt.BasicMwebInputSigner{
		DeriveOutputKeys: w.mwebDeriveOutputKeys,
	}
	psbtSigner, err := psbt.NewSigner(packet, mwebInputSigner)
	if err != nil {
		return fmt.Errorf("error creating PSBT signer: %v", err)
	}
	signOutcome, err := psbtSigner.SignMwebComponents()
	if err != nil {
		return fmt.Errorf("error during MWEB signing: %v", err)
	} else if signOutcome != psbt.SignSuccesful {
		return fmt.Errorf("mweb components not signed successfully")
	}

	tx, err := psbt.ExtractUnsignedTx(packet)
	if err != nil {
		return err
	}

	// Go through each input that doesn't have final witness data attached
	// to it already and try to sign it. We do expect that we're the last
	// ones to sign. If there is any input without witness data that we
	// cannot sign because it's not our UTXO, this will be a hard failure.
	sigHashes := txscript.NewTxSigHashes(tx, PsbtPrevOutputFetcher(packet))
	for idx, in := range packet.Inputs {
		// MWEB inputs are signed below this loop
		if in.MwebOutputId != nil {
			continue
		}

		// We can only sign if we have UTXO information available. We
		// can just continue here as a later step will fail with a more
		// precise error message.
		if in.WitnessUtxo == nil && in.NonWitnessUtxo == nil {
			continue
		}

		// Skip this input if it's got final witness data attached.
		if len(in.FinalScriptWitness) > 0 {
			continue
		}

		// We can only sign this input if it's ours, so we try to map it
		// to a coin we own. If we can't, then we'll continue as it
		// isn't our input.
		fullTx, txOut, _, _, err := w.FetchInputInfo(
			&wire.OutPoint{
				Hash:  *in.PrevoutHash,
				Index: *in.PrevoutIndex,
			},
		)
		if err != nil {
			continue
		}

		// Find out what UTXO we are signing. Wallets _should_ always
		// provide the full non-witness UTXO for segwit v0.
		var signOutput *wire.TxOut
		if in.NonWitnessUtxo != nil {
			prevIndex := *in.PrevoutIndex
			signOutput = in.NonWitnessUtxo.TxOut[prevIndex]

			if !psbt.TxOutsEqual(txOut, signOutput) {
				return fmt.Errorf("found UTXO %#v but it "+
					"doesn't match PSBT's input %v", txOut,
					signOutput)
			}

			if fullTx.TxHash() != *in.PrevoutHash {
				return fmt.Errorf("found UTXO tx %v but it "+
					"doesn't match PSBT's input %v",
					fullTx.TxHash(),
					*in.PrevoutHash)
			}
		}

		// Fall back to witness UTXO only for older wallets.
		if in.WitnessUtxo != nil {
			signOutput = in.WitnessUtxo

			if !psbt.TxOutsEqual(txOut, signOutput) {
				return fmt.Errorf("found UTXO %#v but it "+
					"doesn't match PSBT's input %v", txOut,
					signOutput)
			}
		}

		// Finally, if the input doesn't belong to a watch-only account,
		// then we'll sign it as is, and populate the input with the
		// witness and sigScript (if needed).
		watchOnly := false
		err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
			ns := tx.ReadBucket(waddrmgrNamespaceKey)
			var err error
			if keyScope == nil {
				// If a key scope wasn't specified, then coin
				// selection was performed from the default
				// wallet accounts (NP2WKH, P2WKH, P2TR), so any
				// key scope provided doesn't impact the result
				// of this call.
				watchOnly, err = w.Manager.IsWatchOnlyAccount(
					ns, waddrmgr.KeyScopeBIP0084, account,
				)
			} else {
				watchOnly, err = w.Manager.IsWatchOnlyAccount(
					ns, *keyScope, account,
				)
			}
			return err
		})
		if err != nil {
			return fmt.Errorf("unable to determine if account is "+
				"watch-only: %v", err)
		}
		if watchOnly {
			continue
		}

		witness, sigScript, err := w.ComputeInputScript(
			tx, signOutput, idx, sigHashes, in.SighashType, nil,
		)
		if err != nil {
			return fmt.Errorf("error computing input script for "+
				"input %d: %v", idx, err)
		}

		// Serialize the witness format from the stack representation to
		// the wire representation.
		var witnessBytes bytes.Buffer
		err = psbt.WriteTxWitness(&witnessBytes, witness)
		if err != nil {
			return fmt.Errorf("error serializing witness: %v", err)
		}
		packet.Inputs[idx].FinalScriptWitness = witnessBytes.Bytes()
		packet.Inputs[idx].FinalScriptSig = sigScript
	}

	// Make sure the PSBT itself thinks it's finalized and ready to be
	// broadcast.
	err = psbt.MaybeFinalizeAll(packet)
	if err != nil {
		return fmt.Errorf("error finalizing PSBT: %v", err)
	}

	return nil
}

// PsbtPrevOutputFetcher returns a txscript.PrevOutFetcher built from the UTXO
// information in a PSBT packet.
func PsbtPrevOutputFetcher(packet *psbt.Packet) *txscript.MultiPrevOutFetcher {
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	for _, in := range packet.Inputs {
		if in.MwebOutputId != nil {
			continue // TODO: MWEB
		}

		// Skip any input that has no UTXO.
		if in.WitnessUtxo == nil && in.NonWitnessUtxo == nil {
			continue
		}

		prevOutPoint := wire.OutPoint{Hash: *in.PrevoutHash, Index: *in.PrevoutIndex}

		if in.NonWitnessUtxo != nil {
			prevIndex := *in.PrevoutIndex
			fetcher.AddPrevOut(
				prevOutPoint,
				in.NonWitnessUtxo.TxOut[prevIndex],
			)

			continue
		}

		// Fall back to witness UTXO only for older wallets.
		if in.WitnessUtxo != nil {
			fetcher.AddPrevOut(
				prevOutPoint, in.WitnessUtxo,
			)
		}
	}

	return fetcher
}

func (w *Wallet) mwebDeriveOutputKeys(spentOutputPk *mw.PublicKey, keyExchangePubKey *mw.PublicKey, spentOutputSharedSecret *mw.SecretKey) (*mw.BlindingFactor, *mw.SecretKey, error) {
	var preBlind *mw.BlindingFactor
	var outputSpendKey *mw.SecretKey

	// The key wasn't in the cache, let's fully derive it now.
	err := walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		ns := tx.ReadBucket(waddrmgrNamespaceKey)
		err := w.forEachMwebAccount(ns, func(ma *mwebAccount) error {
			if outputSpendKey != nil {
				return nil
			}

			mwebKeychain, err := ma.skm.LoadMwebKeychain(ns, ma.account)
			if mwebKeychain != nil {
				fmt.Printf("Master Scan: %x, Master Spend: %x\n", *mwebKeychain.Scan, *mwebKeychain.Spend)

				sharedSecret := spentOutputSharedSecret
				if sharedSecret == nil {
					if keyExchangePubKey == nil {
						return errors.New("key exchange pubkey or shared secret needed")
					}
					sharedSecretPk := keyExchangePubKey.Mul(mwebKeychain.Scan)
					sharedSecret = (*mw.SecretKey)(mw.Hashed(mw.HashTagDerive, sharedSecretPk[:]))
				}

				addrB := spentOutputPk.Div((*mw.SecretKey)(mw.Hashed(mw.HashTagOutKey, sharedSecret[:])))
				addrA := addrB.Mul(mwebKeychain.Scan)
				address := mw.StealthAddress{Scan: addrA, Spend: addrB}
				addr := ltcutil.NewAddressMweb(&address, w.chainParams)

				secrets := secretSource{w.Manager, ns}
				spendKeyPriv, _, err := secrets.GetKey(addr)
				if err != nil {
					return nil
				}

				defer spendKeyPriv.Zero()
				addrSpendSecret := (*mw.SecretKey)(spendKeyPriv.Serialize())
				defer zero.Bytes(addrSpendSecret[:])

				// Calculate pre-blind and output spend key
				preBlind = (*mw.BlindingFactor)(mw.Hashed(mw.HashTagBlind, sharedSecret[:]))
				outputSpendKey = addrSpendSecret.Mul((*mw.SecretKey)(mw.Hashed(mw.HashTagOutKey, sharedSecret[:])))
			}
			return err
		})
		return err
	})

	if err != nil {
		return nil, nil, err
	}

	if preBlind == nil || outputSpendKey == nil {
		return nil, nil, fmt.Errorf("no keychain found for output %x", *spentOutputPk)
	}

	return preBlind, outputSpendKey, nil
}
