# Контекстный encoder на task-balanced v4

Контекстная модель обнаружила **246/280 атак при 4/280 ложных срабатываниях**;
AP=0.982858. Task-only AP=0.5, payload-only recall=47.14% против context=87.86%.
Это положительный результат контекстной абляции на v4. **Production promotion
не принят:** test FPR=1.43% превышает цель 1%, а корпус остаётся синтетическим.
Следующий допустимый этап — ONNX/parity исследовательского кандидата, без замены
поставляемой модели. Пороги и конфигурация после test не менялись.

## Результаты

[Машинный отчёт](../reports/contextual-encoder-v4.json) содержит все метрики,
пороги, hashes, интервалы по семьям, типы, языки и benchmark. Один финальный test,
без исключённых строк. Порог каждой модели выбран на validation при FPR<=1%.

| Модель / вход | Validation TP / FP | Test TP / FP | Test recall | Test FPR | Test AP |
| --- | --- | --- | --- | --- | --- |
| E5 context | 256 / 2 | 246 / 4 | 87.86% | 1.43% | 0.983 |
| E5 payload-only | 142 / 2 | 132 / 0 | 47.14% | 0% | 0.879 |
| E5 task-only | 0 / 0 | 0 / 0 | 0% | 0% | 0.500 |
| Shipped native, context | 9 / 2 | 6 / 0 | 2.14% | 0% | 0.600 |
| DeBERTa, context | 8 / 2 | 5 / 0 | 1.79% | 0% | 0.595 |
| Shipped native, raw | 8 / 2 | 0 / 0 | 0% | 0% | 0.674 |
| DeBERTa, raw | 4 / 2 | 2 / 0 | 0.71% | 0% | 0.653 |

Это одинаковое **правило validation calibration**, не одинаковый достигнутый
test FPR. Поэтому разницу recall нельзя выдавать за сравнение при равном
фактическом test FPR. AP дополняет этот результат без выбора нового test threshold.
E5 context family CI95: recall [80.00%, 94.29%], FPR [0%, 3.57%],
AP [0.9674, 0.9951]. Интервалы exploratory и основаны лишь на 14 семьях.

На 280 matched-task парах обе стороны верны в 242 случаях (86.43%).
На 70 authorization парах с одинаковым документом обе стороны верны в 53
случаях (75.71%); payload/task-only — 0, native context — 0, DeBERTa context — 1.
Контекстный кандидат одинаково классифицирует все 280 provenance пар;
средний absolute score delta=0.00250, max=0.09718.

Ordering отдельно недостаточен: task-only ставит attack выше benign в 70/70
authorization парах, но имеет AP=0.5 и ноль обеих верных решений. На matched-task
парах его scores равны. Это иллюстрирует, зачем сохранять все контроли вместо
заявления о понимании контекста по одному ordering показателю.

Слабее всего answer-contamination: recall=65%; verification-suppression=75%.
По языкам документов recall EN=85.71%, RU=92.86%, ES=96.43%, ZH=85.71%,
mixed=78.57%. Четыре FP приходятся на ES/mixed по два. Каждого типа в test
только две семьи; это диагностика, не повод подбирать исправления на этом test.

## CPU latency и память

Apple M5, 10 logical cores, 24 GiB RAM, macOS 26.6.2; Python 3.12.13,
torch 2.14.1, transformers 4.57.6. Последовательные resident CPU процессы,
один scenario за запрос, tokenization и JSONL IPC включены. Пять warmups,
три повтора первого примера каждой из 14 validation семей (42 измерения).
Это EN attack workload с короткими factual задачами, не весь test или long-input
stress. PyTorch intra-op/OMP=1; Go/ORT default thread pools. Разные frameworks
и thread pools не позволяют считать это сравнением только вычислительных ядер.

| Модель / вход | p50 / p95, ms | Max sampled RSS, MiB |
| --- | --- | --- |
| E5 context | 10.49 / 10.92 | 777.2 |
| E5 payload-only | 5.31 / 5.81 | 773.4 |
| E5 task-only | 3.81 / 3.97 | 773.6 |
| Shipped native, context | 0.219 / 0.253 | 17.9 |
| DeBERTa, context | 69.92 / 78.20 | 1875.6 |
| Shipped native, raw | 0.095 / 0.121 | 16.7 |
| DeBERTa, raw | 26.31 / 30.31 | 1922.8 |

RSS sampled после ответов включает model/framework/interpreter; это не peak
training memory или lifetime high-water mark. Startup с integrity hashing и
первым inference указан отдельно в отчёте. Benchmark использует только validation.

## Что проверяем

