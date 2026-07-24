BINARY := holocron
PKG     := ./cmd/holocron
DIST    := dist

# templ is pinned as a module tool (see go.mod), so no global install is needed.
TEMPL := go tool templ

.PHONY: generate build build-pi run test lint vet vulncheck tidy check clean deploy release

## generate: regenerate *_templ.go from .templ files
generate:
	$(TEMPL) generate

## build: build for the local machine (quick check)
build: generate
	go build -o $(DIST)/$(BINARY) $(PKG)

## build-pi: cross-compile a static arm64 binary for the Raspberry Pi
build-pi: generate
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o $(DIST)/$(BINARY)-arm64 $(PKG)

## run: run locally
run: generate
	go run $(PKG)

## test: run tests with the race detector and randomised order
test:
	go test -race -shuffle=on ./...

## lint: run golangci-lint
lint:
	golangci-lint run

## vet: run go vet
vet:
	go vet ./...

## vulncheck: scan for known vulnerabilities
vulncheck:
	govulncheck ./...

## tidy: tidy modules and fail if anything changed
tidy:
	go mod tidy && git diff --exit-code go.mod go.sum

## check: full quality gate before shipping a binary
check: vet lint test vulncheck

## clean: remove build output
clean:
	rm -rf $(DIST)

## deploy: build the arm64 binary and copy it to the Pi (usage: make deploy PI=user@host)
deploy: build-pi
	@test -n "$(PI)" || (echo "set PI=user@host" && exit 1)
	scp $(DIST)/$(BINARY)-arm64 $(PI):/tmp/holocron
	@echo "Copied. On the Pi: sudo mv /tmp/holocron /usr/local/bin/holocron && sudo systemctl restart holocron"

## release: cross-compile arm64, checksum it and publish a GitHub release (usage: make release VERSION=v0.1.0)
release: build-pi
	@test -n "$(VERSION)" || (echo "set VERSION=vX.Y.Z" && exit 1)
	@command -v gh >/dev/null || (echo "missing 'gh' CLI (https://cli.github.com/)" && exit 1)
	cp $(DIST)/$(BINARY)-arm64 $(DIST)/$(BINARY)-linux-arm64
	cd $(DIST) && shasum -a 256 $(BINARY)-linux-arm64 > $(BINARY)-linux-arm64.sha256
	gh release create $(VERSION) \
		$(DIST)/$(BINARY)-linux-arm64 $(DIST)/$(BINARY)-linux-arm64.sha256 \
		--title "Holocron $(VERSION)" --generate-notes
