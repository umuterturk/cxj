// Package server exposes the loopback HTTP API defined in CONTRACT.md.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"example.com/cx/internal/ir"
)

// Index is the minimal read surface the HTTP layer needs.
type Index interface {
	Status() ir.Status
	Search(q, mode string, limit int) (ir.SearchReply, error)
	References(symbol string) ir.RefReply
	Endpoints(method, path string) any
	Beans(typ string) any
	Context(symbol string, budget int) any
}

// Server is the loopback JSON API.
type Server struct {
	index Index
	log   *log.Logger
	mu    sync.Mutex
}

// New creates a Server. logger may be nil (stderr default).
func New(index Index, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{index: index, log: logger}
}

// ListenAndServe binds addr (must be loopback) and serves until the listener closes.
func (s *Server) ListenAndServe(addr string) error {
	if err := requireLoopback(addr); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", s.handleStatus)
	mux.HandleFunc("/v1/search", s.handleSearch)
	mux.HandleFunc("/v1/references", s.handleReferences)
	mux.HandleFunc("/v1/endpoints", s.handleEndpoints)
	mux.HandleFunc("/v1/beans", s.handleBeans)
	mux.HandleFunc("/v1/context", s.handleContext)
	srv := &http.Server{Handler: mux}
	s.log.Printf("listening on http://%s", ln.Addr().String())
	return srv.Serve(ln)
}

func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --http address %q: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("refusing non-loopback bind address %q; use 127.0.0.1 or ::1", addr)
	}
	return nil
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st := s.index.Status()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	st.GoHeapAllocBytes = mem.HeapAlloc
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query().Get("q")
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "lexical"
	}
	limit := 10
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = n
	}
	if limit > 50 {
		limit = 50
	}
	reply, err := s.index.Search(q, mode, limit)
	if err != nil {
		if isSemanticNotReady(err) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "semantic_not_ready"})
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, reply)
}

func (s *Server) handleReferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	symbol := r.URL.Query().Get("symbol")
	writeJSON(w, http.StatusOK, s.index.References(symbol))
}

func (s *Server) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.index.Endpoints(r.URL.Query().Get("method"), r.URL.Query().Get("path")))
}

func (s *Server) handleBeans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.index.Beans(r.URL.Query().Get("type")))
}

func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	budget := 500
	if raw := r.URL.Query().Get("budget"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			budget = n
		}
	}
	writeJSON(w, http.StatusOK, s.index.Context(r.URL.Query().Get("symbol"), budget))
}

func isSemanticNotReady(err error) bool {
	return err != nil && strings.Contains(err.Error(), "semantic_not_ready")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
