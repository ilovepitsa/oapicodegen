.PHONY: build test vet fmt lint generate e2e clean tidy

GO ?= go
PKG := ./...
BIN := oapigen

build:
	$(GO) build $(PKG)

test:
	$(GO) test $(PKG)

vet:
	$(GO) vet $(PKG)

fmt:
	gofmt -s -w .

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, skipping"

generate:
	@echo "target placeholder: run generator on testdata"

e2e:
	$(GO) test -tags=e2e ./...

tidy:
	$(GO) mod tidy

clean:
	rm -f $(BIN) coverage.txt
