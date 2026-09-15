TEMPL = go run github.com/a-h/templ/cmd/templ@v0.3.1020

.PHONY: generate build run test vet tidy

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
