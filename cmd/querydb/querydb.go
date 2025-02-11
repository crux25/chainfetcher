package main

import (
	"chainfetcher/db"
	"flag"
	"log"
)

func main() {
	// - `startindex`: The starting index of the log to retrieve. Defaults to 0.
	// - `endindex`: The ending index of the log to retrieve. Defaults to 10.
	// - `dbpath`: The path to the BadgerDB database file. This is a required flag.
	startIndex := flag.Uint64("startindex", 0, "The first index of the log to retrieve. Defaults to 0.")
	endIndex := flag.Uint64("endindex", 10, "The last index of the log to retrieve. Defaults to 10.")
	dbPath := flag.String("dbpath", "", "The path to the BadgerDB database file. This is a required flag.")

	// Parse the command-line flags provided by the user.
	flag.Parse()

	// Validate that the `dbpath` flag is provided. If not, log an error and exit.
	if *dbPath == "" {
		log.Fatal("The `dbpath` flag is required. Please provide the path to the BadgerDB file using `--dbpath`.")
	}

	// Initialize the BadgerDB database using the provided path.
	// If initialization fails, log the error and exit.
	db, err := db.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize the database: %v", err)
	}

	// Iterate over the range of log indices specified by `startindex` and `endindex`.
	// For each index, retrieve the corresponding log from the database.
	for i := *startIndex; i < *endIndex; i++ {
		// Retrieve the log entry from the database using the current index.
		storedLogs, err := db.GetLog(i)
		if err != nil {
			// If an error occurs while retrieving the log, log the error and exit.
			log.Fatalf("Error retrieving log at index %d: %v", i, err)
		}

		// Log the retrieved log entry to the console.
		log.Printf("Retrieved logs from BadgerDB at index %d: %v", i, storedLogs)
	}
}
