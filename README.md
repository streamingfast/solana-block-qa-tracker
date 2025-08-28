# Solana Block QA Tracker

This project implements functionality to fetch the same Solana block using both StreamingFast Firehose and RPC Fetcher, with periodic execution capabilities. It runs block comparisons at configurable time intervals to continuously monitor block consistency and sends Slack notifications when differences are detected.

## Overview

The implementation:
1. Runs periodic block comparisons at configurable intervals
2. Fetches the latest block from StreamingFast Firehose
3. Extracts the slot number from the Firehose block
4. Uses the slot number to fetch the same block via RPC Fetcher from firehose-solana package
5. Compares checksums and writes JSON files when differences are found
6. Sends Slack notifications when block differences are detected
7. Supports graceful shutdown with Ctrl+C

## Key Features

- **Periodic Execution**: Runs block comparisons at configurable intervals
- **Dual Block Fetching**: Retrieves blocks using both Firehose and RPC Fetcher methods
- **Block Comparison**: Compares sanitized checksums and writes JSON files when differences are found
- **Slack Integration**: Sends notifications to Slack when block differences are detected
- **Configurable Endpoints**: Supports custom Firehose and Solana RPC endpoints
- **Graceful Shutdown**: Supports clean shutdown with Ctrl+C
- **Authentication Support**: Supports both JWT tokens and API keys for Firehose access
- **Error Handling**: Comprehensive error handling for network and API issues
- **Reusable Connections**: Maintains persistent connections for better performance

## Configuration

### Command Line Usage

The tracker requires an interval as a positional argument:

```bash
./tracker 30s
```

**Supported time formats:**
- `30s` - 30 seconds
- `5m` - 5 minutes  
- `1h30m` - 1 hour 30 minutes
- `2h` - 2 hours

### Command Line Flags

- `--slack-webhook-url`: Slack webhook URL for notifications (optional)
- `--slack-channel`: Slack channel for notifications (default: "solana")
- `--firehose-endpoint`: StreamingFast Solana Firehose endpoint (default: "mainnet.sol.streamingfast.io:443")
- `--solana-rpc-endpoint`: Solana RPC endpoint (default: "https://api.mainnet-beta.solana.com")

### Example Usage

```bash
# Basic usage with 30 second intervals
./tracker 30s

# With Slack notifications
./tracker 1m --slack-webhook-url="https://hooks.slack.com/services/..." --slack-channel="alerts"

# With custom endpoints
./tracker 30s --firehose-endpoint="custom.endpoint:443" --solana-rpc-endpoint="https://custom.rpc.endpoint"
```

## Dependencies

- `github.com/gagliardetto/solana-go` - Solana RPC client
- `github.com/streamingfast/firehose-solana` - Firehose Solana types and RPC Fetcher
- `github.com/streamingfast/pbgo` - Firehose protocol buffers
- `github.com/slack-go/slack` - Slack webhook integration
- `github.com/spf13/cobra` - CLI framework
- `github.com/streamingfast/logging` - Structured logging

## Authentication

To use this application, you need authentication credentials for StreamingFast Firehose:

### Option 1: JWT Token
```bash
export FIREHOSE_API_TOKEN="your_jwt_token_here"
```

### Option 2: API Key
```bash
export FIREHOSE_API_KEY="your_api_key_here"
```

You can obtain these credentials from [StreamingFast](https://streamingfast.io/).

## Slack Integration

The tracker can send notifications to Slack when block differences are detected:

### Setup
1. Create a Slack webhook URL in your workspace
2. Use the `--slack-webhook-url` flag or set it via command line
3. Optionally specify a channel with `--slack-channel` (defaults to "solana")

### Notification Content
When differences are found, the Slack notification includes:
- Slot number where the difference occurred
- Checksums from both Firehose and RPC Fetcher
- File paths of the generated JSON comparison files
- Timestamp of the detection

## Usage

### Building the Application
```bash
go build -o tracker ./cmd/tracker
```

### Running the Tracker
```bash
# Basic usage
./tracker 30s

# With all options
./tracker 1m \
  --slack-webhook-url="https://hooks.slack.com/services/..." \
  --slack-channel="alerts" \
  --firehose-endpoint="mainnet.sol.streamingfast.io:443" \
  --solana-rpc-endpoint="https://api.mainnet-beta.solana.com"
```

## Output Files

When block differences are detected, the tracker generates:
- `firehose_block_<slot>.json` - Block data from Firehose
- `rpc_fetcher_block_<slot>.json` - Block data from RPC Fetcher

These files contain the full block data in JSON format for manual comparison and analysis.