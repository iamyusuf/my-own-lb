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
	servers     []*Server
	current     int
	mu          sync.Mutex
	healthCheck string
}

// NextServer returns the next server based on round-robin algorithm
func (lb *LoadBalancer) NextServer() *Server {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Check for available servers
	serverCount := len(lb.servers)
	for i := 0; i < serverCount; i++ {
		lb.current = (lb.current + 1) % serverCount
		if lb.servers[lb.current].IsAlive() {
			return lb.servers[lb.current]
		}
	}
	return nil
}

// ServeHTTP implements the http.Handler interface
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		for {
			select {
			case <-ticker.C:
				lb.HealthCheck()
			}
		}
	}()
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
		servers:     servers,
		healthCheck: *healthCheckPath,
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
