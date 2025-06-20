package main

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestNextServer(t *testing.T) {
	// Create three test servers
	servers := []*Server{
		{URL: &url.URL{Scheme: "http", Host: "localhost:8080"}, Alive: true},
		{URL: &url.URL{Scheme: "http", Host: "localhost:8081"}, Alive: true},
		{URL: &url.URL{Scheme: "http", Host: "localhost:8082"}, Alive: true},
	}

	lb := &LoadBalancer{
		servers: servers,
	}

	// Check that we rotate through all servers in round-robin fashion
	s1 := lb.NextServer()
	if s1.URL.Host != "localhost:8081" {
		t.Errorf("Expected server 1, got %s", s1.URL.Host)
	}

	s2 := lb.NextServer()
	if s2.URL.Host != "localhost:8082" {
		t.Errorf("Expected server 2, got %s", s2.URL.Host)
	}

	s3 := lb.NextServer()
	if s3.URL.Host != "localhost:8080" {
		t.Errorf("Expected server 0, got %s", s3.URL.Host)
	}

	s4 := lb.NextServer()
	if s4.URL.Host != "localhost:8081" {
		t.Errorf("Expected server 1, got %s", s4.URL.Host)
	}

	// Test with a server marked as not alive
	servers[1].SetAlive(false)
	s5 := lb.NextServer()
	if s5.URL.Host != "localhost:8082" {
		t.Errorf("Expected server 2 (skipping unhealthy server 1), got %s", s5.URL.Host)
	}

	// All servers down
	servers[0].SetAlive(false)
	servers[2].SetAlive(false)
	s6 := lb.NextServer()
	if s6 != nil {
		t.Errorf("Expected nil server when all servers are down")
	}
}

func TestHealthCheck(t *testing.T) {
	// Create a test server
	testServer := http.NewServeMux()
	testServer.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	go http.ListenAndServe(":9090", testServer)
	time.Sleep(100 * time.Millisecond) // Give the server some time to start

	serverURL, _ := url.Parse("http://localhost:9090")
	server := &Server{
		URL:   serverURL,
		Alive: false, // Start with server as down
	}

	lb := &LoadBalancer{
		servers:     []*Server{server},
		healthCheck: "/health",
	}

	// Run health check, should mark the server as alive
	lb.HealthCheck()

	if !server.IsAlive() {
		t.Errorf("Server should be marked as alive after health check")
	}

	// Change the health check path to a non-existent one
	lb.healthCheck = "/does-not-exist"

	// Run health check, should mark the server as down
	lb.HealthCheck()

	if server.IsAlive() {
		t.Errorf("Server should be marked as down after failed health check")
	}
}
