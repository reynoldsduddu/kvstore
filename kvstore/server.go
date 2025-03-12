package kvstore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
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
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	http.ServeFile(w, r, filepath.Join("frontend", path))
}

// PutHandler handles distributed PUT requests.
func (s *Server) PutHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	fmt.Println("📥 Received PUT request...")
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Println("❌ Failed to decode JSON:", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	fmt.Printf("🔹 Storing key=%s, value=%s...\n", req.Key, req.Value)
	err := s.store.Put(req.Key, req.Value)
	if err != nil {
		fmt.Printf("❌ Consensus failed for PUT key=%s: %v\n", req.Key, err)
		http.Error(w, fmt.Sprintf("Consensus not reached: %v", err), http.StatusConflict)
		return
	}

	fmt.Printf("✅ PUT successful: key=%s, value=%s\n", req.Key, req.Value)
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
		fmt.Println("❌ This node is a leader and cannot approve its own requests.")
		http.Error(w, "Leaders cannot approve their own requests", http.StatusForbidden)
		return
	}

	// Simulate approval (followers always approve)
	w.WriteHeader(http.StatusOK)
	fmt.Println("✅ Approval granted by follower.")
}

// Start initializes the HTTP server.
func (s *Server) Start(addr string) error {
	http.Handle("/", http.HandlerFunc(s.ServeStatic))
	http.HandleFunc("/put", s.PutHandler)
	http.HandleFunc("/get", s.GetHandler)
	http.HandleFunc("/delete", s.DeleteHandler)
	http.HandleFunc("/approve", s.ApproveHandler)
	return http.ListenAndServe(addr, nil)
}
