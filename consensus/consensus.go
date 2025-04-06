package consensus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Consensus manages distributed agreement between nodes.
type Consensus struct {
	mu            sync.Mutex
	State         *ServerState
	prioMgr       *PriorityManager
	nodes         []string
	httpClient    *http.Client
	nodeAlive     map[string]bool
	failureCount  map[string]int
	aliveStatusMu sync.RWMutex
}

// NewConsensus initializes consensus with PriorityManager.
func NewConsensus(myAddress string, nodes []string) *Consensus {
	serverState := NewServerState(myAddress)
	priorityManager := &PriorityManager{}
	priorityManager.Init(len(nodes), (len(nodes)/2)+1, 1, 0.01, true)
	if CabinetWeights == nil {
		CabinetWeights = make(map[string]float64)
	}
	cons := &Consensus{
		State:         serverState,
		prioMgr:       priorityManager,
		nodes:         nodes,
		httpClient:    &http.Client{Timeout: 3 * time.Second},
		nodeAlive:     make(map[string]bool),
		failureCount:  make(map[string]int),
		aliveStatusMu: sync.RWMutex{},
	}

	fmt.Println("Nodes in consensus:", nodes)

	// Start heartbeat monitor only if follower
	if !cons.State.IsLeader() {
		go cons.monitorHeartbeat()

	}

	return cons
}

// ProposeChange starts a Cabinet++ consensus operation initiated by any node.
func (c *Consensus) ProposeChange(opType, key, value string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Printf("🔍 Checking consensus for %s: key=%s, value=%s\n", opType, key, value)
	fmt.Printf("ℹ️ Initiating proposal from: %s\n", c.State.GetMyAddress())

	// 📦 Log CabinetWeights snapshot before proposal
	fmt.Println("📦 CabinetWeights BEFORE proposal:")
	for node, weight := range CabinetWeights {
		fmt.Printf("🔸 %s → %.2f\n", node, weight)
	}

	type responderInfo struct {
		node     string
		duration time.Duration
	}

	proposer := c.State.GetMyAddress()
	fullAddr := serverIDFromAddress(proposer) + ":" + portFromAddress(proposer)

	c.aliveStatusMu.RLock()
	isAlive := c.nodeAlive[fullAddr]
	c.aliveStatusMu.RUnlock()

	approvalWeight := 0.0
	var responders []responderInfo

	// ✅ Count proposer vote if alive
	if isAlive {
		if w, ok := CabinetWeights[fullAddr]; ok {
			approvalWeight += w
			responders = append(responders, responderInfo{node: fullAddr, duration: 0})
			fmt.Printf("✅ Proposer %s is alive with weight %.2f\n", fullAddr, w)
		} else {
			fmt.Printf("⚠️ Proposer %s is alive but has no Cabinet weight entry\n", fullAddr)
		}
	}

	// 📣 Ask other nodes for approval
	for _, node := range c.nodes {
		if node == proposer {
			continue
		}

		start := time.Now()
		approved := c.requestApproval(node, opType, key, value)
		elapsed := time.Since(start)

		id := serverIDFromAddress(node)
		port := portFromAddress(node)
		fullAddr := id + ":" + port

		if approved {
			sid := c.getServerIDFromAddress(node)
			if sid == -1 {
				fmt.Printf("⚠️ Unknown node %s, skipping\n", node)
				continue
			}
			w := c.prioMgr.GetNodeWeight(sid)
			approvalWeight += w
			fmt.Printf("✅ %s approved with weight %.2f\n", fullAddr, w)
			responders = append(responders, responderInfo{node: fullAddr, duration: elapsed})
		} else {
			fmt.Printf("🔹 Approval from %s: false\n", node)
		}

		fmt.Printf("🧮 Final approvalWeight = %.2f, required = %.2f\n", approvalWeight, c.prioMgr.GetMajority())

		// ✅ If quorum met, commit change
		if approvalWeight > CabinetThreshold {
			fmt.Println("✅ Consensus REACHED. Committing change.")
			c.commitChange(opType, key, value)

			// ⚡ Sort responders by responsiveness (fastest first)
			sort.Slice(responders, func(i, j int) bool {
				return responders[i].duration < responders[j].duration
			})

			// 🔄 Extract ordered responder list
			var ordered []string
			for _, r := range responders {
				ordered = append(ordered, r.node)
			}

			// 🔁 Update Cabinet Weights (skip for dummy writes)
			if !isDummyKey(key) {
				if !c.State.IsLeader() {
					leader := c.State.GetLeader()
					if leader != "" {
						body := bytes.NewBuffer([]byte(fmt.Sprintf(`{"sender": "%s"}`, c.State.GetMyAddress())))
						http.Post("http://"+leader+"/api/notify-consensus", "application/json", body)
					}
				}
				c.UpdateCabinetWeights(ordered)

				// 📦 Log new weights
				fmt.Println("📦 CabinetWeights AFTER update:")
				for node, weight := range CabinetWeights {
					fmt.Printf("🔸 %s → %.2f\n", node, weight)
				}
			}
			return true
		}
	}

	fmt.Println("❌ Consensus NOT REACHED. Rejecting request.")
	return false
}

