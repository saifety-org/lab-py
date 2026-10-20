# Одна исследовательская лаборатория

По решению пользователя от 2026-10-20 поддерживаемая лаборатория — `lab-py`.
Исторический `saifety-org/lab` сохраняется для старых commits/PR/отчётов;
новые data/tool/job изменения идут одним PR в lab-py. Самостоятельный merge
не разрешён; принятым считается пользовательский merge с успешным main `check`.

## Что перенесено

`research/` содержит Go module, команды, internal packages, корпуса, fixtures,
Makefile, linter config и методику из lab head
`c7130bdb3aba0acbee69872c22f4988eec9c255b` (main
`7419a289ccaddc95b4c59571ba7c38d0f9199e64` плюс исправление lab#21).
`research/import.json` фиксирует source/destination paths и hashes. Изменены
module/import paths на `github.com/saifety-org/lab-py/research`; исходные data,
frozen labels/manifests, split и regression bytes не меняются. README lab
сохранён отдельно в `research/docs/history/`.

Старое исправление lab#21 и непринятый request lab-py#26 включены в один migration
PR. Независимый results PR lab-py#24 остаётся отдельным: это результат прошлого
прогона, его содержание не переписывается. Старые jobs не получают разрешение
на повтор. Новых model calls, обучения, freeze, weights или promotion перенос
не выполняет. Runtime приложения и production artifacts остаются отдельными
закреплёнными зависимостями sAIfety/prompt-injection-model.

## Локальные данные и source identity

Активные source locks schema2 указывают repository lab-py, относительный
`local_path` и прежний SHA256 каждого файла. `commit`/`source_origin` описывают
историческое происхождение данных, не новую сборку. `sync`/`prepare` читают
проверенные bytes из checkout и наполняют ignored data cache без обращения к lab.
Harness locks schema2 сохраняют прежние core module/version и scenario hashes.

Все прежние locks сохранены дословно в `docs/history/pre-unification-locks/`.
Поддержка legacy schema1 остаётся только для явного historical reproduction;
она может обращаться к закреплённым commits архивного lab. Два отличающихся
прежних файла сохранены в `research/legacy-sources/`: generator source v1
как `.go.txt` (только provenance, не пакет для сборки) и прежний README corpus v1.
Остальные pinned bytes уже содержатся в канонических `research/datasets/testdata`.

Активные Go tools собираются из `research/`. `research/tools.lock.json` фиксирует
полный набор Go исходников (кроме ignored bin/artifacts и legacy snapshots),
go.mod/go.sum и их hashes;
добавление/удаление/изменение input вызывает отказ до сборки. В `tools.json`
effective `lab_version` имеет вид `sha256:<manifest hash>`, сохраняются hashes
исполняемых файлов. Старый ключ metadata сохранён для совместимости, но старую
pseudo-version новой сборке не приписываем. Источники проверяются до и после
build; Git accepted HEAD фиксируется отдельно в job metadata. Это не подпись
доверенного builder и не гарантия идентичных binaries разных OS/toolchains.

После намеренного изменения Go inputs: `python scripts/update-research-lock.py`.
Manifest, tools, data locks и job меняются одним PR. `GOWORK=off`, readonly Go
dependencies, отсутствие local replace и hashes бинарников сохраняют проверяемость.
Прежние prepared caches/artifacts не перезаписывать: при иной identity использовать
отдельные output paths; старые candidate metadata могут законно отклонить новый
source-lock hash. Их воспроизводят историческим checkout/locks, а не подменой hash.

## CI и задания

Общий workflow `check` включает прежние Python lint/tests/build, ML/harness,
перенесённые Go lint/race/native+onnx builds и Windows collection checks.
Обычный CI не вызывает реальную LLM и не обучает новую production модель.
Регрессии проверяют offline sync, неизменность source/frozen hashes, запрет
выхода локального path из research/, code identity drift и реальные Go bridges.

Новый `context-corpus-v5-contracts-v4` собирает collector из принятого lab-py
checkout. Request закрепляет tools manifest SHA, прежние protocol/plan/model/
runtime и finite budgets. Только пользовательский merge этого же PR + успешный
main CI → Todo → In Progress. Отдельного PR/merge/CI в lab нет. Guards исходного
prose остаются эвристическими; pending/local-only/NOASSERTION и bounded key-hidden
semantic audit обязательны. Языковой guard collector не становится production
фильтром неизвестных языков.

## Задачи и исторический архив

Открытые Issues перенесены GitHub transfer с историей:

| lab | lab-py | Назначение |
| --- | --- | --- |
| #16 | #28 | Расширение корпуса |
| #17 | #29 | Agent-сценарии |
| #18 | #30 | Review/leakage/freeze |
| #19 | #31 | Будущий сырой пул |
| #20 | #32 | Исправление prose fidelity |

Project item IDs, статусы, приоритеты, устройства и исполнители сохраняются;
компонент становится lab-py. Перенос Issue не означает выполнения его критериев.
Исторические body/comments сохраняют прежние формулировки и ссылки; новое
расположение и зависимости уточняются комментариями. Lab больше не получает
новые research изменения; существующие refs/data остаются доступными для истории.
