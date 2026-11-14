.PHONY: all lint test test-race fuzz cover build-cli build-gui release-snapshot tools clean

BIN_DIR := ./bin
CLI_BIN := $(BIN_DIR)/skills
GUI_BIN := $(BIN_DIR)/skills-gui
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/speedhs/skilled-manager/internal/version.Version=$(VERSION) \
	-X github.com/speedhs/skilled-manager/internal/version.Commit=$(COMMIT) \
	-X github.com/speedhs/skilled-manager/internal/version.Date=$(DATE)

all: build-cli

tools:
	go install honnef.co/go/tools/cmd/staticcheck@v0.8.2
	go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
	go install github.com/securego/gosec/v2/cmd/gosec@v2.30.0
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

lint:
	go vet ./pkg/core/ ./internal/cli/ ./internal/version/ ./cmd/skills/
	staticcheck ./pkg/core/ ./internal/cli/ ./internal/version/ ./cmd/skills/
	gosec -exclude=G301,G302,G304,G306 ./pkg/core/... ./internal/cli/... ./cmd/skills/...
	golangci-lint run ./pkg/core/... ./internal/cli/... ./internal/version/... ./cmd/skills/...

test:
	go test ./pkg/core/ ./internal/cli/ ./internal/version/
	sh ./scripts/check-deps.sh
	sh ./scripts/check-hermetic.sh
	sh ./scripts/apidiff.sh

test-race:
	go test -race ./pkg/core/ ./internal/cli/ ./internal/version/

fuzz:
	go test ./pkg/core/ -fuzz=FuzzParseFrontmatter -fuzztime=20s
	go test ./pkg/core/ -fuzz=FuzzExpandPath -fuzztime=20s
	go test ./pkg/core/ -fuzz=FuzzValidateSkillName -fuzztime=20s
	go test ./pkg/core/ -fuzz=FuzzImportZip -fuzztime=20s
	go test ./pkg/core/ -fuzz=FuzzGeminiTOML -fuzztime=20s

cover:
	go test -coverprofile=$(BIN_DIR)/cover.out ./pkg/core/
	go tool cover -func=$(BIN_DIR)/cover.out

build-cli:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(CLI_BIN) ./cmd/skills

build-gui:
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(GUI_BIN) ./cmd/skills-gui

release-snapshot:
	goreleaser release --snapshot --clean --skip=publish,sign,sbom

clean:
	rm -rf $(BIN_DIR) dist/
