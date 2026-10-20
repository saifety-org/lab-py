# Работа через задачи GitHub Project

По указанию пользователя от 2026-10-20 поручения оформляем самостоятельными tasks
в [Project](https://github.com/orgs/saifety-org/projects/1), без repository Issues.
API type ProjectV2DraftIssue используется как постоянная task, не как будущий Issue.
Цель/критерии/зависимости/job/журнал — body, Status/Priority/Component/Device/
Assignees — поля Project. Перед append сначала прочитать актуальный текст.

Через настроенный gh, без браузера:

```sh
gh project item-list 1 --owner saifety-org --format json --limit 100
```

Task body читать через gh GraphQL DraftIssue, дописывать через
updateProjectV2DraftIssue. Для Project нужен scope project; токены не извлекать.
Старые Issues/comments и [прежнее описание](history/issue-device-work-before-project-tasks.md)
сохраняют историю. Массовый перенос/закрытие старых карточек не входит в обычный run.

## Исполнение

1. Создать task с устройством, критериями и versioned job. Изменения tooling/
   request/runbook — одним PR lab-py; до usermerge/main CI статус Backlog.
2. Проверить принятый main и общий check, exact source/request/model pins;
   записать версии в task, перевести Todo. Локальный агент назначает исполнителя
   и In Progress до первого model call, сохраняет body snapshot/hash.
3. Использовать чистый принятый checkout (git pull --ff-only) или отдельный clone,
   не сбрасывать чужие изменения. Выполнить конечный runbook; partial/invalid/error
   не разрешает скрытый retry, repair или hosted fallback.
4. В журнале фиксировать attempt/device/UTC/accepted commits/hashes, actual
   calls/usage/time/GPU, errors/nulls, verdict и URL results PR. Значимые compact
   JSON/Markdown — PR; обычный status update PR не требует.
5. При results PR — Review, затем Done только после usermerge/main CI и выполнения
   критериев. Завершение этапа не является model promotion. Без требуемого PR
   достаточно выполненных критериев и журнала, либо Review при требуемой приёмке.

Go tools/data, Python workflow и jobs находятся в lab-py; lab архивирован.
Collector собирается из accepted checkout по research/tools.lock.json, source и
binary identities записываются отдельно. Runtime sAIfety и production model
artifacts остаются отдельными pinned dependencies. Отдельного lab merge нет.

## Текущий результат и ограничения

[Task исправленного прогона](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=276945651):
v4 выполнен с REVISE, первый intent невалиден, documents/pilot не достигнуты.
[Отчёт](context-corpus-v5-contracts-v4.md).
[Следующее исправление](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=276953222)
остаётся Backlog; новые model calls требуют нового принятого versioned request.

[Очередь](../device-jobs/README.md) не заменяет live статус/проверку зависимостей.
Старые jobs не повторять по наличию файла. Issue URLs в accepted JSON requests
остаются provenance; смена учёта не меняет experimental hashes/budgets.
Старый scripts/run-device-job.ps1 исполняет только proxy-refusal-v2; -Plan offline.

Реальный LLM run — локальный Windows/RTX5070Ti агент. Без hand off/remote access/
self-hosted runner/тяжёлого Mac. Браузер/Gmail/secrets не использовать.
Raw plans/prompts/replies/keys/judgments/logs — ignored artifacts на ПК,
local-only/NOASSERTION; неизвестный usage — null, не ноль. Публиковать только compact
metrics/hashes/errors/limitations, без raw текста/личных paths/configs/secrets/weights.
Не менять frozen v4/test/thresholds и не запускать fullpool/training/freeze/promotion
вне отдельного принятого задания; scripted actor не заменяет модельные выводы.
