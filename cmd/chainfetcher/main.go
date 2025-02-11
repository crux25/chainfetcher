package main

import (
	"chainfetcher/db"
	"chainfetcher/processor"
	"chainfetcher/rpc"
	"context"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/joho/godotenv"
)

const defaultBlockBatchSize int64 = 5000
const defaultMaxFetchWorkers int64 = 5
const defaultRpcFetchCoolDown float64 = 1

// Load environment variables
func loadEnv() {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatal("Error loading .env file")
	}
}

func main() {
	// Load environment variables from the .env file
	loadEnv()

	// Retrieve the log file path from environment variables.
	logFile := os.Getenv("LOG_FILE")
	if logFile == "" {
		log.Println("Warning: No log file provided in .env. Logs will only be printed to stdout.")
	}

	// If a log file is specified, configure logging to write to both stdout and the log file.
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("Error opening log file: %v", err) // Fatal stops execution if log file cannot be opened.
		}
		defer f.Close() // Ensure the file is closed when the function exits.

		// Set up multi-writer to output logs to both the terminal (stdout) and the log file.
		wrt := io.MultiWriter(os.Stdout, f)
		log.SetOutput(wrt)
	}

	// Retrieve the database path from environment variables.
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		log.Fatal("Error: No DB_PATH provided in .env. Database initialization cannot proceed.")
	}

	// Initialize the BadgerDB database using the provided path.
	db, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatal("Failed to initialize database:", err) // Fatal logs error and exits the program.
	}

	// Retrieve the RPC URLs (comma-separated) from environment variables.
	rpcURLs := os.Getenv("RPC_URLS")
	if rpcURLs == "" {
		log.Fatal("Error: No RPC URLs provided in .env. At least one RPC URL is required to interact with the blockchain.")
	}

	// Split the comma-separated list of RPC URLs into a slice.
	rpcList := splitCSV(rpcURLs)

	// Create an RPC manager instance to handle blockchain interactions.
	rpcManager := rpc.New(rpcList)

	// Retrieve and parse the smart contract address from environment variables.
	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))

	// Retrieve and parse the event topic hash from environment variables.
	topic := common.HexToHash(os.Getenv("TOPIC"))

	// Initialize the log processor, which will handle fetched logs.
	logProcessor := processor.New(db, rpcManager)

	// Fetch logs from the blockchain, filter them using the specified contract address and topic,
	// and process them using the log processor.
	fetchLogs(rpcManager, contractAddress, topic, logProcessor)
}

// fetchLogs retrieves logs from a smart contract in batch requests and processes them asynchronously.
// It determines the starting block dynamically based on the environment variable "FROM_BLOCK" or the contract's deployment block.
// It also implements retry logic, ensuring failed batch requests are retried up to 3 times before being skipped.
func fetchLogs(rpcManager *rpc.Manager, contractAddress common.Address, topic common.Hash, logProcessor *processor.LogProcessor) {
	// Retrieve starting block from environment variables or find contract creation block if set to a negative value
	envFrmBlock, _ := strconv.ParseInt(os.Getenv("FROM_BLOCK"), 10, 64)
	var fromBlock uint64 = 0

	if envFrmBlock < 0 {
		var err error
		fromBlock, err = findContractCreationBlock(rpcManager, contractAddress)
		if err != nil {
			log.Printf("Failed to determine contract deployment block: %v \n", err)
		}
	} else {
		fromBlock = uint64(envFrmBlock)
	}

	// Determine batch size for fetching logs, using environment variable or default value
	blockBatchSize, _ := strconv.ParseInt(os.Getenv("BLOCK_BATCH_SIZE"), 10, 64)
	if blockBatchSize <= 0 {
		blockBatchSize = defaultBlockBatchSize
	}

	// Fetch the latest block number from the best available RPC endpoint
	bestRPC := rpcManager.GetBestRPC()
	latestBlock, err := bestRPC.Client.BlockNumber(context.Background())
	if err != nil {
		log.Fatal("Failed to get latest block:", err)
	}

	log.Printf("Fetching logs from %d to %d in batches of %d", fromBlock, latestBlock, blockBatchSize)

	// Define worker-related parameters for concurrent batch processing
	var wg sync.WaitGroup
	blockCh := make(chan uint64, 10) // Channel for distributing work
	retryCh := make(chan uint64, 10) // Channel for handling failed fetch attempts
	maxRetries := 3                  // Maximum retries for failed batch requests

	// Determine the number of parallel workers from environment variable or use default value
	maxWorkers, _ := strconv.ParseInt(os.Getenv("MAX_FETCH_WORKERS"), 10, 64)
	if maxWorkers <= 0 {
		maxWorkers = defaultMaxFetchWorkers
	}

	// Set a cooldown period between fetch attempts to avoid hitting RPC rate limits
	rpcFetchCoolDown, _ := strconv.ParseFloat(os.Getenv("RPC_FETCH_COOLDOWN"), 64)
	if rpcFetchCoolDown < 0 {
		rpcFetchCoolDown = defaultRpcFetchCoolDown
	}

	// Spawn worker goroutines to process block ranges concurrently
	for i := 0; i < int(maxWorkers); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for from := range blockCh {
				to := min(from+uint64(blockBatchSize)-1, latestBlock)

				// Attempt to fetch logs, and if unsuccessful, add to retry queue
				if !fetchAndProcessLogs(rpcManager, contractAddress, topic, logProcessor, from, to) {
					retryCh <- from
				}

				// Apply a cooldown period to prevent overwhelming the RPC endpoint
				time.Sleep(time.Duration(rpcFetchCoolDown) * time.Second)
			}
		}()
	}

	// Distribute block ranges to workers
	for from := fromBlock; from <= latestBlock; from += uint64(blockBatchSize) {
		blockCh <- from
	}
	close(blockCh)

	// Wait for initial batch processing to complete
	wg.Wait()

	// Retry logic: Process failed batch requests up to maxRetries times
	for retryAttempt := 1; retryAttempt <= maxRetries; retryAttempt++ {
		if len(retryCh) == 0 {
			break // Exit if there are no failed batches left
		}

		log.Printf("Retry attempt %d for failed batch requests", retryAttempt)

		// Reset worker group for retrying failed batches
		wg.Add(1)
		go func() {
			defer wg.Done()
			for from := range retryCh {
				to := min(from+uint64(blockBatchSize)-1, latestBlock)

				// Retry fetching logs for the failed range
				if !fetchAndProcessLogs(rpcManager, contractAddress, topic, logProcessor, from, to) {
					// If it still fails after maxRetries, log and ignore the batch
					if retryAttempt == maxRetries {
						log.Printf("Batch from %d to %d failed after %d retries. Skipping.", from, to, maxRetries)
					} else {
						retryCh <- from
					}
				}

				// Apply cooldown period to prevent excessive load on the RPC
				time.Sleep(time.Duration(rpcFetchCoolDown) * time.Second)
			}
		}()
		wg.Wait()
	}

	// Close retry channel once all retries are exhausted
	close(retryCh)
}

