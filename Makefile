all:
	go build -o dumpevtx ./cmd/

test:
	go test ./...

windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -o dumpevtx.exe cmd/*.go
