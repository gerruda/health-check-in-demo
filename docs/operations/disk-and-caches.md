# Диск и кэши: регламент очистки

**Зачем:** Go-кэши, модули, toolchains, линтеры и браузерные кэши растут при
каждом прогоне тестов/сборок и **сами не чистятся**. Однажды это уже привело к
`no space left on device`.

## Быстрая диагностика

```bash
make doctor
```

Показывает `df`, размер `.sandbox-cache`, крупные глобальные кэши. Тревога —
**меньше 10 ГБ свободно**; при 1–2 ГБ Go-сборки уже могут падать.

Что считается «нормой»:

- данные и код проекта — единицы мегабайт (`данные_*`, `база_знаний`, `.git`);
- `.sandbox-cache` — 1–4 ГБ (в основном `gocache` и `gopath/pkg/mod`);
- `~/go/pkg/mod` — глобальный Go-кэш, может быть несколько ГБ (toolchains, модули);
- браузерные/IDE-кэши (`ms-playwright`, `JetBrains`, Chrome AI) — не бот.

## Безопасная очистка (репозиторий)

```bash
make clean-cache
```

Удаляет только производные артефакты внутри репозитория:

```text
.sandbox-cache/gocache          # объектники/тест-бинарники
.sandbox-cache/gopath/pkg/mod   # скачанные модули (потом скачаются снова)
.sandbox-cache/coverage.out     # покрытие
.sandbox-cache/lint.out, racecheck, cover.*
```

**Не удаляет:** `.sandbox-cache/env.sh`, `данные_*`, `база_знаний`, код, `.git`,
настройки, логи. После очистки первый `make check` будет дольше (пересборка +
скачивание модулей), дальше — снова быстро.

## Глобальные кэши (вне репозитория)

Эти команды не входят в `make clean-cache`, потому что общие для других
проектов; выполняй осознанно:

```bash
# Go build cache пользователя (безопасно, пересоберётся)
rm -rf ~/.cache/go-build

# Глобальный module cache (удаляет модули/тулчейны всех Go-проектов;
# они скачаются заново)
go clean -modcache
```

Дополнительно можно освободить:

- старые partial-блобы Ollama: `~/.ollama/models/blobs/*-partial` (только если
  не идёт `ollama pull`);
- дубли архивов IDE (например `goland-*.tar.gz`), `journalctl --vacuum-size=200M`;
- браузерные кэши (`~/.cache/ms-playwright`) — если не нужен browser-tooling;
- кэши IDE (`~/.cache/JetBrains`) — лучше через саму IDE.

## Периодичность

- **После каждой крупной серии прогонов** (`make check` / `-race` / coverage /
  `govulncheck` в нескольких ветках) — смотреть `make doctor`.
- **Раз в 1–2 недели** или при свободном месте < 15 ГБ — `make clean-cache`.
- **Раз в 1–2 месяца** — глобальные `~/.cache/go-build`, `go clean -modcache`,
  старые partial-блобы.
- Всегда запускать Go-команды через:

```bash
source .sandbox-cache/env.sh
```

Это держит кэши внутри репозитория (`GOCACHE`, `GOPATH`, `GOLANGCI_LINT_CACHE`);
без source они дублируются в `~/.cache/go-build` и `~/go`.

## Git и данные

`.sandbox-cache/`, `bin/`, `coverage.out`, `logs/` — производные, в git не нужны.
Перед очисткой проверь `git status`: незакоммиченные изменения и данные не
должны попасть под `rm`.
