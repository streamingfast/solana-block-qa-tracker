package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mostynb/go-grpc-compression/zstd"
	"github.com/slack-go/slack"
	"github.com/spf13/cobra"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/firehose-solana/block/fetcher"
	pbsol "github.com/streamingfast/firehose-solana/pb/sf/solana/type/v1"
	pbfirehose "github.com/streamingfast/pbgo/sf/firehose/v2"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// RPCFetcher interface for block fetching
type RPCFetcher interface {
	Fetch(ctx context.Context, client *rpc.Client, requestedSlot uint64) (b *pbbstream.Block, skipped bool, err error)
}

// Tracker manages RPC clients, logger, and block comparison operations
type Tracker struct {
	logger              *zap.Logger
	slackWebhookURL     string
	slackChannel        string
	firehoseEndpoint    string
	solanaRPCEndpoint   string
	// Reusable clients
	firehoseConn        *grpc.ClientConn
	firehoseClient      pbfirehose.StreamClient
	rpcFetcher          RPCFetcher
	rpcClient           *rpc.Client
}

// NewTracker creates a new Tracker instance with the provided configuration
func NewTracker(logger *zap.Logger, slackWebhookURL, slackChannel, firehoseEndpoint, solanaRPCEndpoint string) *Tracker {
	// Setup connection options with TLS and increased message size limits for firehose
	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	// Set max receive message size to 1GB to handle large Solana blocks
	dialOptions = append(dialOptions, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1024*1024*1024)))
	// Set max send message size to 1GB for completeness
	dialOptions = append(dialOptions, grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(1024*1024*1024)))

	// Create gRPC connection for firehose (will be reused)
	conn, err := grpc.Dial(firehoseEndpoint, dialOptions...)
	if err != nil {
		logger.Fatal("failed to connect to Firehose", zap.Error(err))
	}

	// Create Firehose client (will be reused)
	firehoseClient := pbfirehose.NewStreamClient(conn)

	// Create RPCFetcher instance (will be reused)
	rpcFetcher := fetcher.NewRPC(time.Second*5, true, false, logger) // 5s retry interval, mainnet=true

	// Create RPC client (will be reused)
	rpcClient := rpc.New(solanaRPCEndpoint)

	return &Tracker{
		logger:              logger,
		slackWebhookURL:     slackWebhookURL,
		slackChannel:        slackChannel,
		firehoseEndpoint:    firehoseEndpoint,
		solanaRPCEndpoint:   solanaRPCEndpoint,
		// Initialize reusable clients
		firehoseConn:        conn,
		firehoseClient:      firehoseClient,
		rpcFetcher:          rpcFetcher,
		rpcClient:           rpcClient,
	}
}

// sendSlackNotification sends a notification to Slack when blocks differ
func (t *Tracker) sendSlackNotification(firehoseSlot uint64, firehoseSum, rpcSum, firehoseFilePath, rpcFetcherFilePath string) error {
	if t.slackWebhookURL == "" {
		t.logger.Info("SLACK_WEBHOOK_URL not set, skipping Slack notification")
		return nil
	}

	channel := t.slackChannel
	if channel == "" {
		channel = "#general" // default channel
	}

	message := fmt.Sprintf("🚨 *Solana Block QA Alert* 🚨\n"+
		"Block differences detected at slot %d\n"+
		"• Firehose checksum: `%s`\n"+
		"• RPC Fetcher checksum: `%s`\n"+
		"• Firehose JSON file: `%s`\n"+
		"• RPC Fetcher JSON file: `%s`\n"+
		"• Time: %s",
		firehoseSlot, firehoseSum, rpcSum, firehoseFilePath, rpcFetcherFilePath, time.Now().Format("2006-01-02 15:04:05"))

	payload := slack.WebhookMessage{
		Channel:   channel,
		Username:  "Solana Block QA Tracker",
		IconEmoji: ":warning:",
		Text:      message,
	}

	err := slack.PostWebhook(t.slackWebhookURL, &payload)
	if err != nil {
		return fmt.Errorf("failed to send Slack notification: %w", err)
	}

	t.logger.Info("Slack notification sent", zap.String("channel", channel))
	return nil
}

// ApiKeyAuth implements per-RPC credentials using API key
type ApiKeyAuth struct {
	ApiKey string
}

func (a *ApiKeyAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}
	out := make(map[string]string)
	for k, v := range md {
		if len(v) != 0 {
			out[k] = v[0]
		}
	}
	if a.ApiKey != "" {
		out["x-api-key"] = a.ApiKey
	}
	return out, nil
}

func (a *ApiKeyAuth) RequireTransportSecurity() bool {
	return true
}

