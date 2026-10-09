.PHONY: all build build-cli build-gui test test-core run-cli run-gui package-gui clean install-deps release-snapshot

BIN_DIR := ./bin
CLI_BIN := $(BIN_DIR)/skills
GUI_BIN := $(BIN_DIR)/skills-gui

all: build-cli build-gui

# Build CLI binary with CGO disabled (pure static Go binary)
build-cli:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(CLI_BIN) ./cmd/skills

# Build GUI binary (requires CGO and display dependencies)
build-gui:
	@mkdir -p $(BIN_DIR)
	go build -o $(GUI_BIN) ./cmd/skills-gui

build: build-cli

# Package desktop GUI with fyne package tool
package-gui:
	@which fyne > /dev/null || (echo "Installing fyne CLI..." && go install fyne.io/fyne/v2/cmd/fyne@latest)
	cd cmd/skills-gui && fyne package -name "skills-gui" -appID "com.agent.skillsmanager"

# Run unit tests for pure Go core package
test-core:
	go test -v -race ./pkg/core/...

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

# Test GoReleaser build locally in snapshot mode
release-snapshot:
	goreleaser release --snapshot --clean

# Install Linux GUI development dependencies (Debian/Ubuntu)
install-deps:
	sudo apt-get update && sudo apt-get install -y libgl1-mesa-dev xorg-dev libwayland-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev

clean:
	rm -rf $(BIN_DIR) dist/ agent-skills.zip *.tar.gz *.zip
