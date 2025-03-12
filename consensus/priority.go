package consensus

import (
	"fmt"
	"math"
	"sync"
)

type serverID int
type prioClock int
type priority float64

type PriorityManager struct {
	sync.RWMutex
	m        map[prioClock]map[serverID]priority
	scheme   []priority
	majority float64
	n        int
	q        int
}

// Initialize the priority manager with a dynamic voting scheme.
func (pm *PriorityManager) Init(numOfServers, quorumSize, baseOfPriorities int, ratioTryStep float64, isCab bool) {
	pm.n = numOfServers
	pm.q = quorumSize // quorum size is t+1
	pm.m = make(map[prioClock]map[serverID]priority)

	ratio := 1.0
	if isCab {
		ratio = calcInitPrioRatio(numOfServers, quorumSize, ratioTryStep)
	}
	fmt.Println("🔢 Ratio for priority calculation:", ratio)

	newPriorities := make(map[serverID]priority)

	for i := 0; i < numOfServers; i++ {
		p := float64(baseOfPriorities) * math.Pow(ratio, float64(i))
		newPriorities[serverID(numOfServers-1-i)] = priority(p)
		pm.scheme = append(pm.scheme, priority(p))
	}

	reverseSlice(pm.scheme)
	pm.majority = sum(convertToFloat64(pm.scheme)) / 2

	pm.Lock()
	pm.m[0] = newPriorities
	pm.Unlock()
}

// Get the majority weight required for consensus.
func (pm *PriorityManager) GetMajority() float64 {
	return pm.majority
}

// Get the priority assigned to the leader.
func (pm *PriorityManager) GetLeaderWeight() float64 {
	return float64(pm.scheme[0])
}

// Get the priority assigned to a specific node.
func (pm *PriorityManager) GetNodeWeight(node serverID) float64 {
	if int(node) < len(pm.scheme) {
		return float64(pm.scheme[node])
	}
	return 0
}

// Convert `[]priority` to `[]float64` for summation.
func convertToFloat64(arr []priority) []float64 {
	result := make([]float64, len(arr))
	for i, val := range arr {
		result[i] = float64(val)
	}
	return result
}

// Calculate the priority ratio dynamically.
func calcInitPrioRatio(n, f int, ratioTryStep float64) float64 {
	r := 2.0 // initial guess
	for {
		if math.Pow(r, float64(n-f+1)) > 0.5*(math.Pow(r, float64(n))+1) &&
			0.5*(math.Pow(r, float64(n))+1) > math.Pow(r, float64(n-f)) {
			return r
		}
		r -= ratioTryStep
	}
}

// Reverse the priority scheme slice.
func reverseSlice(slice []priority) {
	length := len(slice)
	for i := 0; i < length/2; i++ {
		j := length - 1 - i
		slice[i], slice[j] = slice[j], slice[i]
	}
}

// Sum function for priority calculations.
func sum(arr []float64) float64 {
	total := 0.0
	for _, val := range arr {
		total += val
	}
	return total
}
