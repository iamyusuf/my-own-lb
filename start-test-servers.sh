#!/bin/bash

# Start Python HTTP servers for testing
echo "Starting test servers on ports 8080, 8081, and 8082..."
python3 -m http.server 8080 --directory server8080 &
SERVER1_PID=$!
python3 -m http.server 8081 --directory server8081 &
SERVER2_PID=$!
python3 -m http.server 8082 --directory server8082 &
SERVER3_PID=$!

echo "Test servers started. Press Ctrl+C to stop."

# Trap function to clean up when script is terminated
cleanup() {
    echo "Stopping test servers..."
    kill $SERVER1_PID
    kill $SERVER2_PID
    kill $SERVER3_PID
    exit 0
}

trap cleanup SIGINT SIGTERM

# Wait for termination
while true; do
    sleep 1
done
