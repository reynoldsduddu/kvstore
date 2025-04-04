package kvstore

import (
	"encoding/json"
	"fmt"
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
	err := s.store.Put(req.Key, req.Value)
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

// Start initializes the HTTP server.
func (s *Server) Start(addr string) error {
	fmt.Println("Starting HTTP server on", addr)
	http.Handle("/", http.HandlerFunc(s.ServeStatic))
	http.HandleFunc("/put", s.PutHandler)
	http.HandleFunc("/get", s.GetHandler)
	http.HandleFunc("/get-all", s.GetAllHandler)
	http.HandleFunc("/delete", s.DeleteHandler)
	http.HandleFunc("/approve", s.ApproveHandler)
	return http.ListenAndServe(addr, nil)
}
