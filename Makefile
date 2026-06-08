BINARY  := lastro
OUTDIR  := bin
GOFLAGS := -trimpath -ldflags="-s -w"
PKG     := ./cmd/harness-tools/

PLATFORMS := darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64

.PHONY: build-all test clean $(PLATFORMS)

build-all: $(PLATFORMS)

darwin-arm64:
	GOOS=darwin  GOARCH=arm64 go build $(GOFLAGS) -o $(OUTDIR)/darwin-arm64/$(BINARY)      $(PKG)

darwin-amd64:
	GOOS=darwin  GOARCH=amd64 go build $(GOFLAGS) -o $(OUTDIR)/darwin-amd64/$(BINARY)      $(PKG)

linux-amd64:
	GOOS=linux   GOARCH=amd64 go build $(GOFLAGS) -o $(OUTDIR)/linux-amd64/$(BINARY)       $(PKG)

linux-arm64:
	GOOS=linux   GOARCH=arm64 go build $(GOFLAGS) -o $(OUTDIR)/linux-arm64/$(BINARY)       $(PKG)

windows-amd64:
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o $(OUTDIR)/windows-amd64/$(BINARY).exe $(PKG)

test:
	go test ./...

clean:
	rm -rf $(OUTDIR)/darwin-* $(OUTDIR)/linux-* $(OUTDIR)/windows-*
