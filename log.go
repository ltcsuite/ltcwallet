// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/btcsuite/btclog"
	"github.com/jrick/logrotate/rotator"
	"github.com/ltcsuite/ltcd/rpcclient"
	"github.com/ltcsuite/ltcwallet/chain"
	"github.com/ltcsuite/ltcwallet/rpc/legacyrpc"
	"github.com/ltcsuite/ltcwallet/rpc/rpcserver"
	"github.com/ltcsuite/ltcwallet/wallet"
	"github.com/ltcsuite/ltcwallet/wtxmgr"
	"github.com/ltcsuite/neutrino"
)

// logWriter implements an io.Writer that outputs to both standard output and
// the write-end pipe of an initialized log rotator.
type logWriter struct{}

func (logWriter) Write(p []byte) (n int, err error) {
	_, _ = os.Stdout.Write(p)
	_, _ = logRotatorPipe.Write(p)
	return len(p), nil
}

// Loggers per subsystem.  A single backend logger is created and all subsytem
// loggers created from it will write to the backend.  When adding new
// subsystems, add the subsystem logger variable here and to the
// subsystemLoggers map.
//
// Loggers can not be used before the log rotator has been initialized with a
// log file.  This must be performed early during application startup by calling
// initLogRotator.
var (
	// backendLog is the logging backend used to create all subsystem loggers.
	// The backend must not be used before the log rotator has been initialized,
	// or data races and/or nil pointer dereferences will occur.
	backendLog = btclog.NewBackend(logWriter{})

	// logRotator is one of the logging outputs.  It should be closed on
	// application shutdown.
	logRotator *rotator.Rotator

	// logRotatorPipe is the write-end pipe for writing to the log rotator.  It
	// is written to by the Write method of the logWriter type.
	logRotatorPipe *io.PipeWriter

	log          = backendLog.Logger("LTCW")
	walletLog    = backendLog.Logger("WLLT")
	txmgrLog     = backendLog.Logger("TMGR")
	chainLog     = backendLog.Logger("CHNS")
	grpcLog      = backendLog.Logger("GRPC")
	legacyRPCLog = backendLog.Logger("RPCS")
	btcnLog      = backendLog.Logger("LTCN")
)

// Initialize package-global logger variables.
func init() {
	wallet.UseLogger(walletLog)
	wtxmgr.UseLogger(txmgrLog)
	chain.UseLogger(chainLog)
	rpcclient.UseLogger(chainLog)
	rpcserver.UseLogger(grpcLog)
	legacyrpc.UseLogger(legacyRPCLog)
	neutrino.UseLogger(btcnLog)
}

// subsystemLoggers maps each subsystem identifier to its associated logger.
var subsystemLoggers = map[string]btclog.Logger{
	"LTCW": log,
	"WLLT": walletLog,
	"TMGR": txmgrLog,
	"CHNS": chainLog,
	"GRPC": grpcLog,
	"RPCS": legacyRPCLog,
	"LTCN": btcnLog,
}

// initLogRotator initializes the logging rotater to write logs to logFile and
// create roll files in the same directory.  It must be called before the
// package-global log rotater variables are used.
func initLogRotator(logFile string) {
	logDir, _ := filepath.Split(logFile)
	err := os.MkdirAll(logDir, 0700)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create log directory: %v\n", err)
		os.Exit(1)
	}
	r, err := rotator.New(logFile, 10*1024, false, 3)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create file rotator: %v\n", err)
		os.Exit(1)
	}

	pr, pw := io.Pipe()
	go func() { _ = r.Run(pr) }()

	logRotator = r
	logRotatorPipe = pw
}

// setLogLevel sets the logging level for provided subsystem.  Invalid
// subsystems are ignored.  Uninitialized subsystems are dynamically created as
// needed.
func setLogLevel(subsystemID string, logLevel string) {
	// Ignore invalid subsystems.
	logger, ok := subsystemLoggers[subsystemID]
	if !ok {
		return
	}

	// Defaults to info if the log level is invalid.
	level, _ := btclog.LevelFromString(logLevel)
	logger.SetLevel(level)
}

// setLogLevels sets the log level for all subsystem loggers to the passed
// level.  It also dynamically creates the subsystem loggers as needed, so it
// can be used to initialize the logging system.
func setLogLevels(logLevel string) {
	// Configure all sub-systems with the new logging level.  Dynamically
	// create loggers as needed.
	for subsystemID := range subsystemLoggers {
		setLogLevel(subsystemID, logLevel)
	}
}
