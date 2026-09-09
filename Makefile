.PHONY: build run test clean tmux-start tmux-stop

# Build the unified single binary with embedded web assets
build:
	@mkdir -p bin
	go build -ldflags="-s -w" -o bin/gateway ./cmd/gateway
	@echo "Build complete: bin/gateway"

# Run locally in foreground
run: build
	./bin/gateway -port 58900

# Run all unit and integration tests
test:
	go test -v ./...

# Clean artifacts
clean:
	rm -rf bin logs/*.log

# Launch gateway in a detached tmux session with log tee
tmux-start: build
	@mkdir -p logs
	@./scripts/tmux-start.sh

# Stop the tmux session
tmux-stop:
	@./scripts/tmux-stop.sh
