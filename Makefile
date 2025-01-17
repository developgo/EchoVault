build-server:
	 CC=$(CC) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(DEST)/server ./cmd/main.go

build:
	env CC=x86_64-linux-musl-gcc GOOS=linux GOARCH=amd64 DEST=bin/linux/x86_64 make build-server

run:
	make build && docker-compose up --build

test:
	go clean -testcache && go test ./... -coverprofile coverage/coverage.out

test-race:
	go clean -testcache && go test ./... --race