// fetchAndProcessLogs fetches logs from the blockchain within the specified block range
// using the best available RPC node and processes the retrieved logs.
// It also reports the RPC node's latency and success/failure status to the RPC manager.
func fetchAndProcessLogs(rpcManager *rpc.Manager, contractAddress common.Address, topic common.Hash, logProcessor *processor.LogProcessor, from, to uint64) bool {
	// Retrieve the best available RPC node from the manager.
	bestRPC := rpcManager.GetBestRPC()

	// Construct an Ethereum filter query to fetch logs within the given block range
	// for the specified contract address and topic.
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(from)),           // Start block for the query
		ToBlock:   big.NewInt(int64(to)),             // End block for the query
		Addresses: []common.Address{contractAddress}, // Filter by contract address
		Topics:    [][]common.Hash{{topic}},          // Filter by topic (event signature)
	}

	// Start timing the RPC request to measure latency.
	startTime := time.Now()

	// Fetch logs from the Ethereum blockchain using the best RPC endpoint.
	logs, err := bestRPC.Client.FilterLogs(context.Background(), query)

	// Measure the latency of the RPC request.
	latency := time.Since(startTime)

	log.Printf("Fetched %d logs from block %d to %d", len(logs), from, to)

	// Handle RPC failures.
	if err != nil {
		log.Printf("RPC %s failed to fetch logs from block %d to %d: %v", bestRPC.URL, from, to, err)

		// Report the failed RPC attempt with latency data to the RPC manager.
		rpcManager.ReportRPCLatency(bestRPC.URL, latency, false)

		// Return false indicating that the fetch operation was unsuccessful.
		return false
	}

	// Report a successful RPC response to the RPC manager.
	rpcManager.ReportRPCLatency(bestRPC.URL, latency, true)

	// Process the fetched logs by adding them to the log processor.
	logProcessor.AddLog(logs)

	// Return true indicating a successful log fetch and processing.
	return true
}

// SplitCSV splits comma-separated values, and return an array of the values.
func splitCSV(input string) []string {
	var result []string
	for _, item := range strings.Split(input, ",") {
		result = append(result, strings.TrimSpace(item))
	}
	return result
}

// findContractCreationBlock uses binary search to find the block where the contract was created.
func findContractCreationBlock(rpcManager *rpc.Manager, contractAddress common.Address) (uint64, error) {
	log.Println("Searching for contract creation block...")
	// get the best RPC.
	bestRPC := rpcManager.GetBestRPC()

	// Fetch latest block to set the upper limit
	latestBlock, err := bestRPC.Client.BlockNumber(context.Background())
	if err != nil {
		log.Fatalf("Failed to get latest block: %v", err)
	}

	low := uint64(0)
	high := latestBlock
	var creationBlock uint64 = 0

	for low <= high {
		mid := (high + low) / 2
		block, err := bestRPC.Client.BlockByNumber(context.Background(), big.NewInt(int64(mid)))
		if err != nil {
			return 0, fmt.Errorf("failed to fetch block %d: %v", mid, err)
		}

		// Check if any transaction in the block created the contract
		found := false
		for _, tx := range block.Transactions() {
			if tx.To() == nil {
				receipt, err := bestRPC.Client.TransactionReceipt(context.Background(), tx.Hash())
				if err != nil {
					return 0, fmt.Errorf("failed to fetch receipt for tx %s: %v", tx.Hash().Hex(), err)
				}
				if receipt.ContractAddress.Hex() == contractAddress.Hex() {
					creationBlock = mid
					found = true
					log.Printf("Found contract creation block: %v  \n", block.Number())
					return creationBlock, nil
				}
			}
		}

		if !found {
			if high == low {
				log.Printf("Found contract creation block: %v  \n", high)
				return high, nil
			}
			// Check if the contract exists in a later block
			code, err := bestRPC.Client.CodeAt(context.Background(), common.HexToAddress(contractAddress.Hex()), big.NewInt(int64(mid)))
			if err != nil {
				return 0, fmt.Errorf("failed to fetch contract code: %v", err)
			}
			if len(code) > 2 {
				high = mid - 1
			} else {
				low = mid + 1
			}
		}
	}
	return creationBlock, nil
}

// Helper function to find the minimum of two numbers
func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
