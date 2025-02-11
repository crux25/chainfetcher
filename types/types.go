package types

type LogEntry struct {
	Index      uint64 `msgpack:"index"`        // Unique incrementing index
	BlockTime  uint64 `msgpack:"block_time"`   // Timestamp of block
	ParentHash string `msgpack:"parent_hash"`  // Parent hash of block
	L1InfoRoot string `msgpack:"l1_info_root"` // L1 Info Root data
}