func isDummyKey(key string) bool {
	return strings.HasPrefix(key, "__cabinet_dummy__")
}

func (c *Consensus) GetPeers() []string {
	return c.nodes
}

func (c *Consensus) MarkNodeAlive(address string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nodeAlive == nil {
		c.nodeAlive = make(map[string]bool)
	}
	c.nodeAlive[address] = true
	fmt.Printf("✅ Marked %s as alive via NotifyConsensus\n", address)
}

func (c *Consensus) getServerIDFromAddress(addr string) serverID {
	for i, node := range c.nodes {
		if node == addr {
			return serverID(i)
		}
	}
	return -1 // invalid or not found
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
		go c.StartHeartbeatBroadcast()

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
		// ✅ Auto-trigger dummy write to recalculate CabinetWeights
		go func() {
			time.Sleep(1 * time.Second) // optional small delay
			fmt.Println("📊 Triggering dummy write to refresh CabinetWeights")
			c.ProposeChange("put", "__cabinet_dummy__", fmt.Sprintf("refresh-%d", time.Now().UnixNano()))

		}()
	} else {
		fmt.Println("🙅 This node did not win the election.")
	}
}
func (c *Consensus) StartHeartbeatBroadcast() {
	fmt.Println("📡 Starting heartbeat broadcast loop...")
	fmt.Printf("🔥 Broadcasting heartbeat from Consensus instance: %p\n", c)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !c.State.IsLeader() {
			fmt.Println("🛑 No longer leader. Stopping heartbeat broadcast.")
			return
		}

		// ✅ Mark the leader itself as alive
		leaderAddr := c.State.GetMyAddress()
		id := serverIDFromAddress(leaderAddr)
		port := portFromAddress(leaderAddr)
		fullAddr := id + ":" + port

		c.aliveStatusMu.Lock()
		c.nodeAlive[fullAddr] = true
		fmt.Printf("🧠 Updated nodeAlive[%s] = true. Current map: %+v\n", fullAddr, c.nodeAlive)
		c.aliveStatusMu.Unlock()

		for _, node := range c.nodes {
			if node == leaderAddr {
				continue
			}

			n := node // capture loop variable

			go func(n string) {
				url := "http://" + n + "/api/heartbeat"
				resp, err := c.httpClient.Get(url)

				id := serverIDFromAddress(n)
				port := portFromAddress(n)
				fullAddr := id + ":" + port

				c.aliveStatusMu.Lock()
				defer c.aliveStatusMu.Unlock()

				if err != nil || resp.StatusCode != http.StatusOK {
					c.failureCount[fullAddr]++
					if c.failureCount[fullAddr] >= 3 {
						c.nodeAlive[fullAddr] = false
					}
					fmt.Printf("❌ Failed heartbeat to %s (%d fails)\n", fullAddr, c.failureCount[fullAddr])
					return
				}

				defer resp.Body.Close()
				c.failureCount[fullAddr] = 0
				c.nodeAlive[fullAddr] = true
				fmt.Printf("✅ Heartbeat ACK from %s\n", fullAddr)
			}(n)
		}
	}
}

