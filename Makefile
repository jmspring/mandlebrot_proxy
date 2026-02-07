.PHONY: build test run clean

BIN := mandelbrot-auth-proxy

build:
	go build -o $(BIN) .

test:
	go test -race -count=1 ./...

test-all: test
	TEST_DOCKER=1 go test -race -count=1 -run TestDockerLifecycle ./...

cover:
	go test -coverprofile=cover.out ./...
	go tool cover -html=cover.out -o cover.html

run: build
	./$(BIN)

clean:
	rm -f $(BIN) cover.out cover.html

lint:
	go vet ./...
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run || true
