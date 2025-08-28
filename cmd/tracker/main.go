package main

import (
	"os"

	"github.com/streamingfast/logging"
	"go.uber.org/zap"
)

var zlog *zap.Logger

func main() {
	zlog = logging.MustCreateLoggerWithServiceName("solana-block-qa-tracker")
	defer zlog.Sync()

	if err := RootCmd.Execute(); err != nil {
		zlog.Error("Application error", zap.Error(err))
		os.Exit(1)
	}
}
