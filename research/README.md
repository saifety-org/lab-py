# Go tools и канонические исследовательские данные

Это поддерживаемая часть единого lab-py, перенесённая из исторического lab.
Python training/export и эти инструменты меняются в одном PR с общим CI.

- `cmd/`, `internal/`: collector, corpus gates, bridges, оценка и regression checks.
- `datasets/`, `testdata/`: прежние корпуса/fixtures с исходными bytes и hashes.
- `go.mod`, `go.sum`: отдельный закреплённый Go module; Python приложению не нужен.
- `tools.lock.json`: Go input hashes, проверяемые до локальной сборки.
- `import.json`: исходный commit/path/hash и destination/hash переноса.
- `legacy-sources/`: два прежних файла для byte-identical historical source locks.
- `docs/`: исходная методика; старые отчёты/URL описывают исторические версии.

Из корня lab-py: `make bootstrap`, `make bootstrap-context`,
`make bootstrap-collector`. Из этой папки: `make fmt-check vet ci-test ci-build`.
После намеренного изменения Go code из корня выполнить
`python scripts/update-research-lock.py` и включить manifest в тот же PR.

Сборка проверяет все Go inputs и использует `GOWORK=off`, `-mod=readonly`,
`-trimpath`, `-buildvcs=false`; identity записывается как SHA256 source manifest,
а бинарники имеют отдельные hashes. Версия принятого checkout — в metadata job.
Внешние product/model modules остаются отдельными pinned dependencies.

Raw новых GPU runs по-прежнему ignored local-only/NOASSERTION; новые веса,
full traces и зависимости сюда не добавляются. Перенос не меняет frozen labels,
thresholds, data licenses или production policy.

[Описание миграции](../docs/repository-unification.md),
[исходный README lab](docs/history/lab-README.md).
