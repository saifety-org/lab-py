# Работа с задачами и устройствами

По прямому указанию пользователя от 2026-10-20 новые поручения ведём
самостоятельными tasks в [sAIfety Project](https://github.com/orgs/saifety-org/projects/1),
без repository Issues. В API это ProjectV2DraftIssue; не конвертировать task в Issue.
Body содержит цель, критерии, зависимости и журнал; поля — Status/Priority/
Component/Device/Assignees. Перед append журнала прочитать актуальную body
и сохранить существующий текст. Старые Issues/обсуждения остаются историей.
Не выполнять массовую миграцию/архивацию/закрытие старых карточек без отдельного поручения.

Прогресс/ошибки/обычные результаты — в журнал task, без PR. Код, воспроизводимые
requests/runbooks и значимые итоговые отчёты — через PR. Самостоятельно не merge
и не push в main.
Названия `<type>: <суть>`, без codex, без AI coauthor.
Публичный email согласован: `7062352+alexandr-mironov@users.noreply.github.com`;
задавать через `git -c user.email=...`, не менять глобальный конфиг.

При запросе продолжить работу на Windows-ПК с RTX 5070 Ti:

1. Прочитать [docs/device-work.md](docs/device-work.md) и [очередь заданий](device-jobs/README.md).
2. Через настроенный `gh` прочитать task body/журнал, статус
   в Project и зависимости.
   Брать только готовую `Todo`; `Backlog`/ожидание merge не означает разрешение запуска.
3. Проверить принятый main и CI, отметить в карточке `In Progress` до запуска.
   Использовать версионированное задание из репозитория, не искать промпт в другой сессии.
4. Прогресс/ошибки/результат и точные версии дописывать в журнал task.
   При недоступном устройстве записать это там. Значимый итоговый отчёт сохранять
   через PR, если это требует принятое задание; обычному сообщению PR не нужен.
5. При требуемом PR — `Review`, затем `Done` task только после
   пользовательского merge и успешного main CI. Для работы без PR — после
   выполнения критериев и зафиксированного результата в task; если нужна приёмка,
   сначала `Review`. Поддерживать body и поля task согласованными.
   Модельные выводы не подменять результатами scripted actor.

Работа на одном устройстве не даёт доступа к другому. Не использовать hand off,
удалённое управление или постоянный self-hosted runner для переноса задачи.
Нужен локальный агент на ПК; оператор запускает его, когда устройство включено.

Общаться по-русски. Браузер не использовать без явного поручения пользователя;
не предлагать его и не запрашивать доступ самостоятельно. Не обращаться к Gmail,
не читать/копировать секреты или токены. Пользоваться настроенными CLI.

Не выполнять тяжёлые LLM-задания на Mac по наличию карточки для GPU-ПК.
Не применять E5 для production-блокировки, не менять frozen v4 test/пороги,
не публиковать веса или полные локальные traces без отдельного задания.

## Единая лаборатория

`lab-py` — единственный поддерживаемый research repository. Данные, Go module/
cmd/internal и их тесты находятся в `research/`; Python — в `src/`, задания —
в `device-jobs/`. Исправление инструмента, source pins и задание идут одним PR.
Не создавать новые PR/Issues в историческом `saifety-org/lab`.

Активные source locks schema2 читают локальные данные по SHA256 и собирают Go
из этого checkout. После изменения Go inputs обновить `research/tools.lock.json`
через `python scripts/update-research-lock.py`, включить diff manifest в тот же PR.
Перед сборкой/подготовкой источники проверяются; не подменять identity старой
pseudo-version. Frozen dataset bytes/manifest/labels и старые reports не менять.
Исторические locks и исходные commit URLs остаются для воспроизводимости.

Проверки: Python lint/tests/build и Go `make -C research fmt-check vet ci-test
ci-build`; межъязыковые integration checks и frozen corpus gates остаются обязательны.
Canonical GPU job ждёт пользовательского merge этого же lab-py PR и успешного
общего main workflow `check`; отдельного merge/CI в lab больше не требуется.

Текущий v4 прогон ведётся в [Project task](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=276945651).
Issue URLs внутри принятых JSON requests — историческое происхождение; смена
учёта не меняет experiment hashes/budgets и не разрешает повтор. Для получения
task body использовать gh GraphQL DraftIssue, для append — updateProjectV2DraftIssue.
