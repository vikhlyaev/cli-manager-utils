# cli-manager — сборка и запуск
# ─────────────────────────────────────────────

PKG     := ./cmd/cli-manager
BIN     := bin/cli-manager
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X cli-manager/internal/buildinfo.Version=$(VERSION)

.DEFAULT_GOAL := run

## run: запустить приложение (регенерирует скрипт на старте)
.PHONY: run
run:
	go run -ldflags "$(LDFLAGS)" $(PKG)

## ui: запустить веб-интерфейс (аргументы — через UI_ARGS, напр. UI_ARGS="--port 8080 --no-open")
.PHONY: ui
ui:
	go run -ldflags "$(LDFLAGS)" $(PKG) ui $(UI_ARGS)

## build: собрать бинарник в bin/ (веб-интерфейс встроен через go:embed)
.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

## install: установить бинарник в $(go env GOPATH)/bin
.PHONY: install
install:
	go install -ldflags "$(LDFLAGS)" $(PKG)

## test: прогнать все тесты
.PHONY: test
test:
	go test ./...

## fmt: отформатировать код
.PHONY: fmt
fmt:
	go fmt ./...

## vet: статический анализ
.PHONY: vet
vet:
	go vet ./...

## clean: удалить собранные файлы
.PHONY: clean
clean:
	rm -rf bin

## help: показать список целей
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
