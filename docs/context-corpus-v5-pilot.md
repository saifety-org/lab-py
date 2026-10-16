# Пилот сбора контекстного корпуса v5: остановка на smoke (2026-10-16)

Вердикт: **stop**. Первый запрос smoke завершился HTTP-таймаутом до получения
заголовков ответа Ollama. Кандидатов нет; оставшиеся smoke-запросы и pilot 60
не запускались. Повторов, замены модели и изменения параметров не было.

Это результат проверки локального сбора, а не оценка качества корпуса,
анализатора или защиты агента. `training_ready=false`, `quality_evidence=false`.
`NOASSERTION` означает отсутствие декларации лицензии, не право распространения.
Raw candidates/full traces должны оставаться local-only; в этом запуске кандидатные
и response-checkpoint файлы не появились.

- [Карточка пилота](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=272703263).
- [Принятый runbook](../device-jobs/context-corpus-v5-pilot.md) и
  [request](../device-jobs/context-corpus-v5-pilot.json).
- [Компактный отчёт](../reports/context-corpus-v5-pilot.json).
- Родительский корпус этим результатом не закрывается.

## Готовность и воспроизводимость

Карточка была Todo, не занята другим агентом; перед установкой переведена в
In Progress. Прочитаны принятые AGENTS/device-work/request/runbook, сохранены
исходный task body и его hash. Чистый lab-py HEAD совпадал с GitHub main:
`59122840f8bfc02f27b09acba5fea7dd39020330` ([PR #11](https://github.com/saifety-org/lab-py/pull/11)).

Collector [lab #13](https://github.com/saifety-org/lab/pull/13) принят пользователем:
head `95b8b2dc6e294a0348c18a5468096390a498acb1`, merge
`d48473a7f68a5a0581dcbe6ecb23191ee5369868`.
Main CI [lab](https://github.com/saifety-org/lab/actions/runs/38795752312) и
[lab-py](https://github.com/saifety-org/lab-py/actions/runs/38795804326) были SUCCESS
до установки и генерации.

Использован `context-collect` из
`github.com/saifety-org/lab/cmd/context-collect@v0.0.0-20261015173350-95b8b2dc6e29`,
собранный Go 1.26.0 для Windows/amd64 в новом ignored `bin/context-collection-v5`.
`go version -m` подтверждает module/version и не содержит `vcs.modified=true`;
отдельное vcs.revision в versioned module build не записано. Привязка к commit
основана на immutable pseudo-version и проверенном head принятого PR.

Хеш протокола совпал с pin
`c5bd48ef63338196746a9c0b95f228ab9129283d056db9ab67327bdaba8c1676`,
план из 6380 tickets — с
`0a4af4d9d6bd05cef70af019b3f33ae7777ba5d082edcc4b30f1e7cc239da695`.
Reservation в плане не назначает train/validation/confirmation splits и не
подтверждает независимость или review. Binary/request/task-body hashes и manifest
сохранённых локальных outputs включены в JSON.

## Оборудование и runtime

- Native Windows 11 Enterprise, version 10.0.26200/build 26200.
- AMD Ryzen 9 9950X, 16 физических ядер / 32 логических процессора;
  RAM 68241707008 байт (около 63,56 GiB).
- NVIDIA GeForce RTX 5070 Ti, 16303 MiB, драйвер **610.88**.
  В предыдущем успешном refusal-прогоне был 591.86; обновление драйвера здесь
  не выполнялось и само по себе не устанавливает причину текущего сбоя.
- Повторно использована существующая portable Ollama 0.18.0/CUDA v13 и
  `qwen3:8b-q4_K_M`, digest
  `500a1f067a9f782620b40bee6f7b0c89e17ae61f686b92c24933e4ca4b2b8b41`.
  Модель не скачивалась и не заменялась.
- Только `http://127.0.0.1:11435`, `OLLAMA_NO_CLOUD=1`; без LAN,
  hosted fallback, WSL, удалённого управления или Mac inference.

Collector проверил установленный tag/digest/GGUF/Ollama version до первого запроса.
После генерации identity не проверена: успешного ответа не было. Нельзя объявлять
успешно пройденными end-to-end identity и GPU residency gates.

## Команды и фактический расход

Использованы установка/plan/collect из принятого runbook. После проверки готовности
план построен без обращения к модели:

```powershell
$env:GOTOOLCHAIN = 'go1.26.0'
# GOBIN указывает на новый bin/context-collection-v5, предыдущие bin не затрагиваются.
go install github.com/saifety-org/lab/cmd/context-collect@v0.0.0-20261015173350-95b8b2dc6e29
```

Локальный сервис и collector запускались в одном дереве процессов с общим
loopback. Непосредственная команда smoke, где `$cli`, `$plan`, `$run` и `$job`
подготовлены по runbook:

```powershell
& $cli collect -input $plan -sha256 $job.plan_sha256 `
  -ollama-url $job.ollama_url -model $job.model -model-digest $job.model_digest `
  -license NOASSERTION -tokens 2048 -start 0 -count 4 -wall-time 10m `
  -out (Join-Path $run 'smoke')
```

Запрос: temperature 0.8, seed 1, context 8192, `think=false`,
`truncate=false`, `shift=false`. Общий request ограничивает сбор smoke 4 + pilot 60,
64 запросами / 131072 output tokens / 120 минут generation wall (10 + 110).
Это caps, не прогноз времени.

| Наблюдение | Результат |
| --- | ---: |
| Запрошено в smoke | 4 |
| Фактически начато generation requests | 1 |
| Принято unreviewed candidates | 0 |
| Сохранено response checkpoints | 0 |
| Начато запросов pilot 60 | 0 |
| Повторных генераций | 0 |
| Reported generated tokens | 0 |
| Фактические generated/prompt tokens | неизвестны |
| Usage complete | false |
| Collector exit code | 1 |

Receipt generation interval: `2026-10-16T05:46:41.5941402Z` —
`2026-10-16T05:49:41.598442Z`, 180,004 с. Полный smoke-процесс collector занял
197,406 с, включая предшествующую identity inspection и завершение процесса.
Подготовка до сервиса — 187,107 с, измеренная от создания bin-каталога до metadata,
включая операторский осмотр, а не только чистое время компиляции. Ожидание HTTP
готовности сервиса перед collector — около 67,175 с. Sample review не выполнялось:
нет доступных кандидатов; это явно отражено как отсутствие измерения.

`generated_tokens=0` в receipt — отсутствие полученного usage, **не доказательство
нулевой фактической генерации**. Заголовки/тело ответа не получены; утверждать
сохранение всех raw replies или полный token accounting нельзя.

## Диагностика и пределы вывода

Collector остановился с:

```text
POST /api/chat: context deadline exceeded
(Client.Timeout exceeded while awaiting headers)
```

В закреплённом LocalClient timeout одного HTTP-запроса — 180 секунд.
Сервис загрузил CUDA backend и сообщил offload 37/37 слоёв, но оставался в
`llm server loading model` / ожидании llama runner. После отмены клиентом
сервис записал прерывание загрузки и HTTP 499 спустя 3m0s.

173 общих GPU-замера во время smoke/loading: максимум VRAM 7457 MiB,
загрузка максимум 15% (средняя 1,295%), температура максимум 38°C.
Размещение весов/выделение памяти не доказывает готовность inference.
`ollama ps`/100% GPU gate после smoke не достигнуты; причиной остановки является
не объявленный CPU fallback, а наблюдавшийся HTTP timeout во время загрузки runner.

Корневая причина задержки не установлена. Обновившийся драйвер — отличие среды,
но эти логи не доказывают его неисправность или несовместимость. Не менялись
таймауты collector, driver/runtime/model, global client config или параметры
генерации; дополнительного warmup-запроса не было.

## Кандидаты, audit и прогноз большого сбора

Доступных кандидатов 0, поэтому аудит всех доступных при неполном прогоне
содержит 0 IDs. Четыре гипотезы авторизации, полезность benign, копии/неоднозначность
и языковое качество **не оценены**; pending не менялось на reviewed.
Известных наблюдённых candidate/source-connected групп 0; семантическая
независимость не измерена, а не подтверждена отсутствием дублей.

Единственный attempted ticket нацелен на EN/instruction-override/file.
Это цель запроса, а не языковое/типовое покрытие полученных данных.
План предусматривает общий генератор и шесть циклических жанров/рецептов
(handover log, technical manual, tracker discussion, incident timeline,
repository note, service reply); разные tickets не делают данные независимыми.

Ни успешной latency, ни accepted-candidate/token throughput не измерено.
Прогноз большого сбора оставлен null: экстраполяция таймаута как скорости
успешной генерации была бы необоснованной. Лимит 120 минут не означает ни
измеренную длительность, ни разрешение следующего запуска.

## Артефакты и решение

Локально сохранены plan, исходный task body, build info, metadata, smoke receipt,
console/service log, GPU CSV и SHA256 manifest всех 16 outputs:

`artifacts/device-jobs/context-corpus-v5-pilot/20261016T054516Z-0e1f1d06/`.

Hash исходного body относится к UTF-8 содержимому GitHub с LF. Локальный текстовый
checkpoint имеет Windows line endings и отдельный byte hash в manifest; это
разные явно определённые представления. Модель/версия связаны предгенерационным
request hash и проверенной identity в receipt.

Сервис эксперимента остановлен. Merge/check кандидатов не запускались, поскольку
валидных checkpoint нет. В Git входят только этот разбор и компактный JSON;
raw content, полные логи, абсолютные личные пути, зависимости и веса не включены.

**Stop для текущего задания.** До большого сбора требуется отдельно исследовать
локальную готовность Ollama/llama runner и оформить новый принятый request/task
для любой повторной генерации. Принятый runbook запрещает скрытые retry/resume
после сбоя. Этот PR сохраняет неудачную попытку; он не подменяет её успешным пилотом
и не разрешает следующую генерацию. Frozen v4/test/пороги/weights не менялись,
contextual E5 не enforcing/promoted.
