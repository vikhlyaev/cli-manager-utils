# cli-manager — сборка и запуск
# ─────────────────────────────────────────────

PKG := ./cmd/cli-manager

.DEFAULT_GOAL := run

## run: запустить приложение (регенерирует generated.zsh на старте)
.PHONY: run
run:
	go run $(PKG)

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

## help: показать список целей
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
