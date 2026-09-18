DB_URL ?= $(shell grep DATABASE_URL .env 2>/dev/null | cut -d= -f2-)
GOOSE   = go run github.com/pressly/goose/v3/cmd/goose@v3.28.0
TEMPL   = go run github.com/a-h/templ/cmd/templ@v0.3.1020

.PHONY: generate build run test threshold vet tidy migrate-up migrate-down migrate-status

generate:
	$(TEMPL) generate

build: generate
	go build -o bin/server ./cmd/server

run: build
	./bin/server

test:
	go test ./...

# Deliberately constructs the forcing conditions this app cannot wait for, because it has no real
# users (ROADMAP.md Phase 18 Step 4). Needs a real database and seeds tens of thousands of
# mch_bench_* rows, which it deletes afterwards -- it never touches application records. Kept out
# of `make test` so the default suite stays fast and needs no database.
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
