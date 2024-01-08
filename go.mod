module github.com/ltcsuite/ltcwallet

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f
	github.com/btcsuite/websocket v0.0.0-20150119174127-31079b680792
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1
	github.com/jessevdk/go-flags v1.4.0
	github.com/jrick/logrotate v1.0.0
	github.com/lightninglabs/gozmq v0.0.0-20191113021534-d20a764486bf
	github.com/ltcsuite/lnd/ticker v1.1.0
	github.com/ltcsuite/lnd/tlv v1.1.1
	github.com/ltcsuite/ltcd v0.23.5
	github.com/ltcsuite/ltcd/btcec/v2 v2.3.2
	github.com/ltcsuite/ltcd/chaincfg/chainhash v1.0.2
	github.com/ltcsuite/ltcd/ltcutil v1.1.3
	github.com/ltcsuite/ltcd/ltcutil/psbt v1.1.8
	github.com/ltcsuite/ltcwallet/wallet/txauthor v1.3.2
	github.com/ltcsuite/ltcwallet/wallet/txrules v1.2.0
	github.com/ltcsuite/ltcwallet/wallet/txsizes v1.2.3
	github.com/ltcsuite/ltcwallet/walletdb v1.4.0
	github.com/ltcsuite/ltcwallet/wtxmgr v1.5.0
	github.com/ltcsuite/neutrino v0.16.0
	github.com/ltcsuite/neutrino/cache v1.1.1
	github.com/stretchr/testify v1.8.2
	golang.org/x/crypto v0.7.0
	golang.org/x/net v0.10.0
	golang.org/x/term v0.8.0
	google.golang.org/grpc v1.53.0
	google.golang.org/protobuf v1.28.1
)

require (
	github.com/aead/siphash v1.0.1 // indirect
	github.com/btcsuite/go-socks v0.0.0-20170105172521-4720035b7bfd // indirect
	github.com/decred/dcrd/crypto/blake256 v1.0.0 // indirect
	github.com/decred/dcrd/lru v1.1.1 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/kkdai/bstream v1.0.0 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/ltcsuite/lnd/clock v1.1.0 // indirect
	github.com/ltcsuite/lnd/queue v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	go.etcd.io/bbolt v1.3.7 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	google.golang.org/genproto v0.0.0-20230110181048-76db0878b65f // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
)

go 1.18

replace github.com/ltcsuite/ltcwallet/walletdb => ./walletdb

replace github.com/ltcsuite/neutrino => ../neutrino

replace github.com/ltcsuite/neutrino/cache => ../neutrino/cache

replace github.com/ltcsuite/lnd/tlv => ../lnd/tlv

replace github.com/ltcsuite/ltcd/ltcutil/psbt => ../ltcd/ltcutil/psbt

replace github.com/ltcsuite/ltcwallet/wallet/txauthor => ./wallet/txauthor

replace github.com/ltcsuite/ltcwallet/wallet/txsizes => ./wallet/txsizes

// loshy temp:

replace github.com/ltcsuite/ltcd => ../ltcd

replace github.com/ltcsuite/ltcd/ltcutil => ../ltcd/ltcutil

replace github.com/ltcsuite/ltcd/btcec/v2 => ../ltcd/btcec

replace github.com/ltcsuite/ltcd/chaincfg/chainhash => ../ltcd/chaincfg/chainhash

replace github.com/ltcsuite/ltcwallet/wallet/txrules => ./wallet/txrules

replace github.com/ltcsuite/ltcwallet/wtxmgr => ./wtxmgr
