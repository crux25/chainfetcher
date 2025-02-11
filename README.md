# ChainFetcher

ChainFetcher is a Go-based application for fetching and processing blockchain logs from Ethereum-compatible chains. It uses BadgerDB for storage and supports multiple RPC endpoints for load balancing and redundancy.

## Table of Contents

- [Main Components](#main-components)
- [Installation](#installation)
- [Usage](#usage)
- [Testing](#testing)

## Main Components

The project is organized into the following main components:

- **\`cmd/chainfetcher/main.go\`**: The main entry point for the log-fetching program. It initializes the application, loads environment variables, and starts fetching logs from the blockchain.

- **\`cmd/querydb/querydb.go\`**: A utility to query logs stored in the BadgerDB database. It allows retrieving logs by index range.

- **\`db/db.go\`**: Manages interactions with the BadgerDB database. It provides functions for storing and retrieving logs.

- **\`types/types.go\`**: Contains type definitions used across the application.

- **\`processor/processor.go\`**: Handles the processing of fetched logs. It fetches additional block data and stores processed logs in the database.

- **\`rpc/rpc.go\`**: Manages interactions with Ethereum RPC nodes. It includes logic for load balancing and retries across multiple RPC endpoints.

## Installation

1. **Clone the Repository**:
   \`\`\`bash
   cd chainfetcher
   \`\`\`

2. **Install Dependencies**:
   Ensure you have Go installed, then run:
   \`\`\`bash
   go mod tidy
   \`\`\`

3. **Set Up Environment Variables**:
   Configure \`.env\` file in the root directory with the following variables:
   \`\`\`env
   DB_PATH=/path/to/your/database
   RPC_URLS=http://rpc1.url,http://rpc2.url
   CONTRACT_ADDRESS=0xYourContractAddress
   TOPIC=0xYourEventTopic
   FROM_BLOCK=TheToStartSyncFrom
   LOG_FILE=/path/to/your/logfile.log
   \`\`\`

## Usage

### Fetch Logs from the Blockchain
To fetch logs from the blockchain, run:
\`\`\`bash
go run cmd/chainfetcher/main.go
\`\`\`

### Query Logs from the Database
To query logs stored in the BadgerDB database, use:
\`\`\`bash
go run cmd/querydb/querydb.go --dbpath=/path/to/db --startindex=0 --endindex=10
\`\`\`

### Command-Line Flags for \`querydb.go\`
- \`--dbpath\`: Path to the BadgerDB database file (required).
- \`--startindex\`: Starting index of the logs to retrieve (default: 0).
- \`--endindex\`: Ending index of the logs to retrieve (default: 10).

## Testing

To run tests, use:
\`\`\`bash
go test -v ./...
\`\`\`

