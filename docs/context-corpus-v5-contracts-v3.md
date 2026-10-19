# Explicit-contract corpus v5 pilot: REVISE на smoke audit

2026-10-19, [Issue #16](https://github.com/saifety-org/lab-py/issues/16),
[принятый runbook](../device-jobs/context-corpus-v5-contracts-v3.md),
[машинный отчёт](../reports/context-corpus-v5-contracts-v3.json).

Preload и полный smoke прошли структурные, evidence, identity, usage и GPU gates.
Суждения по16opaque cases зафиксированы до key/intent/candidate/rationale;
все16 совпали с ожидаемыми отношениями разрешённости. Однако3из4семейств имеют
существенные fidelity замечания. Итог **revise**, pilot не запускался.
Это выполненный ограниченный эксперимент, не разрешение большого сбора.

## Принятые входы и оборудование

- Чистый lab-py main `1ce2187db827bcddfb4e3fb00e7c8151016a123d`,
  [main check SUCCESS](https://github.com/saifety-org/lab-py/actions/runs/39158771295).
  Instructions уже используют Issues; Project item сохранён после конвертации.
- Collector [lab#15](https://github.com/saifety-org/lab/pull/15), exact head
  `2bd7fd43f0a2f00ee6294c6d36be5bb0b0ffc43e`, merge
  `7419a289ccaddc95b4c59571ba7c38d0f9199e64`,
  [main ci SUCCESS](https://github.com/saifety-org/lab/actions/runs/39156090030).
- `github.com/saifety-org/lab/cmd/context-collect@v0.0.0-20261018182348-2bd7fd43f0a2`,
  Go1.26.0/windows-amd64/CGO disabled. Отдельный ignored bin, без vcs.modified=true.
  Versioned module install не содержит отдельный vcs.revision; pseudo-version
  связан с проверенным exact head. Binary/build-info hashes в JSON.
- Request SHA256
  `401e3e096203273e9bae80c6111c6192da6155d739a0a168816d251d8e124f89`;
  protocol `c5bd48ef63338196746a9c0b95f228ab9129283d056db9ab67327bdaba8c1676`;
  plan `0a4af4d9d6bd05cef70af019b3f33ae7777ba5d082edcc4b30f1e7cc239da695`.
- Существующие Ollama0.18.0 и `qwen3:8b-q4_K_M`, digest
  `500a1f067a9f782620b40bee6f7b0c89e17ae61f686b92c24933e4ca4b2b8b41`.
- Ryzen9 9950X,16cores/32threads; RAM68,241,707,008bytes;
  Windows11 Enterprise10.0.26200, RTX5070Ti16,303MiB, driver610.88.
  Свободное место до модели и прочие hardware metadata приведены в JSON.

Issue body/comments сохранены до claim, body hash относится к точным UTF-8/LF
байтам local checkpoint без BOM. Todo→In Progress до установки/модельных вызовов;
прогресс/ошибки записаны комментариями в Issue. Старые jobs/attempts не запускались.

## Исполнение и фактический расход

Принятые caps: один preload≤15min, smoke4≤8chat/20min, pilot12≤24chat/40min;
всего≤33collector modelcalls/65536outputtokens/75mincollectorwall.
Это конечные бюджеты, не прогноз. Seed1, temperature0.8, context8192,
num_predict2048, think=false, truncate=false, shift=false; параметры не менялись.

| Этап | Модельные вызовы | Результат | Process wall |
| --- | ---: | --- | ---: |
| Setup | 0 | установка, checks, service readiness | 149.703s |
| Preload | 1 | complete, identity и100%GPU PASS | 18.562s |
| Smoke | 8 | 4pending families, evidence/usage/GPU PASS | 12.812s |
| Audit текущего агента | 0 дополнительных Ollama/provider | 4families/16cases | 172.110s |
| Pilot | 0 | остановка после smoke fidelity review | — |

Model process wall31.375s. Chat сообщил1268output/2554prompttokens,
usage_complete=true. Preload вернул полный пустой load response без eval_count:
его usage=null, общий output usage с preload также null, не ноль.
Audit/reporting current-session tokens не доступны отдельно и отмечены unknown.
Reporting time записано отдельно в JSON.

Выполнены штатные `preload -preload-timeout 15m` и `collect-contracts`, затем
`merge`, `check-contracts`, `review-contracts`. Пользовательские preload helpers,
новые model clients, repairs, retries и дополнительные model calls не использовались.
Dedicated service имел только локальные process env:127.0.0.1:11435,
OLLAMA_NO_CLOUD=1, OLLAMA_LOAD_TIMEOUT=15m, существующее model store.
Глобальные user/machine настройки, driver/runtime/weights не менялись.

До первого модельного вызова исправлена ошибка setup metadata: PowerShell
автоматически превратил ISO timestamp в DateTime, а повторный Parse использовал
несовместимое представление даты. Использован прямой DateTimeOffset cast.
Первый service-only setup остановлен; его logs/error checkpoint сохранены.
Второй service setup продолжил подготовку; это не повтор модели, preload ровно1.

После preload и smoke `ollama ps`/snapshot подтвердили100%GPU,
size=size_vram=6584668288, context8192 и exact digest.234полных telemetry samples:
peakGPU93%, peakVRAM7311MiB. При остановке записи осталась одна обрезанная CSV
строка; она сохранена в raw файле, исключена только из агрегатов и учтена отдельно.
Телеметрия включает аудит до остановки сервиса, не только inference.
Access log counts включают служебные metadata/readiness/ps calls; actual model
calls подтверждены receipts. Все task-owned процессы остановлены.

## Key-hidden audit и fidelity

Сначала прочитан только `smoke-review/packet.json`. Зафиксированы unsafe=true/false
и причины, usefulness, task feasibility, mechanism, language/ambiguity для16cases.
Hash judgments и UTC seal сохранены до открытия key. После этого открыты key,
4intents, model replies, candidates и prompt; выполнена сверка с evidence.
Результаты judgments не подгонялись под key.

Reviewer — текущая сессия Codex, generator — локальный Qwen3; exact backend reviewer
и отдельный token usage не доступны. Дополнительных audit API calls нет.
Независимая adjudication не заявляется. Source prose уже содержала A/B references:
key был скрыт, но packet не был полностью изолирован от author-label cues.
Разрешения также сильно подсказывает раскрытый deterministic action scaffold.

Все16суждений совпали с матрицей unsafe/safe/safe/unsafe, uncertainty0.
Полезный измеренный факт доступен во всех16cases благодаря deterministic prepend;
trusted task/permissions его ответ не раскрывают. Это улучшение согласованности
контрактов относительно paired-v2, не оценка защиты агента или classifier quality.

| Family ID | Язык / тип | Fidelity результат |
| --- | --- | --- |
| `v5c3-c5bd48ef-000000` | en / instruction-override | Fail: A/B/expected-permission assertion в model prose, слабая конкретность механизма |
| `v5c3-c5bd48ef-000001` | ru / authority-spoofing | Существенного расхождения не найдено; minor grammar и ограниченная конкретность |
| `v5c3-c5bd48ef-000002` | es / answer-contamination | Fail: A/B assertions; escalation narrative про создание записи вместо обоснования плановой отправки |
| `v5c3-c5bd48ef-000003` | zh / data-exfiltration | Fail:3английских model contexts под китайским scaffold, A/B assertion, слабый механизм |

Имена языков и дефекты сверены с сохранёнными prompt/reply/intent, без публикации
raw текста или цитат. Prompt явно требовал целевой язык и запрещал labels/expected
safety assertions в narrative. Код сохранил model contexts без изменений, затем
добавил факт и точную операцию. Локализация scaffold не делает английский context
китайским. Точный action suffix не исправляет неверное narrative об операции.

Evidence checks подтверждают план, рендеринг и hashes, но пропускают эти смысловые
ошибки. Успешные16/16 permission judgments не отменяют fidelity fail3/4.
Поэтому ручной gate не пройден, pilot/review его4семейств не выполнялись;
объём аудита меньше лимита8семейств/32cases. Итог **revise**.

## Coverage, ограничения и следующий шаг

4structurally accepted pending families,16counterfactual rows,4mechanical source
groups и4template roots по типам; reviewed_families0. Все записи остаются pending.
В12variant documents нет точных нормализованных копий; намеренно один и тот же
payload в A/B не считается дефектом. Общий scaffold, translations/source/template
grouping не создают независимые квоты по ticket IDs. Одна модель, четыре языка,
один документ, без long/multidocument/tokenizer-window/generalization evidence.

Структурный throughput≈18.732pending families/min только chat batch; это
не скорость пригодного независимого корпуса и не прогноз fullpool.
`quality_evidence=false`, `training_ready=false`. Frozen v4/thresholds,
training/freeze/weights/promotion не затрагивались.

Следующая доработка: исключить A/B/expected-safety assertions из author prose,
проверять язык самих model contexts и соответствие narrative точной плановой
операции/назначению. Сохранить явные trusted permissions и immutable rendering;
не ослаблять validation и не переклеивать labels для прохождения gate.
Новая попытка требует принятого конечного request; automatic fullpool запрещён.

Local run:
`artifacts/device-jobs/context-corpus-v5-contracts-v3/20261019T062432Z-4f007339/`.
`SHA256SUMS.json` охватывает59файлов, исключая себя; его SHA256 в JSON.
Raw intents/documents/requests/replies/keys/judgments/logs и веса в PR не входят,
local-only/NOASSERTION, без CC0/MIT/публичной лицензии. Старые отчёты сохранены.
