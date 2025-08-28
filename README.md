# Solana Block QA Tracker

This project implements functionality to fetch the same Solana block using both StreamingFast Firehose and Solana RPC, with periodic execution capabilities. It runs block comparisons at configurable time intervals to continuously monitor block consistency.

## Overview

The implementation:
1. Runs periodic block comparisons at configurable intervals
2. Fetches the latest block from StreamingFast Firehose
3. Extracts the slot number from the Firehose block
4. Uses the slot number to fetch the same block via Solana RPC
5. Compares checksums and writes JSON files when differences are found
6. Supports graceful shutdown with Ctrl+C

## Key Features

- **Periodic Execution**: Runs block comparisons at configurable intervals
- **Dual Block Fetching**: Retrieves blocks using both Firehose and RPC methods
- **Block Comparison**: Compares checksums and writes JSON files when differences are found
- **Configurable Intervals**: Set comparison frequency via command line flags or environment variables
- **Graceful Shutdown**: Supports clean shutdown with Ctrl+C
- **Authentication Support**: Supports both JWT tokens and API keys for Firehose access
- **Error Handling**: Comprehensive error handling for network and API issues
- **Type Safety**: Proper handling of different data types between Firehose and RPC responses

## Configuration

### Comparison Interval

The time interval between block comparisons can be configured in two ways:

#### Command Line Flag
```bash
./solana-block-qa-tracker -interval 1m30s
```

#### Environment Variable
```bash
export COMPARISON_INTERVAL="2m"
./solana-block-qa-tracker
```

**Supported time formats:**
- `30s` - 30 seconds
- `5m` - 5 minutes  
- `1h30m` - 1 hour 30 minutes
- `2h` - 2 hours

**Default:** 30 seconds

**Priority:** Command line flag takes precedence over environment variable

## Dependencies

- `github.com/gagliardetto/solana-go` - Solana RPC client
- `github.com/streamingfast/firehose-solana` - Firehose Solana types and functionality
- `github.com/streamingfast/pbgo` - Firehose protocol buffers

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

## Usage

### Using the Test Script (Recommended)
```bash
./test-block-fetching.sh
```

The test script will:
- Check for authentication credentials
- Run the block fetching program
- Display results and comparisons

### Direct Execution
```bash
go run main.go
```

## Implementation Details

### fetchLatestBlock()
Connects to StreamingFast Firehose and retrieves the latest Solana block:
- Uses gRPC with TLS encryption
- Supports compression (zstd)
- Handles authentication via JWT or API key
- Unmarshals protobuf data into Solana block structure

### fetchBlockByRPC()
Fetches a specific block by slot number using Solana RPC:
- Uses the `gagliardetto/solana-go/rpc` client
- Configures block options similar to the firehose fetcher
- Uses confirmed commitment level
- Returns raw RPC block result for comparison

### Block Comparison
The main function compares:
- Block hashes (converted to consistent string format)
- Parent slot numbers
- Transaction counts
- Provides visual feedback on whether blocks match

## Technical Notes

- The implementation avoids direct use of the `firehose-solana/block/fetcher` package due to version compatibility issues with `gagliardetto/solana-go`
- Instead, it implements a simpler RPC approach using the RPC client directly
- Block hash comparison properly handles type conversion between `string` and `solana.Hash`
- The RPC endpoint uses Solana mainnet: `https://api.mainnet-beta.solana.com`

## Example Output

```
Fetching latest block from StreamingFast Firehose...
Successfully fetched Firehose block (Slot: 123456789)
Fetching the same block (Slot: 123456789) using RPC...
Successfully fetched RPC block (Slot: 123456789)

=== Block Comparison ===
Firehose Block Hash: ABC123...
RPC Block Hash:      ABC123...
Firehose Parent Slot: 123456788
RPC Parent Slot:      123456788
Firehose Transactions: 1234
RPC Transactions:      1234
✅ Block hashes match - both methods fetched the same block!
```

## Error Handling

The application handles various error conditions:
- Missing authentication credentials
- Network connectivity issues
- Invalid block slots
- API rate limiting
- Malformed responses

## Files

- `main.go` - Main application with Firehose and RPC fetching logic
- `test-block-fetching.sh` - Test script with authentication checks
- `go.mod` - Go module dependencies
- `README.md` - This documentation