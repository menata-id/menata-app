.PHONY: build run test vet tidy

build:
	go build -o bin/server ./cmd/server

run: build
	./bin/server

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy
