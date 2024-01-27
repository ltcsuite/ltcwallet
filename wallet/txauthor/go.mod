module github.com/ltcsuite/ltcwallet/wallet/txauthor

go 1.17

require (
	github.com/ltcsuite/ltcd v0.23.5
	github.com/ltcsuite/ltcd/ltcutil v1.1.3
	github.com/ltcsuite/ltcwallet/wallet/txrules v1.2.0
	github.com/ltcsuite/ltcwallet/wallet/txsizes v1.2.3
)

replace github.com/ltcsuite/ltcwallet/wallet/txsizes => ../txsizes
