DB_URL ?= $(shell grep DATABASE_URL .env 2>/dev/null | cut -d= -f2-)
GOOSE   = go run github.com/pressly/goose/v3/cmd/goose@v3.28.0
TEMPL   = go run github.com/a-h/templ/cmd/templ@v0.3.1020

.PHONY: generate build run test vet tidy migrate-up migrate-down migrate-status

generate:
	$(TEMPL) generate

build: generate
	go build -o bin/server ./cmd/server

run: build
	./bin/server

test:
	go test ./...

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
