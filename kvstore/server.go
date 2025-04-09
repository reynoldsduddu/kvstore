package kvstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// Server represents an HTTP server for the key-value store.
type Server struct {
	store *KVStore
}

// NewServer initializes an HTTP server for the store.
func NewServer(store *KVStore) *Server {
	return &Server{store: store}
}

// ServeStatic serves static files (HTML, JS, CSS).
func (s *Server) ServeStatic(w http.ResponseWriter, r *http.Request) {
	var path string
	if r.URL.Path == "/" {
		path = "/app/frontend/index.html"
	} else {
		path = "/app/frontend" + r.URL.Path
	}

	http.ServeFile(w, r, path)
}

// PutHandler handles distributed PUT requests.
func (s *Server) PutHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	fmt.Println("📥 Received PUT request...")
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Println("Failed to decode JSON:", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	fmt.Printf("🔹 Storing key=%s, value=%s...\n", req.Key, req.Value)
	var err error
	if s.store.consensus.Mode == "cabinet" {
		if !s.store.consensus.State.IsLeader() {
			leader := s.store.consensus.State.GetLeader()
			if leader == "" {
				http.Error(w, "Leader unknown", http.StatusServiceUnavailable)
				return
			}

			// 🔁 Forward to leader
			fmt.Printf("🔀 Forwarding PUT to leader %s\n", leader)
			proxyURL := fmt.Sprintf("http://%s/api/put", leader)
			reqBody, _ := json.Marshal(req)
			resp, err := http.Post(proxyURL, "application/json", bytes.NewReader(reqBody))
			if err != nil {
				fmt.Printf("❌ Forwarding failed: %v\n", err)
				http.Error(w, "Failed to forward to leader", http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
			return
		}

		// ✅ This node is the leader — handle normally
		err = s.store.Put(req.Key, req.Value)
	} else if s.store.consensus.Mode == "cabinet++" {
		err = s.store.Put(req.Key, req.Value)
	} else {
		http.Error(w, "Unknown consensus mode", http.StatusInternalServerError)
		return
	}

	if err != nil {
		fmt.Printf("Consensus failed for PUT key=%s: %v\n", req.Key, err)
		http.Error(w, fmt.Sprintf("Consensus not reached: %v", err), http.StatusConflict)
		return
	}

	fmt.Printf("PUT successful: key=%s, value=%s\n", req.Key, req.Value)
	w.WriteHeader(http.StatusOK)
}

// GetHandler handles GET requests.
func (s *Server) GetHandler(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "Missing key parameter", http.StatusBadRequest)
		return
	}

	value, exists, err := s.store.Get(key)
	if err != nil {
		http.Error(w, "Failed to retrieve value", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Key not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"value": value})
}

// DeleteHandler handles distributed DELETE requests.
func (s *Server) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "Missing key parameter", http.StatusBadRequest)
		return
	}

	if err := s.store.Delete(key); err != nil {
		http.Error(w, "Consensus not reached", http.StatusConflict)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ApproveHandler allows followers to approve leader proposals.
func (s *Server) ApproveHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("📥 Received approval request...")

	// Check if this node is a follower
	if s.store.consensus.State.IsLeader() {
		fmt.Println("This node is a leader and cannot approve its own requests.")
		http.Error(w, "Leaders cannot approve their own requests", http.StatusForbidden)
		return
	}

	// Simulate approval (followers always approve)
	w.WriteHeader(http.StatusOK)
	fmt.Println("Approval granted by follower.")
}

