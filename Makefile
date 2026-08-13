BIN := jobkit
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: help build test test-race vet fmt license-check vulnerability-check verify verify-publication publish-ready claimguard-bridge install demo clean help-sizes

help:
	@echo "jobkit local targets:"
	@echo "  make build | test | test-race | vet | fmt"
	@echo "  make verify                 # test + vet + license-check"
	@echo "  make verify-publication     # verify + gitleaks (history + tree)"
	@echo "  make publish-ready          # publication + race + vulnerability + ClaimGuard fixtures"
	@echo "  make claimguard-bridge      # optional external claimguard dogfood"
	@echo "  make install | demo | clean"

build:
	go build -o bin/$(BIN) ./cmd/jobkit

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:" >&2; \
		echo "$$unformatted" >&2; \
		exit 1; \
	fi

license-check:
	go run ./tools/license-audit

vulnerability-check:
	go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

verify: fmt test vet license-check

verify-publication: verify
	gitleaks git --no-banner --redact .
	gitleaks dir --no-banner --redact .

# Local publication gate (no remote push). claimguard-bridge is optional skip-if-missing.
publish-ready: test-race verify-publication vulnerability-check claimguard-bridge

.PHONY: claimguard-bridge
claimguard-bridge:
	go run ./tools/claimguard-bridge

help-sizes: build
	@full=$$(./bin/jobkit help | wc -c | tr -d ' '); \
	compact=$$(./bin/jobkit help --compact | wc -c | tr -d ' '); \
	echo "full=$$full compact=$$compact"; \
	test "$$compact" -gt 0; \
	test "$$compact" -lt $$((full / 3))

install: build
	mkdir -p $(INSTALL_DIR)
	cp -f bin/$(BIN) $(INSTALL_DIR)/$(BIN)
	@echo "installed $(INSTALL_DIR)/$(BIN)"

# End-to-end demo against an isolated state dir (never touches ~/.jobkit).
demo: build
	@JOBKIT_HOME=$$(mktemp -d) ./scripts/demo.sh

clean:
	rm -rf bin
