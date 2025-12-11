.PHONY: build run clean tidy

build:
	go build -o bin/agent cmd/agent/main.go

run:
	go run cmd/agent/main.go

clean:
	rm -rf bin/
	rm -rf browser-data/
	go clean

tidy:
	go mod tidy

install-deps:
	go mod download

.DEFAULT_GOAL := build
