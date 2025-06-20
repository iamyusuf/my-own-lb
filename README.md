# Simple Load Balancer in Go

This is a simple HTTP load balancer built in Go as part of the [Coding Challenges](https://codingchallenges.fyi/challenges/challenge-load-balancer).

## Features

- Distributes traffic across multiple backend servers using a round-robin algorithm
- Performs regular health checks on backend servers
- Automatically removes unhealthy servers from the rotation
- Reintroduces servers when they become healthy again
- Configurable health check path and interval

## Usage

### Building the Load Balancer

```bash
go build -o lb
```

### Running the Load Balancer

```bash
# Basic usage with two backend servers
./lb -server http://localhost:8080 -server http://localhost:8081

# Running on a different port (default: 80)
./lb -port 8000 -server http://localhost:8080 -server http://localhost:8081

# Custom health check path and interval
./lb -health /health -interval 10 -server http://localhost:8080 -server http://localhost:8081
```

### Command Line Options

- `-port`: Port to run the load balancer on (default: 80)
- `-server`: Backend server URL (can be specified multiple times)
- `-health`: Path to use for health checks (default: "/")
- `-interval`: Health check interval in seconds (default: 30)

## Testing

You can run the tests with:

```bash
go test
```

## Example Setup

1. Start backend servers (e.g., using Python's HTTP server):

```bash
# Terminal 1
python3 -m http.server 8080 --directory server1

# Terminal 2
python3 -m http.server 8081 --directory server2

# Terminal 3
python3 -m http.server 8082 --directory server3
```

2. Start the load balancer:

```bash
./lb -port 8000 -server http://localhost:8080 -server http://localhost:8081 -server http://localhost:8082 -interval 10
```

3. Send requests to the load balancer:

```bash
curl http://localhost:8000/
```

4. Test health checking by stopping one of the backend servers and observing how the load balancer adapts.
