# Сбор контекстных кандидатов v5

Новый генератор: [двухэтапный paired-v2](contextual-collection-paired-v2.md).
Одноэтапный `collect` сохранён для воспроизводимости первого STOP-пилота;
новые GPU-задания используют явный `collect-paired` и отдельно учитываемый preload.

[Карточка корпуса](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=272440792).
Этот этап расширяет инструменты и входной пул. Статистические квоты не закрываются
размножением старых14семейств. [Протокол](../datasets/contextual/v5/protocol.json)
и старые версии данных сохраняются; source hashes и declared lineage записываются.

## Raw contract и проверки

Один `Family` содержит operator-assigned ID/source root/template root/provenance
 group, primary type/language/channel, tags, `document_trust=untrusted`, лицензию,
origin author/recipe/model digest/prompt/response hashes и `review_status=pending`.
Origin ID не является доказательством независимости. Common local generator и
общая схема authoring остаются ограничениями происхождения, даже при разных жанрах.

`content` содержит две trusted задачи с отдельными permissions и три массива
документов одинаковой длины:

| Сочетание | Предполагаемая метка для последующего ревью |
| --- | ---: |
| authorized task + payload | 1 |
| authorized task + benign payload | 0 |
| permitted task + тот же payload | 0 |
| permitted task + escalation payload | 1 |

Оба задания встречаются с обеими метками; один документ с разными разрешениями.
Это гипотезы автора/генератора: schema validation не подтверждает семантику меток.
Benign может включать разрешённое действие, цитату, документацию или код;
permission не сводится к добавлению «not» или смене одного слова.

Проверяются размер/UTF-8/закрытая JSON schema/duplicate keys, document boundaries,
непустые permission contracts, явное происхождение и отсутствие самоповышения
review. Данные не исполняются; модель не выбирает файл, ID, лицензию или review status.
Типы v5 соответствуют новому протоколу; старые v4 taxonomy/files не переименовываются.

`check` объединяет явные source roots, шаблоны, derivations и точные NFKC/casefold
копии документов. Выдаёт отдельно candidate IDs, source-connected groups и четыре
предполагаемых строки на candidate. Семантические/лексические near-duplicates,
переводы, scope разметки и actual tokenizer/window counts проверяются на следующем
этапе. `training_ready=false` и blocking gaps сохраняются даже при большом числе IDs.

## Offline команды

```sh
make plan-context-v5
# INPUT — локальный файл с raw candidates, не файл личных документов.
go run ./cmd/context-collect check -input INPUT.jsonl -out artifacts/coverage-new.json
```

Plan требует точный protocol SHA256 и запрещает overwrite. Квоты — резервирование
авторских заданий, **не** утверждение о замороженных test splits. Raw pool не
поступает в fit; confirmation не оценивается моделью этим сборщиком.

По текущему решению новые кандидаты остаются локальными с `license=NOASSERTION`.
Это отсутствие публичной лицензии, а не лицензия для коммерческого использования.
Сбор, `merge` и `check` работают в этом режиме; raw данные и traces не коммитить.
CC0 или MIT этим решением не назначаются. Возможная публикация данных потребует
отдельного выбора условий и разрешения.

Только после такого отдельного решения новые оригинальные кандидаты можно
разложить по типам для PR:

```sh
go run ./cmd/context-collect publish -input INPUT.jsonl -out artifacts/publish-new
```

`publish` проверяет структуру/ID и отклоняет `NOASSERTION` **до** создания каталога.
Поддержаны декларации CC0-1.0 и MIT для оригинальных synthetic кандидатов; сторонние
источники/лицензии этим инструментом не импортируются и не перелицензируются.
Разрешение на публикацию не является модельным review или quality approval.

## Явный запуск на GPU-ПК

Код CLI — Go в lab, job/runbook и pins — в lab-py, статус — в Project.
До запуска принять PR инструментов и request, проверить main CI и режим хранения;
Backlog не означает разрешение генераций. ПК-агент читает accepted инструкцию,
claim In Progress, выполняет bounded smoke/pilot, публикует отчёт/ошибки через PR.
Полный пул собирается отдельным заданием **после** измерения качества/затрат pilot.

Только команда `collect` делает запросы к LLM. Нужны literal HTTP loopback,
предварительно установленный local GGUF tag/digest и Ollama0.18.0. Proxy environment
не наследуется, redirects/cloud/identity drift запрещены. CLI не устанавливает
Ollama, не скачивает модель, не использует hosted provider или реальные tools.
HTTP local server/model считаются operator-trusted.

Запросить максимум64tickets за один запуск; max tokens256..8192, whole job≤2h,
один request≤180s. Request задаёт точные start/count/token/wall caps и выбранную
декларацию прав. Для локального пилота явно передаётся `-license NOASSERTION`;
`publish` с ней запрещён. Generation temperature0.8/seed1, context8192, think/truncate/shift=false;
это создание разнообразных кандидатов, не повтор старого agent benchmark temp0.

После каждого полученного JSON response сохраняется checkpoint0600, включая
truncated generation/невалидный content. HTTP/JSON envelope errors отражаются
incomplete receipt; не утверждать наличие полного raw ответа там, где его нет.
При malformed output, лимите, изменении identity или writer failure сбор
останавливается без скрытого retry. Успешный checkpoint не перезаписывается.
Receipt хранит attempted/accepted/token counts, time/identity/plan hash/лицензию.

`merge -from CHECKPOINT_DIR -out NEW.jsonl` принимает только regular candidate
files с закрытой schema, отклоняет duplicate IDs и никогда не переписывает output.
Относительный артефактный каталог и binary/source/request SHA публикуются в карточке;
сырой output не выдаётся за проверенный корпус. Labels/language/window/provenance
требуют аудита. Большая generated выборка общего автора не равна независимой
естественной выборке и не является доказательством качества анализатора.

## Проверки инструментов

`make ci-test lint ci-build`: race/regression, native+ONNX lint/build.
Mock HTTP tests проверяют no proxy/redirect/cloud, hash/identity drift, truncation,
schema/UTF-8/duplicate keys, quotas, transitive groups, partial checkpoints,
отказ без лицензии и защиту существующего output. В CI нет real LLM/GPU run.
