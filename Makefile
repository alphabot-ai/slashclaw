.PHONY: build run test clean dev

BINARY=slashclaw
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/slashclaw

run: build
	./$(BUILD_DIR)/$(BINARY)

dev:
	go run ./cmd/slashclaw

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)
	rm -f slashclaw.db

fmt:
	go fmt ./...

lint:
	go vet ./...
