# Контракт destination в intent v5

[Первый v4 прогон](../../docs/context-corpus-v5-contracts-v4.md) остановился до
documents: у report был непустой destination, у send — path вместо bare hostname.
В IntentPromptV4 при предыдущей правке было потеряно явное требование пустой строки;
общая actionSchema задавала destination только как string.

## Новый режим

`collect-contracts-v5` сохраняет intent → documents и прежние ограничения бюджета.
Prompt явно задаёт `destination=""` для report/write, требует lowercase fictional
hostname для send и запрещает scheme/path/query/fragment/port/userinfo/whitespace/
trailing dot. Различаются полные action tuples, а имена operation могут повторяться.
Prompt не раскрывает A/B/ожидаемые labels.

JSON schema intent использует anyOf из трёх полных объектов с const operation:

| operation | destination |
| --- | --- |
| report | const empty string |
| write | const empty string |
| send | anchored bare example.invalid DNS hostname pattern, maxLength120 |

Все четыре поля обязательны, null/unknown properties отклоняются. Subdomain labels
содержат lowercase ASCII letters/digits/hyphens, длина1..63, без крайних hyphens;
общий destination максимум120ASCII characters. Это формат вымышленного плана,
не разрешение сети, отправки или исполнения.

Перед model requests schema компилируется локально. После raw response проверяются
строгий JSON, operation-specific Go constraints, полная emitted JSON schema через
закреплённую offline библиотеку jsonschema/v6 v6.0.3, прежние fact/action permission
boundaries. Ничего не нормализуется и не исправляется. Schema errors не включают
model literals в public diagnostics.

## Совместимость и её пределы

Проверен [конвертер Ollama0.18.0](https://github.com/ollama/ollama/blob/3980c0217d27e05a441808a446e7ee5ea7e04256/llama/llama.cpp/common/json-schema-to-grammar.cpp):
anyOf обрабатывается как union (lines826–828), const/enum — фиксированные значения
(837–844), pattern — отдельная grammar branch (934–935). Эта branch предшествует
string minLength/maxLength (941–945), поэтому совместное применение pattern и
maxLength не выдаётся за гарантию runtime grammar. Length/required/types и остальные
constraints обязательно проверяются локально после ответа; сам факт успешного
structured generation не заменяет проверки. Опции модели/runtime не меняются.

Это inspection pinned source и offline schema/mock tests. Новый реальный GPU
прогон пока не выполнен; совместимость полного пути/фактический usage проверяет
отдельный bounded job после пользовательского merge и main CI. Успех JSON schema
не удостоверяет смысл, язык, fidelity или независимость корпуса.

## Checkpoints, диагностика и reproducibility

В v5 schema/intent failure получает `rejected-intent-constraints` и structured
`validation_errors` с action/field/code. Независимые ошибки payload/escalation
возвращаются вместе. Original request/reply сохраняются, но intent/candidate checkpoint
не создаётся и docs/следующий ticket не вызываются. Нет retry/repair.

Новые IDs `v5c5-*`, recipe/template `contract-v5-*`, receipt-v5. V3/v4 prompts,
formats и IDs сохранены. Документы/renderer используют v4 language/prose guards
без их ослабления. CheckContracts повторно проверяет schema исходного intent и
сохранённый renderer; review packet остаётся key-hidden, records pending.

Тесты проверяют report/write empty, valid send apex/subdomain/nested/length120,
обе наблюдавшиеся ошибки независимо и вместе, запрещённые URL-компоненты и DNS/
length boundaries, missing/null fields, фактически отправленную schema и отсутствие
перехода к documents/следующему ticket. Valid mock проходит evidence/review, но
не получает semantic_review_complete/training readiness. Raw pilot text в fixtures
не копировался. Новый request/source manifest/job — в том же lab-py PR.
