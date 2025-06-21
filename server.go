package main

import (
	"net/http"
	"net/url"
	"sync"
)

// Server represents a backend server
type Server struct {
	URL          *url.URL
	Alive        bool
	mux          sync.RWMutex
	ReverseProxy http.Handler
}

// SetAlive updates the alive status of the backend server
func (s *Server) SetAlive(alive bool) {
	s.mux.Lock()
	s.Alive = alive
	s.mux.Unlock()
}

// IsAlive returns true when the backend server is alive
func (s *Server) IsAlive() bool {
	s.mux.RLock()
	defer s.mux.RUnlock()
	return s.Alive
}
