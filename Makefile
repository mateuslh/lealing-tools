GO ?= go
VERSION ?= 1.0.0
ENGINE_VERSION ?= v0.2.4-0.20260731203802-a61cafc5f910
BIN_DIR ?= bin

.PHONY: fmt vet test race cross manifest marketplace build clean

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

cross:
	@for target in darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do \
		os=$${target%/*}; arch=$${target#*/}; \
		printf '%-16s ' $$target; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GO) build ./... && echo ok || exit 1; \
	done

manifest:
	$(GO) run github.com/mateuslh/lealing/cmd/lealing@$(ENGINE_VERSION) \
		-tool-validate manifests/token-usage.yaml

marketplace:
	$(GO) run ./cmd/marketplace-index -check

build: manifest
	@mkdir -p $(BIN_DIR)
	@case "$$(go env GOOS)" in windows) suffix=.exe ;; *) suffix= ;; esac; \
		$(GO) build -trimpath -ldflags '-s -w -X main.version=$(VERSION)' \
		-o "$(BIN_DIR)/lealing-tool-token-usage$$suffix" ./cmd/token-usage
	cp manifests/token-usage.yaml $(BIN_DIR)/manifest.yaml

clean:
	rm -rf $(BIN_DIR) dist coverage.out
