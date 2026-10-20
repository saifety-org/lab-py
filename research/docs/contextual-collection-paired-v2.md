# Двухэтапная генерация контекстных кандидатов v5

[Задача исправления](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=273503006).
Принятый [первый пилот](https://github.com/saifety-org/lab-py/blob/e0f6b88142048453308dde60ea86d501d99a4a56/docs/context-corpus-v5-pilot.md)
остановился: timeout загрузки, затем одинаковые payload/escalation в отдельном повторе.
Результаты и старые request/collector pins сохраняются. Реальная эффективность
нового генератора пока не измерена; локальные HTTP mocks проверяют только механику.

## Генерация

`collect-paired` делает **два** последовательных `/api/chat` на кандидата:

1. `base`: две задачи/набора разрешений, payload, benign и объяснение трёх сочетаний.
   JSON Schema исключает escalation; локальная закрытая schema/проверка контракта
   выполняется до второго запроса. Задача B ограниченно разрешает действие payload.
2. `escalation`: получает неизменяемый base как данные, возвращает только новый массив
   документов и объяснение выхода за разрешения задачи B. Переписать задачи/payload
   или добавлять другие поля запрещено. Число документов закреплено schema.

Используется [structured format Ollama](https://docs.ollama.com/api/chat): отдельная
JSON Schema для каждого этапа. Seed1/temp0.8/context8192, без thinking/truncate/shift;
`keep_alive=10m`. До 4 документов, 16 permissions на контракт, 32KiB JSON content на
этап; общий валидатор сохраняет 128KiB bound кандидата. Два rationale соединяются
как явно обозначенные выводы разных этапов. Модельный текст не редактируется.

Готовый raw contract остаётся `saifety-context-candidates-v1`; это pending данные,
которые старый prepared training pipeline не принимает как reviewed. В `origin.stages`
записаны base/escalation с hashes; recipe `paired-<genre>-v2`, ID `v5p2-…` отличает
их от прежнего одноэтапного генератора. Группы происхождения сохраняют общий
генератор/жанры, разные tickets не доказывают независимость.

Валидатор отклоняет одинаковые массивы документов, включая NFKC/casefold/whitespace
копии. Такая консервативная проверка может отсеять чувствительные к регистру случаи;
разные строки всё равно не доказывают разные действия или правильную семантику.
Четыре комбинации авторизации требуют последующего модельного ревью. Никаких
автоматических меток reviewed, изменения frozen v4 или promotion анализатора.

## Checkpoints и расходы

Перед HTTP записывается `request-<ticket>-<stage>.json`; полученный ответ —
`response-<ticket>-<stage>.json`, включая incomplete/truncated/invalid content.
HTTP/невалидный внешний JSON может не оставить response; usage неизвестен, не нулевой.
После каждого этапа проверяется installed model identity. Первый сбой останавливает
весь batch, второй этап/следующий ticket не запускаются, скрытых retries/починки нет.

Receipt `saifety-context-collection-receipt-v2`: requested/accepted относятся к
кандидатам, `generation_requests` — ко всем base/escalation HTTP attempts, cap `2*count`.
Каждый этап имеет ticket/status, reported generated/prompt counts и completeness.
Успешный base не считается принятым кандидатом; его расход не теряется при сбое
escalation. Missing usage не заменяется нулём. Ошибки и текущая phase сохраняются.

Stage request hash — SHA256 canonical `json.Marshal` payload; включает prompt,
schema/options/модель. Prompt hash — SHA256 UTF-8 строки. Response hash — SHA256
`json.Marshal(Generation)`, не исходный HTTP wire или pretty checkpoint file.
Aggregate origin prompt/response hashes — SHA256 JSON массива двух stage hashes
в порядке base/escalation; валидатор проверяет соответствие. Hash manifest файлов
считается отдельно, поскольку pretty JSON/newline дают другие byte hashes.

Новые кандидаты собираются **только** с явным `-license NOASSERTION`. Это отсутствие
публичной лицензии, не разрешение распространения. CC0 отклонена пользователем,
MIT вместо неё не назначается. `publish` по-прежнему отклоняет NOASSERTION.
Request/response/candidates/full traces остаются в ignored artifacts; в PR —
компактный отчёт/hashes/агрегаты без raw content. Успешная структура не означает
правовую или семантическую готовность к обучению.

## Загрузка runtime отдельно

`preload` выполняет один [пустой `/api/generate`](https://github.com/ollama/ollama/blob/v0.18.0/docs/api.md#load-a-model)
с `num_predict=0`, context8192/keep_alive10m, exact installed tag/digest/Ollama0.18.0.
Whole preload cap 5 минут, отдельный receipt/request/response, identity до/после.
Непустой response/thinking, reported tokens !=0 или identity drift — остановка.
Отсутствующий eval_count остаётся null; это не придуманное usage. Проверка не
доказывает 100%GPU — это отдельный `ollama ps`/snapshot gate в runbook.

Только load имеет HTTP timeout до5m; chat остаётся180s и whole batch≤2h.
Redirect/proxy/cloud запрещены, модель не скачивается, драйвер/runtime не меняются.
Это отделяет диагностику загрузки от содержания; причина исходного timeout не установлена.

```sh
# Только на назначенном ПК по принятому отдельному job, не по этому примеру на Mac.
context-collect preload -model TAG -model-digest DIGEST -out NEW_LOAD_DIR
context-collect collect-paired -input PLAN -sha256 PLAN_SHA -model TAG \
  -model-digest DIGEST -license NOASSERTION -start 0 -count 4 \
  -tokens 2048 -wall-time 20m -out NEW_SMOKE_DIR
```

Новый [повторный пилот](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=273503121)
имеет отдельный versioned job в lab-py. Backlog ждёт принятия PR/main CI; cap всех
модельных вызовов включает preload и оба этапа. Большой сбор требует принятого
continue-результата и нового измеренного бюджета, не запускается этой доработкой.
