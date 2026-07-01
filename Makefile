.PHONY: build test vet fmt lint generate e2e clean tidy cover cover-html cover-html-open cover-func

GO ?= go
PKG := ./...
BIN := oapigen
COVER_OUT := coverage.txt
COVER_HTML := coverage.html

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

# cover — генерирует текстовый отчёт покрытия (coverage.out-формат) и печатает summary.
cover:
	$(GO) test $(PKG) -coverprofile=$(COVER_OUT)
	$(GO) tool cover -func=$(COVER_OUT) | tail -1

# cover-html — собирает HTML-отчёт покрытия.
cover-html:
	$(GO) test $(PKG) -coverprofile=$(COVER_OUT)
	$(GO) tool cover -html=$(COVER_OUT) -o $(COVER_HTML)
	@echo "HTML coverage report: $(COVER_HTML)"

# cover-html-open — собирает и открывает HTML-отчёт в браузере.
cover-html-open:
	$(GO) test $(PKG) -coverprofile=$(COVER_OUT)
	$(GO) tool cover -html=$(COVER_OUT)

# cover-func — построчный per-function отчёт покрытия.
cover-func:
	$(GO) test $(PKG) -coverprofile=$(COVER_OUT)
	$(GO) tool cover -func=$(COVER_OUT)

clean:
	rm -f $(BIN) $(COVER_OUT) $(COVER_HTML)
