BINARY := holocron
PKG     := ./cmd/holocron
DIST    := dist

# Stamped into the binary so the app can tell whether a newer release exists.
VERSION  ?= $(shell git describe --tags --exact-match 2>/dev/null)
LDFLAGS  = -X github.com/cristian/holocron/internal/version.Version=$(VERSION)

# templ is pinned as a module tool (see go.mod), so no global install is needed.
TEMPL := go tool templ

.PHONY: generate build build-linux build-pi run test lint vet vulncheck tidy check clean deploy release \
	ios-project ios-build ios-test

IOS_DIR         := ios
IOS_SCHEME      := Holocron
IOS_DESTINATION := platform=iOS Simulator,name=iPhone 17

## generate: regenerate *_templ.go from .templ files
generate:
	$(TEMPL) generate

## build: build for the local machine (quick check)
build: generate
	go build -o $(DIST)/$(BINARY) $(PKG)

# The server is Ginebra (x86-64) now; the Pi is kept as a fallback, so both
# targets stay buildable. Override with GOARCH=arm64.
GOARCH ?= amd64

## build-linux: cross-compile a static linux binary (GOARCH=amd64 by default, or arm64)
build-linux: generate
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -trimpath \
		-ldflags="-s -w $(if $(VERSION),$(LDFLAGS),)" -o $(DIST)/$(BINARY)-$(GOARCH) $(PKG)

## build-pi: kept for the fallback Pi (same as build-linux GOARCH=arm64)
build-pi:
	$(MAKE) build-linux GOARCH=arm64

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

## sast: static application security testing
sast:
	go run github.com/securego/gosec/v2/cmd/gosec@latest -quiet ./...

## check: full quality gate before shipping a binary
check: vet lint test vulncheck sast

## clean: remove build output
clean:
	rm -rf $(DIST)

## ios-install: build Release and install on a connected iPhone
## (usage: make ios-install IOS_TEAM=XXXXXXXXXX IOS_DEVICE=00008150-...)
## Neither value is hardcoded: this repository is public, and an Apple team id
## and a device udid identify the account and the phone.
ios-install: ios-project
	@test -n "$(IOS_TEAM)" || (echo "set IOS_TEAM=<team id>  (security find-identity -v -p codesigning)" && exit 1)
	@test -n "$(IOS_DEVICE)" || (echo "set IOS_DEVICE=<udid>  (xcrun xctrace list devices)" && exit 1)
	cd ios && xcodebuild -project Holocron.xcodeproj -scheme Holocron -configuration Release \
		-destination "id=$(IOS_DEVICE)" -derivedDataPath $(CURDIR)/$(DIST)/ios \
		DEVELOPMENT_TEAM=$(IOS_TEAM) CODE_SIGN_STYLE=Automatic -allowProvisioningUpdates build
	xcrun devicectl device install app --device "$(IOS_DEVICE)" \
		$(DIST)/ios/Build/Products/Release-iphoneos/Holocron.app

## deploy: build and copy the binary to the server (usage: make deploy HOST=ginebra [GOARCH=arm64])
HOST ?= $(PI)
deploy: build-linux
	@test -n "$(HOST)" || (echo "set HOST=user@host" && exit 1)
	scp $(DIST)/$(BINARY)-$(GOARCH) $(HOST):/tmp/holocron
	@echo "Copied. On the server: sudo install -m 0755 /tmp/holocron /usr/local/bin/holocron && sudo systemctl restart holocron"

## ios-project: regenerate the Xcode project from ios/project.yml
ios-project:
	@command -v xcodegen >/dev/null || (echo "missing xcodegen (brew install xcodegen)" && exit 1)
	cd $(IOS_DIR) && xcodegen generate

## ios-build: build the iOS app for the simulator
ios-build: ios-project
	cd $(IOS_DIR) && xcodebuild -project $(IOS_SCHEME).xcodeproj -scheme $(IOS_SCHEME) \
		-destination '$(IOS_DESTINATION)' build

## ios-test: run the iOS unit tests (API contract + helpers) on the simulator
ios-test: ios-project
	cd $(IOS_DIR) && xcodebuild -project $(IOS_SCHEME).xcodeproj -scheme $(IOS_SCHEME) \
		-destination '$(IOS_DESTINATION)' test

## release: prefer pushing a tag (CI publishes both architectures); this is the manual fallback
release:
	@test -n "$(VERSION)" || (echo "set VERSION=vX.Y.Z" && exit 1)
	@command -v gh >/dev/null || (echo "missing 'gh' CLI (https://cli.github.com/)" && exit 1)
	$(MAKE) build-linux GOARCH=amd64
	$(MAKE) build-linux GOARCH=arm64
	for a in amd64 arm64; do cp $(DIST)/$(BINARY)-$$a $(DIST)/$(BINARY)-linux-$$a; \
		(cd $(DIST) && shasum -a 256 $(BINARY)-linux-$$a > $(BINARY)-linux-$$a.sha256); done
	gh release create $(VERSION) \
		$(DIST)/$(BINARY)-linux-amd64 $(DIST)/$(BINARY)-linux-amd64.sha256 \
		$(DIST)/$(BINARY)-linux-arm64 $(DIST)/$(BINARY)-linux-arm64.sha256 \
		--title "Holocron $(VERSION)" --generate-notes
