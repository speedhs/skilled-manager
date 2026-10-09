.PHONY: all build build-cli build-gui test test-core run-cli run-gui clean install-deps

BIN_DIR := ./bin
CLI_BIN := $(BIN_DIR)/skills
GUI_BIN := $(BIN_DIR)/skills-gui

all: build-cli build-gui

# Build CLI binary
build-cli:
	@mkdir -p $(BIN_DIR)
	go build -o $(CLI_BIN) ./cmd/skills

# Build GUI binary
build-gui:
	@mkdir -p $(BIN_DIR)
	go build -o $(GUI_BIN) ./cmd/skills-gui

build: build-cli

# Run unit tests for pure Go core package
test-core:
	go test -v -race ./internal/core/...

test: test-core

# Run all tests (including UI when graphical libraries are available)
test-all:
	go test -v ./...

# Run the CLI tool
run-cli:
	go run ./cmd/skills list

# Run the Fyne Desktop GUI
run-gui:
	go run ./cmd/skills-gui

# Install Linux GUI development dependencies (Debian/Ubuntu)
install-deps:
	sudo apt-get update && sudo apt-get install -y libgl1-mesa-dev xorg-dev libwayland-dev

clean:
	rm -rf $(BIN_DIR) agent-skills.zip
