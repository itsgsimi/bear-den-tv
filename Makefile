# Bear Den TV build entry points. Every target here exists and runs; see `make help`.
SHELL := /bin/bash
BDTV_TOOLCHAIN ?= $(HOME)/.bdtv-toolchain
export PATH := $(BDTV_TOOLCHAIN)/env/bin:$(PATH)
export CMAKE_PREFIX_PATH := $(BDTV_TOOLCHAIN)/env
export GOTOOLCHAIN := local
BUILD ?= build
# Our Go packages: `./...` also walks into apps/*/node_modules, where npm
# packages may ship Go code (flatted/golang) that is not ours to test or vet.
# (make 4.3 does not pass the exported PATH to $(shell), hence PATH=.)
GO_PKGS = $(shell PATH="$(PATH)" GOTOOLCHAIN=local go list ./... | grep -v '/node_modules/')

.PHONY: help deps-check build go web check-web-dist webnav test-webnav shell shell-target package test test-go test-web test-shell lint shots perf dev doctor clean

help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-14s %s\n", $$1, $$2}'

deps-check: ## Verify the pinned toolchain is present
	@go version && cmake --version | head -1 && node --version && qmake6 -query QT_VERSION 2>/dev/null || (echo "run scripts/bootstrap-toolchain.sh" && exit 1)

build: web webnav go shell ## Build everything into $(BUILD)/

go: ## Build the coordinator/CLI binaries
	mkdir -p $(BUILD)/bin
	go build -trimpath -o $(BUILD)/bin/ ./cmd/...

# The phone remote's npm packages, installed on first use and again when the
# lock file changes (a fresh clone needs no manual npm step).
apps/remote-web/node_modules/.installed: apps/remote-web/package-lock.json
	cd apps/remote-web && npm ci --no-audit --no-fund
	touch $@

web: apps/remote-web/node_modules/.installed ## Build the phone remote into apps/remote-web/dist (embedded by go)
	cd apps/remote-web && npm run build

# The navigation script injected into web apps (apps/web-nav), its own npm
# package; dist/nav.js is committed and embedded by go like the remote.
apps/web-nav/node_modules/.installed: apps/web-nav/package-lock.json
	cd apps/web-nav && npm ci --no-audit --no-fund
	touch $@

webnav: apps/web-nav/node_modules/.installed ## Build the web apps navigation script into apps/web-nav/dist (embedded by go)
	cd apps/web-nav && npm run build

check-web-dist: ## Fail if committed apps/remote-web/dist or apps/web-nav/dist differs from a fresh build (CI runs this)
	scripts/check-web-dist.sh

# The shell is installed by copy + rename: overwriting a running executable in
# place fails ("Text file busy") and would silently leave the old binary.
shell: ## Build the Qt Quick TV shell
	cmake -S apps/tv-shell -B $(BUILD)/tv-shell -G Ninja -DCMAKE_BUILD_TYPE=Release -DBDTV_BUILD_TESTS=OFF
	cmake --build $(BUILD)/tv-shell
	mkdir -p $(BUILD)/bin && cp $(BUILD)/tv-shell/bear-den-tv-shell $(BUILD)/bin/.bear-den-tv-shell.new && mv -f $(BUILD)/bin/.bear-den-tv-shell.new $(BUILD)/bin/bear-den-tv-shell

# The TV machine's shell, built here in seconds instead of minutes on the TV:
# conda's compiler against a glibc 2.28 sysroot, so it runs on the TV's older
# glibc; Qt resolves from the same pinned toolchain path there.
TARGET_SYSROOT := $(BDTV_TOOLCHAIN)/sysroot-2.28/x86_64-conda-linux-gnu/sysroot
# Where the toolchain (Qt) lives on the TV machine: the shell's rpath points
# there first, not at this workstation's toolchain. deploy-target.sh passes the TV's
# own path (BDTV_TARGET_TOOLCHAIN in target.env, else ~/.bdtv-toolchain there).
# conda's gcc also appends this workstation's env/lib; ours comes first, so the
# TV loads Qt from its own toolchain and the extra entry is only a miss.
TARGET_TOOLCHAIN ?= $(BDTV_TOOLCHAIN)

