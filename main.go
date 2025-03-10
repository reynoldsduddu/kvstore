package main

import (
	"fmt"
	"kvstore/kvstore"
)

func main() {
	// Create a new key-value store with SQLite backend
	store, err := kvstore.NewKVStore("kvstore.db")
	if err != nil {
		fmt.Printf("Error creating key-value store: %v\n", err)
		return
	}
	defer store.Close()

	// Create a new HTTP server
	server := kvstore.NewServer(store)

	// Start the server on port 8080
	fmt.Println("Starting key-value store server on :8081")
	if err := server.Start(":8081"); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}
