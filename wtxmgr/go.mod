module github.com/ltcsuite/ltcwallet/wtxmgr

go 1.17

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f
	github.com/ltcsuite/lnd/clock v1.1.0
	github.com/ltcsuite/ltcd v0.23.5
	github.com/ltcsuite/ltcd/chaincfg/chainhash v1.0.2
	github.com/ltcsuite/ltcd/ltcutil v1.1.3
	github.com/ltcsuite/ltcwallet/walletdb v1.3.5
	github.com/stretchr/testify v1.8.2 // indirect
	go.etcd.io/bbolt v1.3.7 // indirect
	golang.org/x/crypto v0.7.0 // indirect
)

require (
	github.com/decred/dcrd/crypto/blake256 v1.0.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/ltcsuite/ltcd/btcec/v2 v2.3.2 // indirect
	golang.org/x/sys v0.8.0 // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
)

replace github.com/ltcsuite/ltcd => ../../ltcd

replace github.com/ltcsuite/ltcd/btcec/v2 => ../../ltcd/btcec

replace github.com/ltcsuite/ltcd/chaincfg/chainhash => ../../ltcd/chaincfg/chainhash

replace github.com/ltcsuite/ltcd/ltcutil => ../../ltcd/ltcutil