func serverIDFromAddress(addr string) string {
	return strings.Split(addr, ":")[0]
}

func portFromAddress(addr string) string {
	parts := strings.Split(addr, ":")
	if len(parts) > 1 {
		return parts[1]
	}
	return "8081" // default fallback
}

func (c *Consensus) GetNodeWeight(addr string) float64 {
	for i, node := range c.nodes {
		if node == addr {
			return c.prioMgr.GetNodeWeight(serverID(i))
		}
	}
	return 0
}
func (c *Consensus) UpdateCabinetWeights(responders []string) {
	newWeights := make(map[string]float64)
	totalWeight := 0.0

	// 1. Determine alive nodes
	c.aliveStatusMu.RLock()
	aliveNodes := make([]string, 0)
	for _, node := range c.nodes {
		id := serverIDFromAddress(node)
		port := portFromAddress(node)
		fullAddr := id + ":" + port
		if c.nodeAlive[fullAddr] {
			aliveNodes = append(aliveNodes, fullAddr)
		}
	}
	c.aliveStatusMu.RUnlock()

	// 2. Assign descending weights to responders
	r := 1.5
	a := 1.0
	n := len(responders)
	for i, addr := range responders {
		w := a * math.Pow(r, float64(n-1-i))
		newWeights[addr] = w
		totalWeight += w
	}

	// 3. Fallback: assign small weight to alive non-responders
	baseWeight := 1.0
	for _, addr := range aliveNodes {
		if _, ok := newWeights[addr]; !ok {
			newWeights[addr] = baseWeight
			totalWeight += baseWeight
		}
	}

	// 4. Normalize weights
	for addr, weight := range newWeights {
		newWeights[addr] = weight / totalWeight
	}

	// Update global weights
	CabinetWeights = newWeights

	// Compute threshold as 51% of total weight of ALIVE nodes
	aliveWeight := 0.0
	const quorumRatio = 0.51
	for _, addr := range aliveNodes {
		if w, ok := newWeights[addr]; ok {
			aliveWeight += w
		}
	}
	fmt.Printf("📊 Total alive weight before thresholding: %.2f\n", aliveWeight)

	if aliveWeight == 0 {
		fmt.Println("⚠️ No alive nodes with valid weights — skipping CabinetThreshold update to avoid unsafe quorum.")
		return
	}

	CabinetThreshold = math.Max(quorumRatio*aliveWeight, 0.51)
	fmt.Printf("📊 Total alive weight before thresholding: %.2f\n", aliveWeight)

	fmt.Println("🔁 Updated Cabinet Weights (Normalized):")
	for node, weight := range CabinetWeights {
		fmt.Printf("🔸 %s → %.2f\n", node, weight)
	}
	fmt.Printf("🎯 New CabinetThreshold = %.2f\n", CabinetThreshold)
}

func (c *Consensus) GetNodeStatus() map[string]bool {
	c.aliveStatusMu.RLock()
	defer c.aliveStatusMu.RUnlock()

	fmt.Printf("🔍 [GetNodeStatus] nodeAlive map: %+v\n", c.nodeAlive)
	fmt.Printf("👀 /api/status served from Consensus instance: %p\n", c)

	statusCopy := make(map[string]bool)
	for k, v := range c.nodeAlive {
		statusCopy[k] = v
	}
	return statusCopy
}
func (c *Consensus) GetCabinetWeights() map[string]float64 {
	// Defensive copy to avoid exposing internal map
	result := make(map[string]float64)
	for k, v := range CabinetWeights {
		result[k] = v
	}
	return result
}

func (c *Consensus) GetAllNodes() []string {
	return c.nodes
}
