help:

build:
	rm -rf output/binary/
	mkdir -p output/binary/
	GOOS=linux  GOARCH=amd64 go build -o output/binary/anu_linux_amd64  -trimpath -ldflags '-s -w' .
	GOOS=linux  GOARCH=arm64 go build -o output/binary/anu_linux_arm64  -trimpath -ldflags '-s -w' .
	GOOS=darwin GOARCH=amd64 go build -o output/binary/anu_darwin_amd64 -trimpath -ldflags '-s -w' .
	GOOS=darwin GOARCH=arm64 go build -o output/binary/anu_darwin_arm64 -trimpath -ldflags '-s -w' .
