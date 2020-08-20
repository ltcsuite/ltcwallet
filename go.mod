module github.com/ltcsuite/ltcwallet

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f
	github.com/ltcsuite/ltcwallet/wallet/txauthor v1.0.0
	github.com/ltcsuite/ltcwallet/wallet/txrules v1.0.0
	github.com/ltcsuite/ltcwallet/walletdb v1.3.3
	github.com/ltcsuite/ltcwallet/wtxmgr v1.2.0
	github.com/btcsuite/websocket v0.0.0-20150119174127-31079b680792
	github.com/davecgh/go-spew v1.1.1
	github.com/golang/protobuf v1.2.0
	github.com/jessevdk/go-flags v1.4.0
	github.com/jrick/logrotate v1.0.0
	github.com/lightninglabs/gozmq v0.0.0-20191113021534-d20a764486bf
	github.com/lightningnetwork/lnd/queue v1.0.4 // indirect
	github.com/ltcsuite/ltcd v0.20.1-beta.0.20200617222819-81094527da29
	github.com/ltcsuite/ltcutil v1.0.2-beta
	github.com/ltcsuite/neutrino v0.11.0
	golang.org/x/crypto v0.0.0-20200604202706-70a84ac30bf9
	golang.org/x/net v0.0.0-20190404232315-eb5bcb51f2a3
	google.golang.org/grpc v1.18.0
)

go 1.13
