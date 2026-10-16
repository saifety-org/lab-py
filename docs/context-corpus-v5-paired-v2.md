# Paired contextual corpus v5 pilot: повтор дошёл до smoke, требуется revise

Первая попытка остановилась на preload. В явно разрешённом повторе модель загрузилась
за138.703s, но smoke неполон, а оба сохранённых кандидата нарушают контрфактуал
разрешённости. Общий результат: **revise**. Обе попытки сохранены в JSON и ниже.

## Первая попытка: STOP на preload

2026-10-16, native Windows / RTX 5070 Ti. Новый принятый
[job](../device-jobs/context-corpus-v5-paired-v2.md) остановился до генерации корпуса:
единственный preload не дождался готовности Ollama runner за отведённые 5 минут.
Smoke и pilot не запускались; кандидатов и проверенных семейств нет. Двухэтапное
исправление генератора этим запуском **не проверено**. Решение: **stop**.

[Карточка](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=273503121),
[машинный отчёт](../reports/context-corpus-v5-paired-v2.json).

## Принятые версии и среда

- lab-py main `f2cc17377e56904d6c04e53e884c2dfe0c36b66e`, чистый checkout,
  [main check SUCCESS](https://github.com/saifety-org/lab-py/actions/runs/38898553065).
- Collector [lab PR#14](https://github.com/saifety-org/lab/pull/14), exact head
  `d42f0d781defdd579225670f03de52fbb2032767`, merge
  `043c44b848d0fe6ba12d78fb76e657bf9d8bdaad`,
  [main ci SUCCESS](https://github.com/saifety-org/lab/actions/runs/38898509574).
- `github.com/saifety-org/lab/cmd/context-collect@v0.0.0-20261016113305-d42f0d781def`,
  Go 1.26.0 / windows-amd64 / CGO disabled. Установлен в отдельный ignored bin;
  прежний collector сохранён. Build info соответствует pseudo-version, без
  `vcs.modified=true`; отдельный vcs.revision у versioned module install отсутствует.
- Существующие Ollama 0.18.0 / CUDA v13 и `qwen3:8b-q4_K_M`, digest
  `500a1f067a9f782620b40bee6f7b0c89e17ae61f686b92c24933e4ca4b2b8b41`.
  Identity подтверждена metadata inspect перед preload; проверка после него не состоялась.
- Ryzen 9 9950X (16 cores / 32 threads), RAM 68,241,707,008 bytes,
  Windows 11 Enterprise 10.0.26200, RTX 5070 Ti 16,303 MiB, driver 610.88.
  До запуска свободно C: 216,849,715,200 bytes; D: 104,944,746,496 bytes.
- Только `127.0.0.1:11435`, `OLLAMA_NO_CLOUD=1`. Глобальные настройки не менялись.

Request SHA256:
`848f6a6f7196529c289922e969b3b813f88c8298a7dc8e0b087c04ef541fd93a`.
Protocol SHA256:
`c5bd48ef63338196746a9c0b95f228ab9129283d056db9ab67327bdaba8c1676`.
Plan SHA256:
`0a4af4d9d6bd05cef70af019b3f33ae7777ba5d082edcc4b30f1e7cc239da695`.
Binary SHA256:
`beb35f9a778168af7e324c21fa9e43d4b1f0ada6862b30b768be776d84d78e11`.
Исходный task body/hash и build-info hash приведены в JSON; body checkpoint хранится
локально в UTF-8 с LF, hash относится к его точным байтам.

## Вызовы, время и gates

Карточка была Todo и переведена In Progress до установки и модельных вызовов.
Разрешённые caps: preload1 ≤5min, smoke4 ≤8 chat calls /20min,
pilot12 ≤24 chat calls /40min; всего ≤33 model calls /65536 outputtokens /65min.
Это лимиты, а не прогноз времени. Retries: 0.

| Этап | Фактические вызовы | Результат | Wall time |
| --- | ---: | --- | ---: |
| Setup | 0 модельных | установка, pins, hardware, HTTP readiness | 243.116 s |
| Preload | 1 `/api/generate` | `complete=false`, phase `model-load`, timeout | 300.063 s process |
| Smoke | 0 | не запускался | — |
| Pilot | 0 | не запускался | — |

Preload использовал пустой prompt, num_predict=0, num_ctx=8192, keep_alive=10m.
Пятиминутный collector deadline включает начальный metadata inspect:
receipt после inspect охватывает 297.536 s, process — 300.063 s с накладными расходами.
Клиент получил `context deadline exceeded`; сервер зарегистрировал HTTP499 после
отмены загрузки. Ответ модели не сохранён, `reported_generated_tokens=null`.
Суммарный token usage остаётся неизвестным. Ноль generation outputtokens относится
только к не начавшимся chat stages, не к неизвестному preload usage.

В серверном логе наблюдаются CUDA backend и offload 37/37 layers, затем ожидание
готовности runner вплоть до отмены. Телеметрия: 272 samples, peak GPU utilization100%,
peak VRAM9275MiB. Эти значения не заменяют успешный preload и snapshot `100% GPU`:
gate не пройден, smoke запрещён. Причина зависания runner не установлена.

Access log содержит 30 `/api/version`, 1 `/api/tags`, 1 `/api/show`,
1 `/api/generate`: сюда входят служебные readiness checks обёртки.
Collector выполнил один начальный inspect (3 metadata HTTP requests) и один preload;
остальные `/api/version` относятся к readiness. Генерационных `/api/chat` нет.

Перед запуском обнаружен orphan runner прошлой попытки. Его parent PID и port
сверены с локальными execution/server logs прежнего run; остановлен только этот
подтверждённый оставшийся процесс. После эксперимента остановлено дерево нового
task-сервиса, оставшихся Ollama processes проверкой не обнаружено.

## Coverage, аудит и следующий шаг

Accepted pending candidates0, complete pairs0, rejected complete pairs0,
incomplete candidate pairs0; counterfactual rows0, reviewed families0, known groups0.
Нет кандидатов, поэтому sample audit четырёх комбинаций разрешённости, проверка языка
и дублей невозможны. Merge/check корпуса не запускались после failed preload;
нулевые агрегаты вычислены из отсутствия candidate checkpoints.
Throughput корпуса **не измерен**. Review/report preparation записаны отдельно
в JSON и не включены в model wall. Все будущие кандидаты остаются pending.

Новый результат повторяет симптом timeout до готовности runner из первой старой
попытки. Во второй старой попытке ответ был получен, но payload совпал с escalation.
Этот запуск ничего не говорит об устранении того дефекта двухэтапным collector.
Драйвер, runtime или модель не менялись; причинной связи с их версиями не установлено.

Следующее отдельное задание должно диагностировать bounded cold-start / readiness
существующего runtime. Автоматический повтор, большой сбор, training/freeze/promotion
или изменение v4/thresholds этим результатом не разрешаются.
`quality_evidence=false`, `training_ready=false`.

## Локальные артефакты

`artifacts/device-jobs/context-corpus-v5-paired-v2/20261016T124638Z-9a999b79/`
содержит request/plan, task body, metadata/build info, preload request/receipt,
execution, GPU telemetry, полные server logs и diagnostics/audit checkpoint.
`SHA256SUMS.json` охватывает 20 файлов; hash самого manifest приведён в JSON.
Manifest исключает себя. Полные logs/requests, абсолютные локальные пути и веса
не включены в PR. Режим raw данных: local-only / `NOASSERTION`, публичная лицензия
не назначается. Старые request и оба результата предыдущего пилота сохранены.


## Явно разрешённый повтор: preload успешен, smoke требует доработки

Пользователь 2026-10-16 поручил повторить запуск и пересмотреть 5-minute timeout.
Это отдельная попытка, не скрытый retry. Исходная попытка выше и её20файлов сохранены;
исходные опубликованные Markdown/JSON дополнительно скопированы в новый local run.
Итог двух попыток: **revise**, не продолжать сбор.

Для этой попытки preload cap увеличен до15min, `OLLAMA_LOAD_TIMEOUT=15m` только
в child-сервисе; суммарный model wall cap75min вместо65min. В принятом collector
5min жёстко заданы и в context deadline, и в HTTP client: один preload выполнен
отдельным локальным helper, его SHA256/реализация/прямое разрешение пользователя
записаны в metadata и JSON. Это явное отклонение от принятого runbook, а не результат
неизменённого collector preload. Принятый request и бинарник collector не изменены.

Helper использовал тот же пустой JSON request: hash
`1de97c44159d3b8befff8803717bd2296b5a6ba351b68e67b5e7629507cb581f`
совпадает с первым preload. Сохранены exact local tag/digest/runtime inspect до/после,
запрет proxy/redirect/remote, complete/empty-content checks, nullable eval_count,
900s whole deadline. Chat stages выполняет прежний pinned Go collector с прежними
seed/temperature/context/tokens и caps4+12, без retries. Основой был чистый accepted
main `f2cc17377e56904d6c04e53e884c2dfe0c36b66e`, CI SUCCESS; model/runtime неизменны.

| Этап повторной попытки | Фактические вызовы | Результат | Process wall |
| --- | ---: | --- | ---: |
| Preload | 1 | complete; identity до/после совпала | 138.703 s |
| Smoke (запрошено4) | 5 chat stages | 2 pending pairs; третий base отклонён | 10.078 s |
| Pilot | 0 | не запускался | — |

Preload завершился **за2min19s, в пределах прежних5min**. Поэтому этот запуск
не доказывает необходимость увеличения timeout и не устанавливает причину прошлого
зависания. 5min — [настраиваемый default Ollama0.18.0](https://github.com/ollama/ollama/blob/v0.18.0/envconfig/config.go#L114),
не фундаментальное требование. Для повторной диагностики15min сохраняли конечный предел.
Preload response сохранён (`done_reason=load`, пустое содержимое); eval_count отсутствует,
поэтому usage preload и суммарный outputtokens остаются null.

GPU gate после preload пройден: `ollama ps` показывает100%GPU, snapshot
`size=size_vram=6584668288`, context8192, digest совпал. Smoke responses:
5HTTP200, done/stop, identity сохранена,1029outputtokens и1875prompttokens,
usage_complete=true. Всего в повторе6model calls; обе попытки вместе7calls
(2preload+5chat). Ни один ответ не изменялся, дополнительные модельные вызовы не делались.

Причина остановки третьего base — равенство `authorized_task` и `permitted_task`
после canonical normalization. Строки непустые, оба списка permissions присутствуют
и различаются. Сообщение валидатора `missing trusted authorization contract` объединяет
несколько условий и в этом случае неточно описывает дефект. Escalation третьего
кандидата не запускалась; сохранён raw base response. Полных rejected pairs0,
отклонённых base responses1, incomplete pairs1.

### Семантический аудит двух доступных кандидатов

Проверены все доступные pending candidates и все4комбинации, base fields сверены
с исходными checkpoint без изменений. Raw content и цитаты не публикуются.

| ID | Язык / тип | Главное замечание |
| --- | --- | --- |
| `v5p2-c5bd48ef-000000` | en / instruction-override | B+неизменённый payload требует изменений за пределами B; ожидаемая safe-комбинация нарушена |
| `v5p2-c5bd48ef-000001` | ru / authority-spoofing | B разрешает чтение, payload требует изменений; ожидаемая safe-комбинация нарушена |

Для обоих A+payload не имеет убедительного unsafe-контракта: A слишком широкая,
ограничения не заданы. A+benign не атакует, но полезные факты и выполнение задачи
не показаны. B+escalation выходит за scope B, однако base уже нарушает этот scope:
новая пересекаемая граница не установлена. Тексты читаемы на запрошенных языках,
но сценарии сведены к общим спискам действий; threat mechanism выражен слабо.
Дословных нормализованных копий в двухкандидатной выборке не обнаружено; payload и
escalation теперь различаются. Это ограниченное подтверждение структурной новизны,
без вывода о независимости или качестве. Semantic pass0/2, все записи **pending**.

Coverage частичного run:2pending families /8counterfactual rows, reviewed_families0,
known source roots2. Это механические группы, не reviewed independent families.
Полные merge/check после сбоя не выполнялись. Throughput pending candidates с preload
≈0.807/min, при этом оба кандидата семантически непригодны; это не прогноз скорости
пригодного full pool. Setup и время аудита/reporting приведены отдельно в JSON.

Нужно уточнить сообщения валидатора и научить генератор явно различать разрешения:
тот же payload должен быть запрещён в A и явно разрешён в B, escalation — пересекать
новую границу B. Одного различия строк или документов недостаточно. Не ослаблять
валидацию и не менять labels, чтобы принять ошибочный B+payload. Независимый аудит,
freeze/training/promotion и большой сбор по-прежнему не разрешены.

Новый local run:
`artifacts/device-jobs/context-corpus-v5-paired-v2/20261016T130740Z-01ee15d2/`.
Manifest/hash и hashes helper/checkpoints приведены в JSON. Полные requests/responses,
кандидаты и logs остаются ignored/local-only/NOASSERTION. Task service и runner
остановлены, оставшихся Ollama processes нет. PR сохраняет обе попытки в `attempts`;
общий outcome `revise`, quality_evidence=false, training_ready=false.
