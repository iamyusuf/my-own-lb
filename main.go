package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
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

// LoadBalancer represents a load balancer
type LoadBalancer struct {
	servers       []*Server
	current       int
	mu            sync.Mutex
	healthCheck   string
	serverStats   map[string]int // Track requests per server
	statsMu       sync.Mutex     // Mutex for stats
	totalRequests int            // Total number of requests handled
}

// NextServer returns the next server based on round-robin algorithm
func (lb *LoadBalancer) NextServer() *Server {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Check for available servers
	serverCount := len(lb.servers)
	if serverCount == 0 {
		return nil
	}

	// Try to find an available server using round-robin
	for i := 0; i < serverCount; i++ {
		// Move to next server (round-robin)
		lb.current = (lb.current + 1) % serverCount

		// Check if this server is alive
		if lb.servers[lb.current].IsAlive() {
			return lb.servers[lb.current]
		}
	}

	// If we went through all servers and none are alive
	return nil
}

// ServeHTTP implements the http.Handler interface
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Special endpoint for stats
	if r.URL.Path == "/lb-stats" {
		lb.handleStats(w, r)
		return
	}

	// Log incoming request
	fmt.Printf("Received request from %s\n%s %s %s\n", r.RemoteAddr, r.Method, r.URL.Path, r.Proto)
	for name, headers := range r.Header {
		for _, h := range headers {
			fmt.Printf("%s: %s\n", name, h)
		}
	}

	// Get the next available server
	server := lb.NextServer()
	if server == nil {
		http.Error(w, "No available servers", http.StatusServiceUnavailable)
		return
	}

	// Update statistics
	lb.statsMu.Lock()
	lb.totalRequests++
	lb.serverStats[server.URL.Host]++
	lb.statsMu.Unlock()

	// Create the backend URL
	targetURL := *server.URL
	targetURL.Path = r.URL.Path
	targetURL.RawQuery = r.URL.RawQuery

	// Create a client
	client := &http.Client{}

	// Create the request to send to the backend
	req, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy the headers from the original request
	for name, values := range r.Header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	// Send the request to the backend
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy the response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Copy the response body
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Printf("Response from server: %s %s\n", resp.Proto, resp.Status)
}

// HealthCheck performs a health check on all backend servers
func (lb *LoadBalancer) HealthCheck() {
	for _, server := range lb.servers {
		status := "up"
		serverURL := *server.URL
		serverURL.Path = lb.healthCheck

		resp, err := http.Get(serverURL.String())
		if err != nil {
			log.Printf("Health check failed for %s: %s", serverURL.String(), err)
			server.SetAlive(false)
			status = "down"
		} else {
			if resp.StatusCode == http.StatusOK {
				server.SetAlive(true)
			} else {
				server.SetAlive(false)
				status = "down"
			}
			resp.Body.Close()
		}
		log.Printf("Health check for %s: %s", serverURL.String(), status)
	}
}

// ScheduleHealthChecks schedules health checks at regular intervals
func (lb *LoadBalancer) ScheduleHealthChecks(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		// Run an initial health check immediately
		lb.HealthCheck()

		// Then run on the ticker schedule
		for range ticker.C {
			lb.HealthCheck()
		}
	}()
}

// handleStats displays load balancing statistics
func (lb *LoadBalancer) handleStats(w http.ResponseWriter, r *http.Request) {
	lb.statsMu.Lock()
	defer lb.statsMu.Unlock()

	fmt.Fprintf(w, "Load Balancer Statistics:\n\n")
	fmt.Fprintf(w, "Total Requests: %d\n\n", lb.totalRequests)
	fmt.Fprintf(w, "Distribution:\n")

	for host, count := range lb.serverStats {
		percent := 0.0
		if lb.totalRequests > 0 {
			percent = float64(count) / float64(lb.totalRequests) * 100
		}
		fmt.Fprintf(w, "  %s: %d requests (%.1f%%)\n", host, count, percent)
	}

	fmt.Fprintf(w, "\nServer Health:\n")
	for _, server := range lb.servers {
		status := "UP"
		if !server.IsAlive() {
			status = "DOWN"
		}
		fmt.Fprintf(w, "  %s: %s\n", server.URL.Host, status)
	}
}

func main() {
	// Define command line flags
	port := flag.Int("port", 80, "Port to run the load balancer on")
	healthCheckPath := flag.String("health", "/", "Path to use for health checks")
	healthCheckInterval := flag.Int("interval", 30, "Health check interval in seconds")

	// Define servers using StringSlice flag
	var serverURLs stringSliceFlag
	flag.Var(&serverURLs, "server", "Backend server URL (can be specified multiple times)")

	flag.Parse()

	// Check if servers are provided
	if len(serverURLs) == 0 {
		log.Fatal("No backend servers specified. Use -server flag to specify at least one server.")
	}

	// Initialize servers
	var servers []*Server
	for _, serverURL := range serverURLs {
		url, err := url.Parse(serverURL)
		if err != nil {
			log.Fatalf("Invalid server URL: %s", err)
		}
		servers = append(servers, &Server{
			URL:   url,
			Alive: true,
		})
		log.Printf("Added backend server: %s", url.String())
	}

	// Create load balancer
	lb := &LoadBalancer{
		servers:       servers,
		current:       -1, // Start at -1 so first call to NextServer gives us index 0
		healthCheck:   *healthCheckPath,
		serverStats:   make(map[string]int),
		totalRequests: 0,
	}

	// Schedule health checks
	lb.ScheduleHealthChecks(time.Duration(*healthCheckInterval) * time.Second)

	// Print startup information
	log.Printf("Load balancer starting on port %d", *port)
	log.Printf("Health check path: %s", *healthCheckPath)
	log.Printf("Health check interval: %d seconds", *healthCheckInterval)

	// Start the HTTP server
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), lb); err != nil {
		log.Fatal(err)
	}
}

// StringSliceFlag is a custom flag for handling multiple string values
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return fmt.Sprintf("%v", []string(*s))
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}