// calculateChecksum calculates SHA256 checksum of the given data
func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// sanitizeBlock removes logMessages from all transactions in the block (modifies original)
func sanitizeBlock(block *pbsol.Block) {
	// Remove logMessages from each transaction directly in the original block
	for i := range block.Transactions {
		if block.Transactions[i].Meta != nil {
			block.Transactions[i].Meta.LogMessages = nil
		}
	}
}

// calculateSanitizedChecksum calculates checksum of a block after removing logMessages
func calculateSanitizedChecksum(block *pbsol.Block) (string, error) {
	// Sanitize the block by removing logMessages (modifies the original block)
	sanitizeBlock(block)

	// Marshal the sanitized block to bytes
	sanitizedData, err := proto.Marshal(block)
	if err != nil {
		return "", fmt.Errorf("failed to marshal sanitized block: %w", err)
	}

	// Calculate checksum of sanitized data
	return calculateChecksum(sanitizedData), nil
}

// fetchLatestBlock fetches and unmarshals the latest Solana block from StreamingFast Firehose
func (t *Tracker) fetchLatestBlock(ctx context.Context) (*pbsol.Block, string, error) {
	// Get authentication credentials from environment variables
	jwt := os.Getenv("FIREHOSE_API_TOKEN")
	apiKey := os.Getenv("FIREHOSE_API_KEY")

	// Setup call options for authentication and compression
	var callOpts []grpc.CallOption
	if jwt != "" {
		credentials := oauth.NewOauthAccess(&oauth2.Token{AccessToken: jwt, TokenType: "Bearer"})
		callOpts = append(callOpts, grpc.PerRPCCredentials(credentials))
	} else if apiKey != "" {
		callOpts = append(callOpts, grpc.PerRPCCredentials(&ApiKeyAuth{ApiKey: apiKey}))
	}

	// Add compression support (zstd is preferred by firehose servers)
	callOpts = append(callOpts, grpc.UseCompressor(zstd.Name))

	// Create a request to get the latest blocks (following official pattern)
	req := &pbfirehose.Request{
		StartBlockNum:   -1,    // Start from head (latest block)
		StopBlockNum:    0,     // Stream indefinitely
		FinalBlocksOnly: false, // Include all blocks
	}

	// Create stream with call options using reusable client
	stream, err := t.firehoseClient.Blocks(ctx, req, callOpts...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create stream: %v", err)
	}

	// Get the first (latest) block
	resp, err := stream.Recv()
	if err != nil {
		return nil, "", fmt.Errorf("failed to receive block: %v", err)
	}

	// Extract basic block information
	block := resp.Block
	if block == nil {
		return nil, "", fmt.Errorf("received empty block")
	}

	// Unmarshall the block data into Solana Block structure first
	var solanaBlock pbsol.Block
	err = proto.Unmarshal(block.Value, &solanaBlock)
	if err != nil {
		return nil, "", fmt.Errorf("failed to unmarshall Solana block: %v", err)
	}

	// Calculate sanitized checksum (without logMessages)
	checksum, err := calculateSanitizedChecksum(&solanaBlock)
	if err != nil {
		return nil, "", fmt.Errorf("failed to calculate sanitized checksum: %v", err)
	}
	t.logger.Info("Firehose block sanitized checksum calculated", zap.String("checksum_sha256", checksum))

	return &solanaBlock, checksum, nil
}

// fetchBlockWithRPCFetcher fetches the same block using the block fetcher from firehose-solana
func (t *Tracker) fetchBlockWithRPCFetcher(ctx context.Context, slot uint64) (*pbsol.Block, string, error) {

	// Use reusable RPCFetcher and RPC client instances
	// Fetch the block using reusable RPCFetcher and RPC client
	block, skipped, err := t.rpcFetcher.Fetch(ctx, t.rpcClient, slot)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch block with RPCFetcher: %w", err)
	}

	if skipped {
		return nil, "", fmt.Errorf("block %d was skipped", slot)
	}

	// Extract the pbsol.Block from the pbbstream.Block payload
	if block.Payload == nil {
		return nil, "", fmt.Errorf("block payload is nil")
	}

	// Unmarshal the block data into Solana Block structure first
	var solanaBlock pbsol.Block
	err = proto.Unmarshal(block.Payload.Value, &solanaBlock)
	if err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal Solana block: %w", err)
	}

	// Calculate sanitized checksum (without logMessages)
	checksum, err := calculateSanitizedChecksum(&solanaBlock)
	if err != nil {
		return nil, "", fmt.Errorf("failed to calculate sanitized checksum: %w", err)
	}
	t.logger.Info("RPCFetcher block sanitized checksum calculated", zap.String("checksum_sha256", checksum))

	return &solanaBlock, checksum, nil
}

