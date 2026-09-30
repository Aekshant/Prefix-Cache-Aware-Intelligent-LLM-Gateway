BINARY := bin/gateway
IMAGE  := llm-gateway:latest

.PHONY: build run test vet fmt tidy docker-build up down logs clean

build:
	go build -o $(BINARY) ./cmd/gateway

run:
	go run ./cmd/gateway

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

docker-build:
	docker build -t $(IMAGE) .

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f gateway

clean:
	rm -rf bin
