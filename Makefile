DB_URL ?= $(shell grep DATABASE_URL .env 2>/dev/null | cut -d= -f2-)
GOOSE   = go run github.com/pressly/goose/v3/cmd/goose@v3.28.0
TEMPL   = go run github.com/a-h/templ/cmd/templ@v0.3.1020

# Tailwind's standalone CLI -- a single prebuilt binary that bundles its own runtime, so this
# stays a Node-free build (there is no `go run` equivalent for it, which is why this is the one
# tool fetched rather than compiled). Pinned and checksum-verified for the same reason CI pins
# goose: an unpinned build tool silently changes generated output. bin/ is gitignored, so the
# binary is a local artifact; app.css, the thing it produces, is what gets committed.
TAILWIND_VERSION = v4.3.3
TAILWIND_SHA256  = dc61b3ac6b8c9ca874c0cc4c57b2409791a64c5540404ca5f5367360babc313a
TAILWIND         = bin/tailwindcss

.PHONY: generate css build run test conformance threshold vet tidy migrate-up migrate-down migrate-status check-generated install-hooks

generate: css
	$(TEMPL) generate

# Tailwind scans the .templ sources named by static/css/input.css's own @source directive, so
# this deliberately runs BEFORE templ generate is required -- the class names live in the .templ
# files, not in the generated Go.
css: $(TAILWIND)
	$(TAILWIND) -i static/css/input.css -o static/css/app.css --minify

$(TAILWIND):
	@mkdir -p bin
	curl -sSfL -o $(TAILWIND).tmp \
		https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-linux-x64
	echo "$(TAILWIND_SHA256)  $(TAILWIND).tmp" | sha256sum -c -
	chmod +x $(TAILWIND).tmp
	mv $(TAILWIND).tmp $(TAILWIND)

build: generate
	go build -o bin/server ./cmd/server

run: build
	./bin/server

test:
	go test -race ./...

# internal/conformance's own executable checks for architectural obligations 001-007 and
# CLAUDE.md state in prose (plane boundaries, handler size, and -- since this session -- that
# Workspace Home stays metadata-driven rather than hand-typing an Application route). No database,
# no race flag: fast enough for the pre-commit hook, not just pre-push.
conformance:
	go test ./internal/conformance/...

# Fails if a .templ file was edited without re-running `templ generate` before committing, or if
# a class was added to a .templ without rebuilding static/css/app.css. Both are generated-and-
# committed artifacts, so the same staleness check covers them: committing app.css is what keeps
# `go build ./cmd/server` working with neither Node nor the Tailwind binary present.
check-generated: generate
	git diff --exit-code -- '*_templ.go' static/css/app.css

# One-time setup: makes git use the tracked hooks in .githooks/ (pre-push gate) instead of
# the untracked, per-clone .git/hooks/.
install-hooks:
	git config core.hooksPath .githooks

# Deliberately constructs the forcing conditions this app cannot wait for, because it has no real
# users (ROADMAP.md Phase 18 Step 4). Needs a real database and seeds tens of thousands of
# mch_bench_* rows, which it deletes afterwards -- it never touches application records. Kept out
# of `make test` so the default suite stays fast and needs no database.
#
# It also REINDEXes `records` on the way out (cleanupBench). Deleting the seeded rows does not
# give back the index pages they cost: autovacuum marks a bloated btree reusable, never smaller.
# Without that step each run left the indexes permanently larger -- measured at 21 MB of indexes
# over a 72 kB table before it was added (menata-app-document,
# audits/2026-09-22-lapisan-query-dan-indeks-kajian.md).
threshold:
	DATABASE_URL="$(DB_URL)" go test -tags=threshold -run Threshold -v -timeout 10m ./internal/composition/

vet:
	go vet ./...

tidy:
	go mod tidy

migrate-up:
	$(GOOSE) -dir migrations postgres "$(DB_URL)" up

migrate-down:
	$(GOOSE) -dir migrations postgres "$(DB_URL)" down

migrate-status:
	$(GOOSE) -dir migrations postgres "$(DB_URL)" status