// writeBlocksToJSONFiles writes both pbsol.Block objects to separate JSON files
func writeBlocksToJSONFiles(block1, block2 *pbsol.Block, filename1, filename2 string) error {
	// Convert blocks to JSON using protojson for better formatting
	marshaler := protojson.MarshalOptions{
		Indent:          "  ",
		EmitUnpopulated: false,
	}

	// Marshal first block
	json1, err := marshaler.Marshal(block1)
	if err != nil {
		return fmt.Errorf("failed to marshal first block to JSON: %w", err)
	}

	// Marshal second block
	json2, err := marshaler.Marshal(block2)
	if err != nil {
		return fmt.Errorf("failed to marshal second block to JSON: %w", err)
	}

	// Write first block to file
	err = os.WriteFile(filename1, json1, 0644)
	if err != nil {
		return fmt.Errorf("failed to write first block to file %s: %w", filename1, err)
	}

	// Write second block to file
	err = os.WriteFile(filename2, json2, 0644)
	if err != nil {
		return fmt.Errorf("failed to write second block to file %s: %w", filename2, err)
	}

	return nil
}

func (t *Tracker) compareBlocks(ctx context.Context) error {
	// Fetch the latest block from Firehose
	t.logger.Info("Fetching latest block from StreamingFast Firehose")
	firehoseBlock, firehoseBlockSum, err := t.fetchLatestBlock(ctx)
	if err != nil {
		return fmt.Errorf("error fetching block from Firehose: %w", err)
	}

	t.logger.Info("Successfully fetched Firehose block", zap.Uint64("slot", firehoseBlock.Slot))

	// Now fetch the same block using the block fetcher from firehose-solana
	t.logger.Info("Fetching block using RPCFetcher", zap.Uint64("slot", firehoseBlock.Slot))
	rpcFetcherBlock, rpcFetcherBlockSum, err := t.fetchBlockWithRPCFetcher(ctx, firehoseBlock.Slot)
	if err != nil {
		return fmt.Errorf("error fetching block with RPCFetcher: %w", err)
	}

	t.logger.Info("Successfully fetched block using RPCFetcher",
		zap.Uint64("slot", rpcFetcherBlock.Slot),
		zap.String("block_hash", rpcFetcherBlock.Blockhash))

	// Compare checksums and only write to JSON files if they are not equal
	t.logger.Info("Comparing checksums",
		zap.String("firehose_checksum", firehoseBlockSum),
		zap.String("rpc_fetcher_checksum", rpcFetcherBlockSum))

	if rpcFetcherBlockSum != firehoseBlockSum {
		t.logger.Warn("Checksums are different - writing blocks to JSON files",
			zap.Uint64("slot", firehoseBlock.Slot))
		firehoseFilename := fmt.Sprintf("firehose_block_%d.json", firehoseBlock.Slot)
		rpcFetcherFilename := fmt.Sprintf("rpc_fetcher_block_%d.json", rpcFetcherBlock.Slot)

		err = writeBlocksToJSONFiles(firehoseBlock, rpcFetcherBlock, firehoseFilename, rpcFetcherFilename)
		if err != nil {
			return fmt.Errorf("error writing blocks to JSON files: %w", err)
		}

		t.logger.Info("Block JSON files written",
			zap.String("firehose_file", firehoseFilename),
			zap.String("rpc_fetcher_file", rpcFetcherFilename))

		// Send Slack notification about the difference
		if err := t.sendSlackNotification(firehoseBlock.Slot, firehoseBlockSum, rpcFetcherBlockSum, firehoseFilename, rpcFetcherFilename); err != nil {
			t.logger.Error("Failed to send Slack notification", zap.Error(err))
		}
	} else {
		t.logger.Info("Checksums are equal - skipping JSON file output")
	}

	return nil
}

func (t *Tracker) runTracker(interval time.Duration) error {
	ctx := context.Background()

	t.logger.Info("Starting Solana Block QA Tracker", zap.Duration("interval", interval))
	t.logger.Info("Press Ctrl+C to stop the tracker")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create a ticker for periodic execution
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run the first comparison immediately
	t.logger.Info("Running initial block comparison")
	if err := t.compareBlocks(ctx); err != nil {
		t.logger.Error("Error in initial block comparison", zap.Error(err))
	}

	// Main loop
	for {
		select {
		case <-ticker.C:
			t.logger.Info("Running periodic block comparison")
			if err := t.compareBlocks(ctx); err != nil {
				t.logger.Error("Error in periodic block comparison", zap.Error(err))
			}
		case sig := <-sigChan:
			t.logger.Info("Received shutdown signal, stopping gracefully", zap.String("signal", sig.String()))
			return nil
		}
	}
}

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
