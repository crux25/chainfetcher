package processor

import (
	"chainfetcher/db"
	"chainfetcher/rpc"
	"chainfetcher/types"
	"context"
	"log"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
)

type LogProcessor struct {
	db         *db.DB
	rpcManager *rpc.Manager
	queue      chan []ethTypes.Log
	wg         sync.WaitGroup
}

// NewLogProcessor initializes log processing
func New(db *db.DB, rpcManager *rpc.Manager) *LogProcessor {
	lp := &LogProcessor{
		db:         db,
		rpcManager: rpcManager,
		queue:      make(chan []ethTypes.Log, 1000),
	}
	lp.wg.Add(1)
	go lp.worker()
	return lp
}

// worker processes logs asynchronously by fetching block data and storing log entries
func (lp *LogProcessor) worker() {
	defer lp.wg.Done()

	for logs := range lp.queue {
		for _, logEntry := range logs {
			var block *ethTypes.Block
			var err error
			var bestRPC *rpc.RPCNode
			var latency time.Duration
			maxRetries := 3 // Maximum retry attempts for fetching block data

			// Attempt to fetch the block with retries
			for attempt := 0; attempt < maxRetries; attempt++ {
				bestRPC = lp.rpcManager.GetBestRPC()
				startTime := time.Now()

				// Fetch block data using the best available RPC
				block, err = bestRPC.Client.BlockByHash(context.Background(), logEntry.BlockHash)
				latency = time.Since(startTime)

				if err == nil {
					// Successfully retrieved the block, report successful RPC latency
					lp.rpcManager.ReportRPCLatency(bestRPC.URL, latency, true)
					break
				}

				// Log the failure and report unsuccessful latency
				log.Printf("Attempt %d: Failed to fetch block data for %s from %s: %v",
					attempt+1, logEntry.BlockHash.Hex(), bestRPC.URL, err)

				lp.rpcManager.ReportRPCLatency(bestRPC.URL, latency, false)

				// Exponential backoff before retrying
				time.Sleep(time.Duration((attempt+1)*500) * time.Millisecond)
			}

			// If block data retrieval failed after all retries, skip processing this log entry
			if err != nil {
				log.Printf("Max retries reached. Skipping log entry for block: %s", logEntry.BlockHash.Hex())
				continue
			}

			// Get next available log index from the database
			nextIndex, err := lp.db.GetNextIndex()
			if err != nil {
				log.Printf("Failed to retrieve next log index: %v", err)
				continue
			}

			// Construct log data to store in the database
			logData := types.LogEntry{
				Index:      nextIndex,
				BlockTime:  block.Time(),
				ParentHash: block.ParentHash().Hex(),
				L1InfoRoot: common.BytesToHash(logEntry.Data).Hex(),
			}

			// Store the processed log entry in the database
			err = lp.db.StoreLog(logData)
			if err != nil {
				log.Printf("Failed to store log entry in database: %v", err)
			}
		}
	}
}

// AddLog sends a log entry for processing
func (lp *LogProcessor) AddLog(logEntry []ethTypes.Log) {
	lp.queue <- logEntry
}

// Stop processing
func (lp *LogProcessor) Stop() {
	close(lp.queue)
	lp.wg.Wait()
}
