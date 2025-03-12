package consensus

import (
	"fmt"
	"sync"
)

// ServerState tracks if a node is a leader or a follower.
type ServerState struct {
	mu        sync.RWMutex
	myAddress string
	leader    string
}

// NewServerState initializes a node's state.
func NewServerState(myAddress string) *ServerState {
	state := &ServerState{
		myAddress: myAddress,
	}

	// Automatically set the first node as leader
	if myAddress == "127.0.0.1:8081" {
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
