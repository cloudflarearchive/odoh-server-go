all: test build

test:
	go test ./...

build:
	go build -o odoh-server
