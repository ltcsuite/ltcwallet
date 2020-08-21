module github.com/ltcsuite/ltcwallet/wtxmgr

go 1.12

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f
	github.com/ltcsuite/lnd/clock v1.0.1
	github.com/ltcsuite/ltcd v0.20.1-beta
	github.com/ltcsuite/ltcutil v0.0.0-20191227053721-6bec450ea6ad
	github.com/ltcsuite/ltcwallet/walletdb v1.3.2
	github.com/stretchr/testify v1.5.1 // indirect
	golang.org/x/crypto v0.0.0-20200820211705-5c72a883971a // indirect
)

replace github.com/ltcsuite/lnd/clock => ../../lnd/clock
