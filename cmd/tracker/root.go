package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// RootCmd is the exported cobra command that can be used by main.go
var RootCmd = &cobra.Command{
	Use:   "solana-block-qa-tracker [interval]",
	Short: "A tool to compare Solana blocks between Firehose and RPC Fetcher",
	Long: `Solana Block QA Tracker compares blocks between StreamingFast Firehose and RPC Fetcher 
to ensure data consistency. It runs periodic comparisons at the specified interval.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		interval, err := time.ParseDuration(args[0])
		if err != nil {
			return fmt.Errorf("invalid interval format: %w (examples: 30s, 5m, 1h)", err)
		}
		slackWebhookURL, _ := cmd.Flags().GetString("slack-webhook-url")
		slackChannel, _ := cmd.Flags().GetString("slack-channel")
		firehoseEndpoint, _ := cmd.Flags().GetString("firehose-endpoint")
		solanaRPCEndpoint, _ := cmd.Flags().GetString("solana-rpc-endpoint")

		// Create a new Tracker instance
		tracker := NewTracker(zlog, slackWebhookURL, slackChannel, firehoseEndpoint, solanaRPCEndpoint)
		return tracker.runTracker(interval)
	},
}

func init() {
	RootCmd.Flags().String("slack-webhook-url", "", "Slack webhook URL for notifications")
	RootCmd.Flags().String("slack-channel", "solana", "Slack channel for notifications (default: #general)")
	RootCmd.Flags().String("firehose-endpoint", "mainnet.sol.streamingfast.io:443", "StreamingFast Solana Firehose endpoint")
	RootCmd.Flags().String("solana-rpc-endpoint", "https://api.mainnet-beta.solana.com", "Solana RPC endpoint")
}
