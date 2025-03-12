package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// NodeConfig stores configuration for each node
type NodeConfig struct {
	ID   int
	IP   string
	Port string
}

// LoadClusterConfig reads the `cluster.conf` file
func LoadClusterConfig(filePath string) ([]NodeConfig, error) {
	var nodes []NodeConfig

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			continue // Ignore invalid lines
		}

		var node NodeConfig
		fmt.Sscanf(fields[0], "%d", &node.ID)
		node.IP = fields[1]
		node.Port = fields[2]

		nodes = append(nodes, node)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	return nodes, nil
}
