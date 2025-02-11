package rpc

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
)

// RPCNode represents an Ethereum RPC node with performance metrics.
type RPCNode struct {
	URL          string            // Node URL
	Client       *ethclient.Client // RPC Client for making requests
	ResponseTime float64           // Average response time in milliseconds
	SuccessRate  float64           // Ratio of successful requests
	CurrentLoad  int               // Number of active requests
	LastFailure  time.Time         // Timestamp of the last failure
}

// Manager manages multiple RPC nodes and balances the load across the nodes.
type Manager struct {
	nodes []*RPCNode // List of available RPC nodes
	mu    sync.Mutex // Mutex for synchronizing access
}

// New initializes an RPC Manager with a list of RPC URLs.
func New(urls []string) *Manager {
	nodes := make([]*RPCNode, len(urls))
	for i, url := range urls {
		client, err := ethclient.Dial(url)
		if err != nil {
			log.Printf("Failed to connect to RPC %s: %v", url, err)
			continue
		}
		// Initialize with default performance metrics
		nodes[i] = &RPCNode{
			URL:          url,
			Client:       client,
			ResponseTime: 1.0, // Default response time
			SuccessRate:  1.0, // Assume perfect reliability initially
			CurrentLoad:  0,
		}
	}
	return &Manager{nodes: nodes}
}

// GetBestRPC selects the best RPC node based on performance metrics.
// Uses a weighted random selection strategy to balance load dynamically.
func (rm *Manager) GetBestRPC() *RPCNode {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	totalWeight := 0.0
	weights := make([]float64, len(rm.nodes))

	for i, ep := range rm.nodes {
		// If the node has recently failed, temporarily exclude it
		if time.Since(ep.LastFailure) < time.Minute {
			weights[i] = 0
			continue
		}

		// Compute weight based on response time, success rate, and current load
		weight := (1.0 / ep.ResponseTime) * ep.SuccessRate / float64(ep.CurrentLoad+1)
		weights[i] = weight
		totalWeight += weight
	}

	// Select an node using weighted random selection
	randomPoint := rand.Float64() * totalWeight
	sum := 0.0
	for i, weight := range weights {
		sum += weight
		if sum >= randomPoint {
			rm.nodes[i].CurrentLoad++ // Increase load count on selected node
			return rm.nodes[i]
		}
	}
	return nil // In case all nodes fail
}

// ReportRPCLatency updates node's performance metrics after a request.
// Adjusts response time, success rate, and resets failure timestamps.
func (rm *Manager) ReportRPCLatency(url string, duration time.Duration, success bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, ep := range rm.nodes {
		if ep.URL == url {
			// Smooth out response time changes using exponential moving average
			ep.ResponseTime = 0.9*ep.ResponseTime + 0.1*float64(duration.Milliseconds())

			if success {
				ep.SuccessRate = 0.9*ep.SuccessRate + 0.1 // Gradually improve success rate
			} else {
				ep.SuccessRate *= 0.9       // Reduce success rate on failure
				ep.LastFailure = time.Now() // Mark last failure timestamp
			}
			ep.CurrentLoad-- // Decrease active load after request completes
			break
		}
	}
}
