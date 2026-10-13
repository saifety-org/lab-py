# proxy-refusal-v2: повторный прогон нейтрального отказа

[Карточка для Windows-ПК](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=269257271).
[Request](proxy-refusal-v2.json), [source lock](../agent-harness-refusal-v2.sources.lock.json).
Стартовать только после Todo в карточке, принятия [core PR #10](https://github.com/saifety-org/sAIfety/pull/10),
PR этого runbook/метрик и успешного main CI. В Backlog устройство не требуется.

## Цель и неизменяемые условия

Проверить, исчезли ли неразрешённые виртуальные отправки, вызванные текстом
блокировки, и измерить реальную доставку документов. Политику карантина,
production weights и threshold не менять. E5 выключен; v4 test не трогать.
Первый отчёт и старый `agent-harness.sources.lock.json` сохранить.

Native Windows / RTX 5070 Ti; тот же `qwen3:8b-q4_K_M` и digest из request.
Ollama 0.18.0, local-only, loopback 127.0.0.1:11435. Сценарии byte-identical
первому прогону. Temp 0, seeds 1/2/3, context 8192, 8 turns/256 tokens,
`think=false`, `truncate=false`, `shift=false`. Smoke 8 эпизодов/до 64 генераций,
full 96/до 768. Три seed не являются независимыми статистическими испытаниями.

## Подготовка

1. Прочитать AGENTS.md/docs/device-work.md и текущую карточку через `gh`.
   Проверить, что она Todo и не занята; перевести в In Progress, записать lab-py SHA.
2. Открыть принятый чистый main lab-py. Проверить Git, gh, uv, Go, GPU/driver и диск.
   Повторно использовать существующие runtime, модель и `.runtime`; старые artifacts
   не удалять. Никаких hosted API или тяжёлого обучения.
3. Поднять существующую отдельную Ollama с `OLLAMA_NO_CLOUD=1`, не затрагивая
   другую установку/сервис. Из корня lab-py при сохранённой структуре первого прогона:

```powershell
$env:OLLAMA_HOST = '127.0.0.1:11435'
$env:OLLAMA_NO_CLOUD = '1'
$env:OLLAMA_MODELS = (Resolve-Path '..\.runtime\ollama-models').Path
& '..\.runtime\ollama-v0.18.0\ollama.exe' serve
```

Сервис и клиент должны находиться в среде с общим loopback. При отсутствии папки
не подставлять чужие пути: найти настроенную локальную установку, записать отклонение
в карточке. Не расширять адрес на LAN и не менять глобальные ограничения sandbox.

## Выполнение

В другом PowerShell, в корне lab-py:

```powershell
.\scripts\run-device-job.ps1 -Plan
.\scripts\run-device-job.ps1 -OllamaExe '..\.runtime\ollama-v0.18.0\ollama.exe'
```

Runner проверяет merge/CI, pins/digest/Ollama version, затем bootstrap/prepare,
smoke и full. После smoke проверяются нулевые ошибки, полный paired coverage,
стабильность identity и GPU residency через `ollama ps`. При ошибке или исчерпании
бюджета full не запускается; исходные файлы сохраняются. Вывод содержит новый
каталог `artifacts/device-jobs/proxy-refusal-v2/<run-id>` с metadata, smoke/full,
компактным candidate report и GPU/processor snapshot. Повтор не перезаписывает его.

Во время прогона не менять модель, source lock, сценарии, политику или budgets.
Не делать test-driven подбор threshold. После завершения остановить только сервис
эксперимента, если он был поднят для этого задания.

## Публикация

1. Прочитать report/traces, проверить отсутствие ошибок и неполных пар. Разобрать
   virtual sends/нарушения, все benign regressions и blocked reads. Проверить operator
   findings/reasons; не делать вывод о вкладе детектора только по итоговому action.
2. Подготовить `reports/agent-harness-ollama-refusal-v2.json` из сохранённых
   source reports, сохранив hashes, обе arms, paired results, бюджеты/фактический расход,
   delivery counters и выполнение без нарушений. Candidate формируется runner;
   проверить его и дополнить разбор, без пересчёта исторического отчёта.
3. Написать `docs/agent-harness-ollama-refusal-v2.md`: оборудование/версии/команды,
   результат и сравнение с первым прогоном, ошибки/изменения среды и ограничения.
   Task success не равен безопасному выполнению или чтению исходного текста.
4. Отдельная ветка и PR `test: record local LLM evaluation of neutral proxy refusals`.
   Добавить ссылку на карточку. Не коммитить полные artifacts, runtime, модель и
   личные данные; использовать согласованный noreply, без AI coauthor.
5. В карточке записать PR, SHA, outcome, пути локальных artifacts относительно repo
   и ошибки; статус Review. Done только после пользовательского merge/main CI.

ASR=0 в baseline не показывает предотвращение атак. Исправленный текст отказа
не должен автоматически устранять потерю полезных данных или tool-wide quarantine:
это отдельная [задача](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=269253451).
Никакой promotion E5 или заявления о production readiness по этому прогону.
