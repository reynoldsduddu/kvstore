package consensus

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// ServerState tracks if a node is a leader or a follower.
type ServerState struct {
	mu            sync.RWMutex
	myAddress     string
	leader        string
	lastHeartbeat time.Time
}

func (s *ServerState) UpdateHeartbeat() {
	s.mu.Lock()
	s.lastHeartbeat = time.Now()
	s.mu.Unlock()
}

func (s *ServerState) IsHeartbeatStale(timeout time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.lastHeartbeat) > timeout
}

// NewServerState initializes a node's state.
func NewServerState(myAddress string) *ServerState {
	state := &ServerState{
		myAddress: myAddress,
	}

	// Automatically set the first node as leader
	if myAddress == "node0:8081" {
		state.SetLeader(myAddress)
		fmt.Println("👑 This node is the leader.")
	} else {
		fmt.Println("🔹 This node is a follower.")
	}

	return state
}

// SetLeader assigns a leader.
func (s *ServerState) SetLeader(leader string) {
	s.mu.Lock()
	s.leader = leader
	s.mu.Unlock()
}

// GetLeader returns the current leader.
func (s *ServerState) GetLeader() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.leader
}

// IsLeader checks if the current node is the leader.
func (s *ServerState) IsLeader() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.myAddress == s.leader
}

// IsFollower checks if the current node is a follower.
func (s *ServerState) IsFollower() bool {
	return !s.IsLeader()
}

// GetMyAddress returns the node's address.
func (s *ServerState) GetMyAddress() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.myAddress
}

var CabinetWeights map[string]float64
var CabinetThreshold float64

func InitCabinetWeights(peers []string) {
	CabinetWeights = make(map[string]float64)
	r := 1.5
	a := 1.0
	sum := 0.0

	for i := 0; i < len(peers); i++ {
		id := peers[i]
		w := a * math.Pow(r, float64(len(peers)-1-i))
		CabinetWeights[id] = w
		sum += w
	}

	CabinetThreshold = sum / 2.0
}

func HasCabinetQuorum(acks map[string]bool) bool {
	total := 0.0
	for id, ack := range acks {
		if ack {
			total += CabinetWeights[id]
		}
	}
	return total > CabinetThreshold
}
