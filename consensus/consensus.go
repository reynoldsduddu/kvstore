package consensus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Consensus manages distributed agreement between nodes.
type Consensus struct {
	mu         sync.Mutex
	State      *ServerState
	prioMgr    *PriorityManager
	nodes      []string
	httpClient *http.Client
}

// NewConsensus initializes consensus with PriorityManager.
func NewConsensus(myAddress string, nodes []string) *Consensus {
	serverState := NewServerState(myAddress)
	priorityManager := &PriorityManager{}
	priorityManager.Init(len(nodes), (len(nodes)/2)+1, 1, 0.01, true)
	fmt.Println("Nodes in consensus:", nodes)
	return &Consensus{
		State:      serverState,
		prioMgr:    priorityManager,
		nodes:      nodes,
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}
}

// ProposeChange starts a consensus operation.
func (c *Consensus) ProposeChange(opType, key, value string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Printf("🔍 Checking consensus for %s: key=%s, value=%s\n", opType, key, value)

	// Ensure the node is the leader
	if !c.State.IsLeader() {
		fmt.Println("Rejecting proposal: This node is NOT the leader.")
		return false
	}

	majority := c.prioMgr.GetMajority()
	approvalWeight := c.prioMgr.GetLeaderWeight()

	fmt.Printf("Majority required: %f, Current leader weight: %f\n", majority, approvalWeight)

	for _, node := range c.nodes {
		if node == c.State.GetMyAddress() {
			continue // Skip self
		}
		fmt.Printf("🔎 Checking if approval is needed from: %s\n", node)

		// Identify node index based on IP
		nodeIndex := -1
		for i, addr := range c.nodes {
			if addr == node {
				nodeIndex = i
				break
			}
		}

		if nodeIndex == -1 {
			fmt.Printf("Node %s not found in node list\n", node)
			continue
		}

		approved := c.requestApproval(node, opType, key, value)
		fmt.Printf("🔹 Approval from %s (index %d): %v\n", node, nodeIndex, approved)

		if approved {
			approvalWeight += c.prioMgr.GetNodeWeight(serverID(nodeIndex))
		}

		if approvalWeight > majority {
			fmt.Println("Consensus REACHED. Committing change.")
			c.commitChange(opType, key, value)
			return true
		}
	}

	fmt.Println("Consensus NOT REACHED. Rejecting request.")
	return false
}

// requestApproval asks followers for approval.
func (c *Consensus) requestApproval(node, opType, key, value string) bool {
	reqBody, _ := json.Marshal(map[string]string{
		"opType": opType,
		"key":    key,
		"value":  value,
	})

	url := fmt.Sprintf("http://%s/approve", node)
	fmt.Printf("🔹 Sending approval request to %s for key=%s\n", url, key)

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		fmt.Printf("Approval request to %s failed: %v\n", url, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Approval request to %s rejected with status %d\n", url, resp.StatusCode)
		return false
	}

	fmt.Printf("Approval granted by %s for key=%s\n", node, key)
	return true
}

// commitChange applies the agreed change.
func (c *Consensus) commitChange(opType, key, value string) {
	fmt.Printf("Consensus reached: %s %s = %s\n", opType, key, value)
}

// HandleApproval allows followers to approve leader proposals.
func (c *Consensus) HandleApproval(w http.ResponseWriter, r *http.Request) {
	if !c.State.IsFollower() {
		http.Error(w, "Only followers can approve", http.StatusForbidden)
		fmt.Println("Leader cannot approve its own requests.")
		return
	}

	var req map[string]string
	json.NewDecoder(r.Body).Decode(&req)
	opType := req["opType"]
	key := req["key"]
	value := req["value"]

	fmt.Printf("Approval granted: %s %s = %s\n", opType, key, value)
	w.WriteHeader(http.StatusOK)
}