type PaginatedResponse struct {
	Data       map[string]string `json:"data"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
	TotalItems int               `json:"totalItems"`
	TotalPages int               `json:"totalPages"`
}

// GetAllHandler handles GET requests to retrieve paginated key-value pairs.
func (s *Server) GetAllHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	// Set default values if not provided
	page := 1
	limit := 10

	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Calculate the offset
	offset := (page - 1) * limit

	// Query the database for paginated results
	rows, err := s.store.db.Query("SELECT key, value FROM kv_store LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		http.Error(w, "Failed to retrieve key-value pairs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Create a map to store key-value pairs
	data := make(map[string]string)

	// Iterate through the rows and populate the map
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			http.Error(w, "Failed to scan row", http.StatusInternalServerError)
			return
		}
		data[key] = value
	}

	// Get the total number of items
	var totalItems int
	err = s.store.db.QueryRow("SELECT COUNT(*) FROM kv_store").Scan(&totalItems)
	if err != nil {
		http.Error(w, "Failed to count key-value pairs", http.StatusInternalServerError)
		return
	}

	// Calculate the total number of pages
	totalPages := (totalItems + limit - 1) / limit

	// Return the data and metadata as JSON
	response := PaginatedResponse{
		Data:       data,
		Page:       page,
		Limit:      limit,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ReplicationRequest is the body sent to followers
type ReplicationRequest struct {
	OpType string `json:"opType"` // "PUT" or "DELETE"
	Key    string `json:"key"`
	Value  string `json:"value"` // only used for PUT
}

func (s *Server) ReplicationHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("📥 Received REPLICATION request!")

	var req ReplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	fmt.Printf("📦 Replicating %s: %s = %s\n", req.OpType, req.Key, req.Value)

	if req.OpType == "PUT" {
		s.store.ReplicatedPut(req.Key, req.Value)
	} else if req.OpType == "DELETE" {
		s.store.ReplicatedDelete(req.Key)
	} else {
		http.Error(w, "Unknown operation", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Leader status
func (s *Server) HeartbeatHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) PriorityHandler(w http.ResponseWriter, r *http.Request) {
	weight := s.store.consensus.GetNodeWeight(s.store.consensus.State.GetMyAddress())
	json.NewEncoder(w).Encode(weight)
}

// set leader
func (s *Server) SetLeaderHandler(w http.ResponseWriter, r *http.Request) {
	var payload map[string]string
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	leader, ok := payload["leader"]
	if ok {
		s.store.consensus.State.SetLeader(leader)
		fmt.Printf("🔄 Leader updated to: %s\n", leader)
	}
	w.WriteHeader(http.StatusOK)
}

// LeaderHandler returns the current leader's address.
func (s *Server) LeaderHandler(w http.ResponseWriter, r *http.Request) {
	leader := s.store.consensus.State.GetLeader()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"leader": leader})
}

// ProxyHandler forwards unknown requests to the current leader
func (s *Server) ProxyHandler(w http.ResponseWriter, r *http.Request) {
	leader := s.store.consensus.State.GetLeader()
	if leader == "" {
		http.Error(w, "No leader available", http.StatusServiceUnavailable)
		return
	}

	url := fmt.Sprintf("http://%s%s", leader, r.URL.Path)
	req, err := http.NewRequest(r.Method, url, r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	req.Header = r.Header
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Leader not reachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
func (s *Server) StatusHandler(w http.ResponseWriter, r *http.Request) {
	if !s.store.consensus.State.IsLeader() {
		leader := s.store.consensus.State.GetLeader()
		if leader == "" {
			http.Error(w, "No leader available", http.StatusServiceUnavailable)
			return
		}
		resp, err := http.Get("http://" + leader + "/api/status")
		if err != nil {
			http.Error(w, "Failed to proxy status to leader", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, resp.Body)
		return
	}

	// Serve locally if leader
	status := s.store.consensus.GetNodeStatus()
	fmt.Printf("📤 Serving /api/status: %+v\n", status)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
func (s *Server) WeightsHandler(w http.ResponseWriter, r *http.Request) {
	if !s.store.consensus.State.IsLeader() {
		leader := s.store.consensus.State.GetLeader()
		if leader == "" {
			http.Error(w, "No leader available", http.StatusServiceUnavailable)
			return
		}
		resp, err := http.Get("http://" + leader + "/api/weights")
		if err != nil {
			http.Error(w, "Failed to proxy weights to leader", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, resp.Body)
		return
	}

	// ✅ Return the weights from the Consensus instance
	w.Header().Set("Content-Type", "application/json")
	weights := s.store.consensus.GetCabinetWeights()
	json.NewEncoder(w).Encode(weights)
}
func (s *Server) NotifyConsensusHandler(w http.ResponseWriter, r *http.Request) {
	if !s.store.consensus.State.IsLeader() {
		http.Error(w, "Not leader", http.StatusForbidden)
		return
	}

	var payload struct {
		Sender string `json:"sender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	fmt.Printf("🔔 Received consensus notification from %s\n", payload.Sender)
	s.store.consensus.MarkNodeAlive(payload.Sender)
	s.store.consensus.UpdateCabinetWeights(s.store.consensus.GetPeers())
	w.WriteHeader(http.StatusOK)
}
func (s *Server) ModeHandler(w http.ResponseWriter, r *http.Request) {
	mode := s.store.consensus.Mode
	json.NewEncoder(w).Encode(map[string]string{"mode": mode})
}

// Start initializes the HTTP server.
func (s *Server) Start(addr string) error {
	fmt.Println("Starting HTTP server on", addr)
	http.Handle("/", http.HandlerFunc(s.ServeStatic))
	http.HandleFunc("/api/put", s.PutHandler)
	http.HandleFunc("/api/get", s.GetHandler)
	http.HandleFunc("/api/get-all", s.GetAllHandler)
	http.HandleFunc("/api/delete", s.DeleteHandler)
	http.HandleFunc("/api/approve", s.ApproveHandler)
	http.HandleFunc("/api/replicate", s.ReplicationHandler)
	http.HandleFunc("/api/heartbeat", s.HeartbeatHandler)
	http.HandleFunc("/api/priority", s.PriorityHandler)
	http.HandleFunc("/api/set-leader", s.SetLeaderHandler)
	http.HandleFunc("/api/leader", s.LeaderHandler)
	http.HandleFunc("/api/weights", s.WeightsHandler)
	http.HandleFunc("/api/status", s.StatusHandler)
	http.HandleFunc("/api/notify-consensus", s.NotifyConsensusHandler)
	http.HandleFunc("/api/mode", s.ModeHandler)

	http.HandleFunc("/api/", s.ProxyHandler) // Catch-all fallback

	return http.ListenAndServe(addr, nil)
}
