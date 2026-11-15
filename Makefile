# healthbot — Makefile для локальной разработки и CI.
# Версии инструментов зафиксированы для воспроизводимости (см. ТЗ §8).
# Запуск см. `make help`.

GOLANGCI_LINT_VERSION := v2.15.0
GOBIN ?= $(CURDIR)/.bin
GOLANGCI_LINT := $(GOBIN)/golangci-lint
COVERAGE_FILE := coverage.out

.PHONY: help all check e2e fmt fmt-check vet lint test cover build build-linux run dry-run ask image checkin vuln tidy install-tools clean

help: ## показать список целей
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

all: check build ## все проверки + сборка

check: fmt-check vet lint test ## все проверки качества (как в CI)

fmt: $(GOLANGCI_LINT) ## форматировать код (gofmt + gofumpt + goimports)
	$(GOLANGCI_LINT) fmt

fmt-check: $(GOLANGCI_LINT) ## проверить форматирование без правок
	$(GOLANGCI_LINT) fmt --diff

vet: ## go vet
	go vet ./...

lint: $(GOLANGCI_LINT) ## golangci-lint по .golangci.yml (0 предупреждений — блокер)
	$(GOLANGCI_LINT) run

test: ## тесты с race detector
	go test -race -count=1 ./...

e2e: ## E2E-набор команд/форм через роутер и временный DATA_ROOT
	go test -count=1 -v -run 'TestE2E' ./internal/app

cover: ## покрытие тестами
	go test -coverprofile=$(COVERAGE_FILE) ./... && go tool cover -func=$(COVERAGE_FILE)

build: ## собрать бинарь в bin/healthbot
	go build -o bin/healthbot ./cmd/healthbot

GOARCH ?= amd64

build-linux: ## кросс-сборка статического бинаря для VPS (GOARCH=amd64|arm64)
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o bin/healthbot-linux-$(GOARCH) ./cmd/healthbot

run: ## запустить бота (long polling; нужен .env с токеном)
	go run ./cmd/healthbot

dry-run: ## самопроверка без Telegram: конфиг + обработчик /help
	go run ./cmd/healthbot -dry-run

Q ?= Ответь одним предложением по-русски: кто ты?
ask: ## спросить LLM без Telegram: make ask Q="Какой HRV в последнем чек-ине?"
	go run ./cmd/healthbot -ask "$(Q)"

IMG ?=
image: ## распознать скриншот(ы): make image IMG="path[,path2]"
	go run ./cmd/healthbot -image "$(IMG)"

TEXT ?= сон 7:33, HRV 38, пульс 58, настроение 7
checkin: ## распознать текстовый чек-ин: make checkin TEXT="..."
	go run ./cmd/healthbot -checkin "$(TEXT)"

vuln: ## govulncheck по зависимостям
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

tidy: ## go mod tidy
	go mod tidy

install-tools: $(GOLANGCI_LINT) ## поставить зафиксированные инструменты в .bin/

$(GOLANGCI_LINT):
	GOBIN=$(GOBIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

clean: ## удалить артефакты сборки (bin/coverage)
	rm -rf bin $(COVERAGE_FILE)

doctor: ## диагностика диска и кэшей (Go/репозиторий)
	@echo "== диск =="
	@df -h . | tail -1
	@echo "== .sandbox-cache =="
	@du -sh .sandbox-cache 2>/dev/null || true
	@du -sh .sandbox-cache/gocache .sandbox-cache/gopath/pkg/mod .sandbox-cache/golangci-lint 2>/dev/null || true
	@echo "== глобальные кэши =="
	@du -sh $$HOME/go/pkg/mod $$HOME/.cache/go-build 2>/dev/null || true

clean-cache: ## безопасно очистить Go/линтер-кэши репозитория (данные не трогает)
	@echo "Чищу производные кэши репозитория…"
	@rm -rf .sandbox-cache/gocache .sandbox-cache/gopath/pkg/mod
	@rm -f .sandbox-cache/coverage.out .sandbox-cache/lint.out .sandbox-cache/racecheck \
		.sandbox-cache/fmt.out .sandbox-cache/cover.log .sandbox-cache/cover.txt
	@echo "Готово. Первый make check будет дольше; данные и .sandbox-cache/env.sh не тронуты."
