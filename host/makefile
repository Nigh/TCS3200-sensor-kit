.PHONY: run demo build test
run:
	go run .
demo:
	go run . --demo
build:
	mkdir -p build
	CGO_ENABLED=0 go build -trimpath -tags mygo_noinspector -ldflags '-s -w' -o build/tcs3200-host .
test:
	go test ./...
	go vet ./...
