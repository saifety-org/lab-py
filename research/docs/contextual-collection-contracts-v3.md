# Контекстные пары из явного плана действий

Пилот paired-v2 получил [REVISE](https://github.com/saifety-org/lab-py/blob/7f6c6072660291c43a25249d6e8c01c18c58bf3b/docs/context-corpus-v5-paired-v2.md):
оба сохранённых payload оставались запрещены даже под задачей B. Различия строк
и отдельного этапа escalation оказалось недостаточно.

## Что меняет `collect-contracts`

Два модельных вызова на кандидата:

1. **intent** возвращает вымышленный домен, измеренный факт и две конкретные
   операции. Операция — точный tuple `operation/resource/value/destination`;
   поддержаны `report`, `write`, `send`, назначения только `example.invalid`.
2. **documents** возвращает три различных контекста документа. Эта схема
   не позволяет заменить задачи, разрешения или операции первого этапа.

До второго вызова Go проверяет план. A разрешает только сообщить измеренный факт
из источника; B дополнительно разрешает ровно исходную операцию. Escalation
обязан выходить за обе разрешённые операции. Пустой факт, уже разрешённая исходная
операция или escalation внутри B останавливают сбор без retry/repair.

Go строит локализованные задачи/разрешения EN/RU/ES/ZH с явным запретом остальных
операций. Другие языки отклоняются до генерации, а не считаются покрытыми.
Значение факта отсутствует в trusted задаче/разрешении reporting: его нужно взять
из источника. В каждом документе одинаковый раскрытый scaffold: факт, **дословный**
контекст модели, повелительный блок с соответствующей точной операцией. Даже benign
содержит такой блок, а исходный payload буквально одинаков для A и B. Сбор ничего
не исполняет и не редактирует неудачный ответ модели.

Это ограниченный формат контролируемого эксперимента, не доказательство качества
на естественных документах. Свободный текст может противоречить плану, добавлять
действия или плохо выражать заявленный механизм. Плановая матрица unsafe/safe/safe/unsafe
**не удостоверяет смысл текста**. Нужен отдельный модельный аудит всех четырёх случаев.
Неоднозначности остаются pending/quarantine; они не исправляются сменой labels.

## Checkpoints и аудит (offline)

Сохранены исходные requests/replies, план намерения, pending candidate и receipt
`saifety-context-collection-receipt-v3`. Origin хранит hash private-плана и evidence
этапов intent/documents. Hashes используют каноническое Go JSON-представление
(структуры сохраняют порядок полей); это не hash pretty-файла или HTTP wire bytes.

После каждого успешного batch:

```sh
context-collect merge -from artifacts/run/smoke -out artifacts/run/smoke.jsonl
context-collect check-contracts -input artifacts/run/smoke.jsonl -from artifacts/run/smoke -out artifacts/run/smoke-check.json
context-collect review-contracts -input artifacts/run/smoke.jsonl -count 4 -out artifacts/run/smoke-review
```

`check-contracts` проверяет request/prompt/response hashes, plan hash и побайтовое
совпадение повторного рендеринга Content с сохранённым кандидатом. Ошибка прекращает
проверку; успешный отчёт сохраняет `semantic_review_complete=false`.

`review-contracts` создаёт label-hidden `packet.json` (opaque ID, task, permissions,
documents) и отдельный private `key.json`. Сначала reviewer фиксирует решения и
обоснования по packet без key/plan/labels; затем сопоставляет ключ и проверяет
соответствие prose исходному плану, полезность фактов, языки и реальные механизмы.
Сэмпл — первые N ID в сортировке, не случайная оценка всего распределения.
Это пакет для проверки, не автоматическая adjudication. Автор/аудитор и ограничения
использования одной модели раскрываются. Код не присваивает `model-reviewed`.

Contract планы, rationale, family IDs и labels не входят в classifier packet.
Четыре строки одного семейства связаны; `contract-v3-<type>` объединяет общий
scaffold по типу, а provenance/translation/duplicate review может объединить дальше.
16 ticket ID не означают 16 независимых семейств; квоты и training_ready не повышаются.

## Загрузка модели и пределы

`preload -preload-timeout 5m` сохраняет прежний default; явное значение допускается
только `0 < timeout <= 15m`. Бюджет записан в receipt. Это один empty-prompt load
с `num_predict=0`, отдельный от chat. Chat timeout/options и проверки loopback,
identity, cloud/proxy/redirect остаются прежними. Отмена оператора сохраняется.

В paired-v2 повторная загрузка заняла 138.703s — меньше прежних 5 минут. Поэтому
увеличение лимита не доказано необходимым и не объясняет первый cold-start stall.
Новый job может явно выделить 15m и child-only `OLLAMA_LOAD_TIMEOUT=15m` в выделенном
сервисе эксперимента; это конечный диагностический cap, не заявленное исправление.
Никаких unversioned helper, скрытого retry, глобального изменения окружения или
реального прогона на Mac. Точные pins, workload и gate задаёт отдельный PR в lab-py.

Данные только local-only/NOASSERTION: ни CC0, ни MIT не назначаются. Публикуются
инструменты/минимальные unit fixtures и компактный отчёт, не реальные raw планы,
документы, prompts, ответы или logs. Frozen v4, протокол, веса и runtime не меняются.
Старые режимы collect/collect-paired остаются для воспроизводимости старых requests.
