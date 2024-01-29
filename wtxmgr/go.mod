module github.com/ltcsuite/ltcwallet/wtxmgr

go 1.12

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/ltcsuite/lnd/clock v1.1.0
	github.com/ltcsuite/ltcd v0.23.5
	github.com/ltcsuite/ltcd/chaincfg/chainhash v1.0.2
	github.com/ltcsuite/ltcd/ltcutil v1.1.3
	github.com/ltcsuite/ltcwallet/walletdb v1.3.5
	github.com/onsi/gomega v1.26.0 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	github.com/stretchr/testify v1.8.2 // indirect
	go.etcd.io/bbolt v1.3.7 // indirect
	golang.org/x/crypto v0.7.0 // indirect
	golang.org/x/net v0.10.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace github.com/ltcsuite/ltcd => ../../ltcd

replace github.com/ltcsuite/ltcd/btcec/v2 => ../../ltcd/btcec

replace github.com/ltcsuite/ltcd/chaincfg/chainhash => ../../ltcd/chaincfg/chainhash

replace github.com/ltcsuite/ltcd/ltcutil => ../../ltcd/ltcutil
