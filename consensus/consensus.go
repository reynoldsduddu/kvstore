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

	cons := &Consensus{
		State:      serverState,
		prioMgr:    priorityManager,
		nodes:      nodes,
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}

	fmt.Println("Nodes in consensus:", nodes)

	// Start heartbeat monitor only if follower
	if !cons.State.IsLeader() {
		go cons.monitorHeartbeat()
	}

	return cons
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

	url := fmt.Sprintf("http://%s/api/approve", node)
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

// commitChange applies the agreed change and followers replicate.
func (c *Consensus) commitChange(opType, key, value string) {
	fmt.Printf("Consensus reached: %s %s = %s\n", opType, key, value)

	// Replicate to all followers
	for _, node := range c.nodes {
		if node == c.State.GetMyAddress() {
			continue // skip self
		}

		go func(target string) {
			payload := map[string]string{
				"opType": opType,
				"key":    key,
				"value":  value,
			}
			data, _ := json.Marshal(payload)
			_, err := c.httpClient.Post("http://"+target+"/api/replicate", "application/json", bytes.NewReader(data))
			if err != nil {
				fmt.Printf("❌ Failed to replicate to %s: %v\n", target, err)
			} else {
				fmt.Printf("✅ Replicated to %s\n", target)
			}
		}(node)
	}
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

func (c *Consensus) monitorHeartbeat() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !c.State.IsFollower() {
			fmt.Println("🛑 Not a follower anymore. Stopping heartbeat monitor.")
			return
		}

		leader := c.State.GetLeader()
		if leader == "" {
			// Try asking other nodes who the current leader is
			for _, node := range c.nodes {
				if node == c.State.GetMyAddress() {
					continue
				}
				resp, err := c.httpClient.Get("http://" + node + "/api/leader")
				if err == nil && resp.StatusCode == http.StatusOK {
					var payload map[string]string
					if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
						leader = payload["leader"]
						c.State.SetLeader(leader)
						fmt.Printf("📡 Learned leader from %s: %s\n", node, leader)
						break
					}
				}
			}

			// If still unknown, skip this cycle
			if leader == "" {
				fmt.Println("🤷 Could not determine leader. Skipping this heartbeat check.")
				continue
			}
		}

		fmt.Printf("⏱️ Checking heartbeat from leader %s...\n", leader)

		resp, err := c.httpClient.Get("http://" + leader + "/api/heartbeat")
		if err == nil && resp.StatusCode == http.StatusOK {
			c.State.UpdateHeartbeat()
			fmt.Println("✅ Heartbeat received from leader.")
			continue
		}

		fmt.Printf("❌ Heartbeat failed: %v\n", err)

		if c.State.IsHeartbeatStale(5 * time.Second) {
			fmt.Println("🚨 Leader is unresponsive! Starting election...")
			c.startElection()
			return
		}
	}
}

func (c *Consensus) startElection() {
	fmt.Println("🗳️ Starting election process...")

	myAddr := c.State.GetMyAddress()
	myWeight := c.GetNodeWeight(myAddr)
	highestWeight := myWeight
	isLeader := true

	// Collect other nodes' weights
	for _, node := range c.nodes {
		if node == myAddr {
			continue
		}

		resp, err := c.httpClient.Get("http://" + node + "/api/priority")
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		defer resp.Body.Close()

		var weight float64
		if err := json.NewDecoder(resp.Body).Decode(&weight); err != nil {
			continue
		}

		if weight > highestWeight {
			isLeader = false
			break
		}
	}

	// Before declaring leadership, check again if someone already won
	for _, node := range c.nodes {
		if node == myAddr {
			continue
		}

		resp, err := c.httpClient.Get("http://" + node + "/api/leader")
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var payload map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
				declaredLeader := payload["leader"]
				if declaredLeader != "" && declaredLeader != myAddr {
					// Check if the declared leader is actually alive
					hbResp, err := c.httpClient.Get("http://" + declaredLeader + "/api/heartbeat")
					if err == nil && hbResp.StatusCode == http.StatusOK {
						fmt.Printf("🤷 Election aborted. %s is already leader and alive.\n", declaredLeader)
						c.State.SetLeader(declaredLeader)
						return
					}

					fmt.Printf("❌ Declared leader %s is unreachable. Proceeding with election...\n", declaredLeader)
				}

			}
		}
	}

	if isLeader {
		fmt.Printf("👑 %s becomes the new leader!\n", myAddr)
		c.State.SetLeader(myAddr)
		go c.startHeartbeatBroadcast()

		// Inform others
		for _, node := range c.nodes {
			if node == myAddr {
				continue
			}
			go func(n string) {
				payload := map[string]string{"leader": myAddr}
				data, _ := json.Marshal(payload)
				_, err := c.httpClient.Post("http://"+n+"/api/set-leader", "application/json", bytes.NewReader(data))
				if err != nil {
					fmt.Printf("❌ Failed to inform %s about new leader: %v\n", n, err)
				}
			}(node)
		}
	} else {
		fmt.Println("🙅 This node did not win the election.")
	}
}

func (c *Consensus) startHeartbeatBroadcast() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !c.State.IsLeader() {
			fmt.Println("🛑 No longer leader. Stopping heartbeat broadcast.")
			return
		}
		for _, node := range c.nodes {
			if node == c.State.GetMyAddress() {
				continue
			}
			go func(n string) {
				_, err := c.httpClient.Get("http://" + n + "/api/heartbeat")
				if err != nil {
					fmt.Printf("❌ Failed to send heartbeat to %s: %v\n", n, err)
				}
			}(node)
		}
	}
}

func (c *Consensus) GetNodeWeight(addr string) float64 {
	for i, node := range c.nodes {
		if node == addr {
			return c.prioMgr.GetNodeWeight(serverID(i))
		}
	}
	return 0
}
