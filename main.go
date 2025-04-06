package main

import (
	"fmt"
	"kvstore/config"
	"kvstore/consensus"
	"kvstore/kvstore"
	"os"
	"strconv"
)

// Load cluster configuration
func loadClusterConfig() ([]config.NodeConfig, int) {
	nodes, err := config.LoadClusterConfig("/app/config/cluster.conf")
	if err != nil {
		fmt.Println("Error loading config:", err)
		os.Exit(1)
	}

	serverID, err := strconv.Atoi(os.Getenv("SERVER_ID"))
	if err != nil {
		fmt.Println("Invalid SERVER_ID, using default 0")
		serverID = 0
	}

	return nodes, serverID
}

func main() {
	cwd, _ := os.Getwd()
	peers := []string{"8081", "8082", "8083", "8084", "8085"} // or loaded from config
	consensus.InitCabinetWeights(peers)
	fmt.Println("🔍 Current working directory:", cwd)
	nodes, serverID := loadClusterConfig()
	myNode := nodes[serverID]

	// Extract list of node addresses from config
	var nodeAddresses []string
	for _, node := range nodes {
		nodeAddresses = append(nodeAddresses, node.IP+":"+node.Port)
	}
	// Initialize consensus
	consensusModule := consensus.NewConsensus(myNode.IP+":"+myNode.Port, nodeAddresses)
	if consensusModule.State.IsLeader() {
		go consensusModule.StartHeartbeatBroadcast() // ✅ manually start it at launch
	}
	// Initialize KV Store with consensus
	store, err := kvstore.NewKVStore("/data/kvstore.db", consensusModule)
	if err != nil {
		fmt.Println("Failed to initialize database:", err)
		return
	}
	defer store.Close()

	server := kvstore.NewServer(store)

	// Start HTTP server
	fmt.Printf("Starting node %d at %s:%s\n", myNode.ID, myNode.IP, myNode.Port)
	if err := server.Start(myNode.IP + ":" + myNode.Port); err != nil {
		fmt.Println("Error starting server:", err)
	}
}
