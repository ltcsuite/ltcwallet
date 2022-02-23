// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netparams

import (
	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/wire"
)

// Params is used to group parameters for various networks such as the main
// network and test networks.
type Params struct {
	*chaincfg.Params
	RPCClientPort string
	RPCServerPort string
}

// MainNetParams contains parameters specific running btcwallet and
// btcd on the main network (wire.MainNet).
var MainNetParams = Params{
	Params:        &chaincfg.MainNetParams,
	RPCClientPort: "9334",
	RPCServerPort: "9332",
}

// TestNet4Params contains parameters specific running btcwallet and
// btcd on the test network (version 4) (wire.TestNet4).
var TestNet4Params = Params{
	Params:        &chaincfg.TestNet4Params,
	RPCClientPort: "19334",
	RPCServerPort: "19332",
}

// SimNetParams contains parameters specific to the simulation test network
// (wire.SimNet).
var SimNetParams = Params{
	Params:        &chaincfg.SimNetParams,
	RPCClientPort: "18556",
	RPCServerPort: "18554",
}

// SigNetParams contains parameters specific to the signet test network
// (wire.SigNet).
var SigNetParams = Params{
	Params:        &chaincfg.SigNetParams,
	RPCClientPort: "38334",
	RPCServerPort: "38332",
}

// SigNetWire is a helper function that either returns the given chain
// parameter's net value if the parameter represents a signet network or 0 if
// it's not. This is necessary because there can be custom signet networks that
// have a different net value.
func SigNetWire(params *chaincfg.Params) wire.BitcoinNet {
	if params.Name == chaincfg.SigNetParams.Name {
		return params.Net
	}

	return 0
}
