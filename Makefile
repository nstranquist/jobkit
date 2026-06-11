BIN := jobkit
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: build test vet install demo clean

build:
	go build -o bin/$(BIN) ./cmd/jobkit

test:
	go test ./...

vet:
	go vet ./...

install: build
	mkdir -p $(INSTALL_DIR)
	cp -f bin/$(BIN) $(INSTALL_DIR)/$(BIN)
	@echo "installed $(INSTALL_DIR)/$(BIN)"

# End-to-end demo against an isolated state dir (never touches ~/.jobkit).
demo: build
	@JOBKIT_HOME=$$(mktemp -d) ./scripts/demo.sh

clean:
	rm -rf bin