shell-target: ## Build the TV shell for the TV machine (glibc 2.28 baseline) into $(BUILD)/tv-shell-target
	@test -d $(TARGET_SYSROOT) || (echo "missing $(TARGET_SYSROOT): run scripts/bootstrap-toolchain.sh" && exit 1)
	CC=x86_64-conda-linux-gnu-cc CXX=x86_64-conda-linux-gnu-c++ cmake -S apps/tv-shell -B $(BUILD)/tv-shell-target -G Ninja \
	  -DCMAKE_BUILD_TYPE=Release -DBDTV_BUILD_TESTS=OFF \
	  -DCMAKE_C_FLAGS="--sysroot=$(TARGET_SYSROOT)" -DCMAKE_CXX_FLAGS="--sysroot=$(TARGET_SYSROOT)" \
	  -DCMAKE_SKIP_BUILD_RPATH=ON \
	  -DCMAKE_EXE_LINKER_FLAGS="--sysroot=$(TARGET_SYSROOT) -Wl,--disable-new-dtags -Wl,-rpath,$(TARGET_TOOLCHAIN)/env/lib" >/dev/null
	cmake --build $(BUILD)/tv-shell-target

# The installable .deb: static coordinator, the shell built against the glibc
# 2.28 sysroot without a toolchain rpath, Qt bundled under /opt/bear-den-tv
# (packaging/build-deb.sh, packaging/nfpm.yaml; docs/operations.md#packaging).
package: ## Build the installable .deb into build/dist (packaging/build-deb.sh; smoke test: packaging/smoke-deb.sh)
	packaging/build-deb.sh

test: test-go test-web test-webnav test-shell ## Run all automated tests

test-go: ## Go unit + contract tests with the race detector
	@test -n "$(GO_PKGS)" || (echo "go list found no packages" && exit 1)
	go test -race -count=1 $(GO_PKGS)

# The browser tests start the real coordinator (make go) and need Playwright's
# Chromium once (the same version as apps/web-nav: see test-webnav).
test-web: apps/remote-web/node_modules/.installed go ## Phone remote: unit + contract tests (Vitest), then browser tests against `bear-den-tv dev` (Playwright)
	cd apps/remote-web && BDTV_BIN=$(abspath $(BUILD)/bin/bear-den-tv) npm test

# Needs Playwright's Chromium once: cd apps/web-nav && npx playwright install chromium
test-webnav: webnav ## Navigation script browser tests (headless Playwright Chromium, local fixture pages only)
	cd apps/web-nav && npm test

test-shell: ## QML/C++ shell tests (offscreen)
	cmake -S apps/tv-shell -B $(BUILD)/tv-shell -G Ninja -DCMAKE_BUILD_TYPE=Debug -DBDTV_BUILD_TESTS=ON
	cmake --build $(BUILD)/tv-shell
	cd $(BUILD)/tv-shell && QT_QPA_PLATFORM=offscreen ctest --output-on-failure

lint: apps/remote-web/node_modules/.installed apps/web-nav/node_modules/.installed ## gofmt/vet, eslint/tsc, qmllint (toolchain qmllint; configures the shell build dir if missing)
	test -z "$$(gofmt -l cmd internal tests embed.go | tee /dev/stderr)"
	@test -n "$(GO_PKGS)" || (echo "go list found no packages" && exit 1)
	go vet $(GO_PKGS)
	cd apps/remote-web && npm run lint
	cd apps/web-nav && npm run lint
	@test -f $(BUILD)/tv-shell/build.ninja || cmake -S apps/tv-shell -B $(BUILD)/tv-shell -G Ninja -DCMAKE_BUILD_TYPE=Debug -DBDTV_BUILD_TESTS=ON
	cmake --build $(BUILD)/tv-shell --target all_qmllint

DEV_ARGS ?=
dev: go shell ## Run coordinator + shell locally with a fake desktop, loopback remote (DEMO rows and weather: make dev DEV_ARGS=--dev-fixtures)
	$(BUILD)/bin/bear-den-tv dev --shell-binary $(BUILD)/bin/bear-den-tv-shell $(DEV_ARGS)

shots: shell ## Sandbox gallery: every theme × main screens as PNGs in build/shots/gallery (scripts/sandbox.sh)
	scripts/sandbox.sh gallery

perf: shell ## Performance sandbox: frames and CPU per phase of Home, offscreen on 2 cores (scripts/perf-sandbox.sh)
	scripts/perf-sandbox.sh

doctor: go ## Print diagnostics
	$(BUILD)/bin/bear-den-tv doctor

clean: ## Remove build outputs
	rm -rf $(BUILD) apps/tv-shell/build
