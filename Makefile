GO ?= go
VERSION ?= 1.0.0
ENGINE_VERSION ?= v0.4.0
# Versão de github.com/mateuslh/lealing-sdk usada por `make marketplace`, que
# roda o cmd/marketplace-index publicado nesse repositório independente.
SDK_VERSION ?= v0.1.0
BIN_DIR ?= bin

TOOLS := token-usage system-info power-control claude-accounts \
	http-probe network-inspector json-lab jwt-inspector cidr-calculator \
	codec-lab checksum-lab uuid-generator
DEVKIT := http-probe network-inspector json-lab jwt-inspector cidr-calculator \
	codec-lab checksum-lab uuid-generator

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
	@for tool in $(TOOLS); do \
		$(GO) run github.com/mateuslh/lealing/cmd/lealing@$(ENGINE_VERSION) \
			-tool-validate manifests/$$tool.yaml || exit 1; \
	done

marketplace:
	$(GO) run github.com/mateuslh/lealing-sdk/cmd/marketplace-index@$(SDK_VERSION) -check

build: manifest
	@mkdir -p $(BIN_DIR)
	@case "$$(go env GOOS)" in windows) suffix=.exe ;; *) suffix= ;; esac; \
	for tool in $(TOOLS); do \
		command=$$tool; \
		case " $(DEVKIT) " in *" $$tool "*) command=devkit ;; esac; \
		mkdir -p "$(BIN_DIR)/$$tool"; \
		$(GO) build -trimpath -ldflags '-s -w -X main.version=$(VERSION)' \
			-o "$(BIN_DIR)/$$tool/lealing-tool-$$tool$$suffix" ./cmd/$$command || exit 1; \
		cp manifests/$$tool.yaml "$(BIN_DIR)/$$tool/manifest.yaml" || exit 1; \
	done

clean:
	rm -rf $(BIN_DIR) dist coverage.out
