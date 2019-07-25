module github.com/ltcsuite/ltcwallet

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f
	github.com/btcsuite/golangcrypto v0.0.0-20150304025918-53f62d9b43e8
	github.com/btcsuite/websocket v0.0.0-20150119174127-31079b680792
	github.com/coreos/bbolt v1.3.2
	github.com/davecgh/go-spew v1.1.1
	github.com/golang/protobuf v1.2.0
	github.com/jessevdk/go-flags v1.4.0
	github.com/jrick/logrotate v1.0.0
	github.com/lightninglabs/gozmq v0.0.0-20180324010646-462a8a753885
	github.com/ltcsuite/ltcd v0.0.0-20190519120615-e27ee083f08f
	github.com/ltcsuite/ltcutil v0.0.0
	github.com/ltcsuite/neutrino v0.0.0-20190105125846-26fb2f58fe6b
	go.etcd.io/bbolt v1.3.3 // indirect
	golang.org/x/crypto v0.0.0-20190618222545-ea8f1a30c443
	golang.org/x/net v0.0.0-20190404232315-eb5bcb51f2a3
	google.golang.org/grpc v1.18.0
)

replace github.com/ltcsuite/ltcutil => ../ltcutil

replace github.com/ltcsuite/ltcd => ../ltcd

replace github.com/ltcsuite/neutrino => ../neutrino
