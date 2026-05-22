VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)
BINARY  := data-analyzer

# macOS Developer ID signing / notarization (see nlink-jp/.github
# CONVENTIONS.md §Code Signing). Defaults match any Developer ID
# Application cert in the keychain and the org-standard notary
# profile. Builds without these fall back to ad-hoc / un-notarized
# with a one-line warning — see scripts/codesign-darwin.sh.
CODESIGN_IDENTITY ?= Developer ID Application
NOTARY_PROFILE    ?= nlink-jp-notary

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build build-all package test clean

build:
	@mkdir -p dist
	go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY) .
	@scripts/codesign-darwin.sh dist/$(BINARY) "$(CODESIGN_IDENTITY)"

build-all:
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" \
			-o dist/$(BINARY)-$$os-$$arch$$ext . || exit 1; \
		scripts/codesign-darwin.sh "dist/$(BINARY)-$$os-$$arch$$ext" "$(CODESIGN_IDENTITY)"; \
	done

## package: Cross-compile, sign, zip (versioned + LICENSE + README), notarize darwin → dist/
package: build-all
	@cd dist && for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		cp ../LICENSE ../README.md .; \
		zip -j "$(BINARY)-$(VERSION)-$$os-$$arch.zip" "$(BINARY)-$$os-$$arch$$ext" LICENSE README.md; \
		rm -f LICENSE README.md; \
	done
	@scripts/notarize-darwin.sh dist/$(BINARY)-$(VERSION)-darwin-amd64.zip "$(NOTARY_PROFILE)"
	@scripts/notarize-darwin.sh dist/$(BINARY)-$(VERSION)-darwin-arm64.zip "$(NOTARY_PROFILE)"

test:
	go test ./...

clean:
	rm -rf dist
