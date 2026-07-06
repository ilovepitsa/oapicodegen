.PHONY: build test vet fmt lint generate e2e golden-check clean tidy cover cover-html cover-html-open cover-func

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
	golangci-lint run

# generate — перегенерирует golden-файлы (petstore + minimal e2e) с флагом -update.
generate:
	$(GO) test ./internal/generator/ -run TestGenerate -update
	$(GO) test ./cmd/oapigen/ -run TestE2E_Minimal -update

# e2e — запускает e2e-тест генерации (полный пайплайн cmd/oapigen).
e2e:
	$(GO) test ./cmd/oapigen/ -run TestE2E_Minimal

# golden-check — верифицирует, что golden-файлы актуальны (без -update).
# Используется в CI: падает, если вывод генератора разошёлся с эталоном.
golden-check:
	$(GO) test ./internal/generator/ -run TestGenerate
	$(GO) test ./cmd/oapigen/ -run TestE2E_Minimal

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
