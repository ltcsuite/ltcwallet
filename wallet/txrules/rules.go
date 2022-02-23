// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package txrules provides transaction rules that should be followed by
// transaction authors for wide mempool acceptance and quick mining.
package txrules

import (
	"errors"

	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/mempool"
	"github.com/ltcsuite/ltcd/txscript"
	"github.com/ltcsuite/ltcd/wire"
)

// DefaultRelayFeePerKb is the default minimum relay fee policy for a mempool.
const DefaultRelayFeePerKb ltcutil.Amount = 1e3

// IsDustOutput determines whether a transaction output is considered dust.
// Transactions with dust outputs are not standard and are rejected by mempools
// with default policies.
func IsDustOutput(output *wire.TxOut, relayFeePerKb ltcutil.Amount) bool {
	// Unspendable outputs which solely carry data are not checked for dust.
	if txscript.GetScriptClass(output.PkScript) == txscript.NullDataTy {
		return false
	}

	return mempool.IsDust(output, relayFeePerKb)
}

// Transaction rule violations
var (
	ErrAmountNegative   = errors.New("transaction output amount is negative")
	ErrAmountExceedsMax = errors.New("transaction output amount exceeds maximum value")
	ErrOutputIsDust     = errors.New("transaction output is dust")
)

// CheckOutput performs simple consensus and policy tests on a transaction
// output.
func CheckOutput(output *wire.TxOut, relayFeePerKb ltcutil.Amount) error {
	if output.Value < 0 {
		return ErrAmountNegative
	}
	if output.Value > ltcutil.MaxSatoshi {
		return ErrAmountExceedsMax
	}
	if IsDustOutput(output, relayFeePerKb) {
		return ErrOutputIsDust
	}
	return nil
}

// FeeForSerializeSize calculates the required fee for a transaction of some
// arbitrary size given a mempool's relay fee policy.
func FeeForSerializeSize(relayFeePerKb ltcutil.Amount, txSerializeSize int) ltcutil.Amount {
	fee := relayFeePerKb * ltcutil.Amount(txSerializeSize) / 1000

	if fee == 0 && relayFeePerKb > 0 {
		fee = relayFeePerKb
	}

	if fee < 0 || fee > ltcutil.MaxSatoshi {
		fee = ltcutil.MaxSatoshi
	}

	return fee
}