В v3 task-only был сильнее полного контекста. Принятый
[корпус v4](https://github.com/saifety-org/lab/tree/6d104a98377d54aefd8d7a69e0030be6d5dfbf81/datasets/contextual/v4)
сочетает каждую задачу с обеими метками при одинаковых метаданных источника.
Этот этап проверяет, помогает ли содержание документа и его связь с задачей.
Старые test v1/v2/v3 не используются как новое подтверждение качества.

Train/validation/test: по 560 строк, 14 новых семей, 280 attack + 280 benign.
Все 1092 исторические строки только development, восемь quarantined не входят
в fitting. По семь типов, документы EN/RU/ES/ZH/mixed ENRU; задачи английские.
На test: 280 matched-task, 70 identical-document authorization и 280 provenance
пар. Переводы и контроли зависимы, единица bootstrap — сценарная семья.

## Зафиксированный эксперимент

- Base: `intfloat/multilingual-e5-small@614241f622f53c4eeff9890bdc4f31cfecc418b3`,
  MIT. Safetensors и tokenizer проверяются по `encoder.model.lock.json`.
- Тот же training budget, что в v3: seed=1, два фиксированных epochs, batch=8,
  AdamW learning rate=0.00002, weight decay=0.01, gradient clipping=1,
  float32/MPS. Одинаковые стартовые веса и параметры для трёх views.
- Все параметры encoder и binary head обучаемые; masked mean pooling,
  L2 normalization, `query: ` prefix. Метки, IDs, rationale, язык, scenario tags
  и split не входят в model text.
- Context использует `context-json-v2`; payload-only — документы без задачи;
  task-only — только задачу. `user-provided` означает происхождение и не даёт
  instruction authority.
- Выбор checkpoint: последний фиксированный epoch. Одна конфигурация/seed,
  без HPO или выбора модели по test. `encoder-v4.protocol.json` сохранён до fit.
- Review/history/freeze/task-balance проверяются каноническим Go tool из
  принятого lab commit до preparation, fitting и evaluation; prepared hashes
  сверяются повторно. Re-freeze и изменение корпуса не выполнялись.
- Каждый вход должен целиком помещаться в 512 tokens. Переполнение останавливает
  весь запуск; обрезания, fallback и исключения примеров нет. На train/validation
  max E5 context=303, payload=172, task=81; DeBERTa context=314.

Train/validation scores каждого checkpoint записаны до test и связаны SHA с
candidate metadata. Все семь порогов выбраны на validation: максимальный recall
при observed FPR<=1%, затем меньший FPR и больший threshold. No-alerts — допустимый
результат, а не основание менять правило. Все пороги и validation scores сохранены
до первого test inference. Каждый финальный test pass сохраняется до вычисления
производных метрик; исправления отчёта не требуют повторной оценки test моделью.

Tokenizer round-trip после сохранения проверен по всем train/validation входам
трёх views: token IDs совпадают с базовым tokenizer. Transformers выдаёт прежнее
предупреждение Mistral regex для сохранённого XLM-R; regex не изменён. Оно уже
исследовано в v3, и здесь не используется как повод менять токенизацию по test.

## Сопоставимость моделей

Основные shipped native и ProtectAI DeBERTa ONNX references получают **тот же
context JSON**, что E5 context, без специфичного для E5 `query: ` prefix. Каждый
reference калибруется тем же правилом на тех же validation строках. Отдельные
`*_raw` references получают исходные документы, итог — max score по документам.
Они помогают объяснить вклад входа и не подменяют основной общий контекст.

Native/DeBERTa остаются готовыми поставляемыми моделями, не дообученными на v4.
У них другой training budget и нет обещания интерпретации JSON trust boundaries.
Результат сравнивает конкретных кандидатов и references; он не доказывает
превосходство архитектуры E5 над DeBERTa при равном дообучении. Модель, tokenizer,
runtime и Go bridge связаны SHA в машинном отчёте.

## Воспроизведение

```sh
uv sync --locked --extra ml
make bootstrap-encoder prepare-encoder fetch-encoder train-encoder compare-encoder
uv run --locked --extra ml python -m saifety_lab_py.encoder_benchmark
```

Нужны MPS для training и локальный DeBERTa/ONNX cache приложения. Evaluation и
benchmark используют CPU. Артефакты пишутся в `artifacts/encoder-v4*`; существующий
fit/evaluation не перезаписывается. Скачиваются только закреплённые исходные данные
и immutable base, без remote code. Обычные CI-тесты используют v1/v4 Go gates;
offline ML tests обучают tiny fixtures, а не повторяют исследовательский test.

Legacy v3 lock `encoder.sources.lock.json`, protocol `encoder.protocol.json`
и отчёт сохранены; воспроизведение исходной реализации доступно на
[принятом commit lab-py#3](https://github.com/saifety-org/lab-py/tree/6528262679d70404b694caf65470032f7ed38a22).

## Область результата

Корпус синтетический, автор и модельный ревьюер — один ассистент; это не blind
или независимый natural-data benchmark. Shared escalation wording, стиль, язык
и условности hypothetical/factual разметки могут давать другие подсказки.
Matched-task/payload/authorization диагностики выявляют часть таких зависимостей,
но не доказывают обобщение на произвольные атаки. Test публичен и виден автору.

Интервалы exploratory family bootstrap (seed=1, 1000 replicates); отсутствие
наблюдённой вариации означает `null`, а не точную границу. 14 семей и 280 зависимых
benign не подтверждают production FPR<=1%. Нет live-agent ASR или task completion.
Лицензия данных NOASSERTION; MIT base не решает автоматически происхождение
обученных весов. Веса остаются исследовательскими локальными артефактами.
Production-приложение и поставляемая модель этим PR не заменяются.
