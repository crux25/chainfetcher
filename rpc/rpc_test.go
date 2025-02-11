package rpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRPCManager_GetBestRPC(t *testing.T) {
	// Initialize RPC manager with mock nodes
	urls := []string{"http://rpc1", "http://rpc2"}
	rm := New(urls)

	// Ensure the manager has nodes
	assert.Equal(t, len(urls), len(rm.nodes))

	// Test GetBestRPC selection
	bestRPC := rm.GetBestRPC()
	assert.NotNil(t, bestRPC)
	assert.Contains(t, urls, bestRPC.URL)
}

func TestRPCManager_ReportRPCLatency(t *testing.T) {
	// Initialize RPC manager with mock nodes
	urls := []string{"http://rpc1", "http://rpc2"}
	rm := New(urls)

	// Report latency for the first node
	rm.ReportRPCLatency(urls[0], 100*time.Millisecond, true)

	// Verify that the node's metrics were updated
	node := rm.nodes[0]
	assert.Greater(t, node.SuccessRate, 0.9)
	assert.Greater(t, node.ResponseTime, 0.0)
}
