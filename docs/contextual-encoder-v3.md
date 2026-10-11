# Multilingual encoder: первый прогон v3

Дата: 2026-10-11. Кандидат **не принят для production**. Обучение и сравнение
состоялись, но task-only ablation выявила shortcut в синтетическом корпусе.
Следующий шаг — новая версия корпуса с контролем зависимости метки от задачи.
Замороженный v3 остаётся наблюдённым диагностическим набором.

## Модель и протокол

Использован [intfloat/multilingual-e5-small](https://huggingface.co/intfloat/multilingual-e5-small/tree/614241f622f53c4eeff9890bdc4f31cfecc418b3):
XLM-R encoder, 12 слоёв, hidden size 384, MIT. Multilingual pretraining не является
доказательством обнаружения инъекций на всех языках. `encoder.model.lock.json`
закрепляет revision и SHA256 весов, tokenizer и model card. Загружаются только
safetensors, `trust_remote_code=False`, публичная загрузка с `token=False`.

В соответствии с [описанием E5](https://huggingface.co/intfloat/multilingual-e5-small/blob/614241f622f53c4eeff9890bdc4f31cfecc418b3/README.md)
используем `query: ` и masked mean pooling с L2-нормализацией. Поверх encoder —
обучаемая linear head на два класса. Обучается **весь encoder вместе с head**.
Вход полного варианта — ровно `context-json-v2`: задача и документы с `kind`,
`trust`, `text`. Labels, IDs, rationale, язык, review и split не входят в признаки.
`user-provided` обозначает происхождение документа, не приоритет инструкции.

До первого test-прогона зафиксирован `encoder.protocol.json`: один seed (1),
одна конфигурация, 2 epochs, AdamW lr=2e-5, weight decay=.01, batch=8, float32 MPS.
Для context, payload-only и task-only одинаковые начальные веса, seed, optimizer
и бюджет шагов; выбирается последний checkpoint, поиска гиперпараметров нет.
Validation используется для общего правила порогов: максимальный recall при
наблюдаемом FPR ≤1%, затем меньшая FPR и больший порог. Пороги всех моделей
сохраняются **до** вычисления test scores.

Исходные данные: [lab@28c2f9d](https://github.com/saifety-org/lab/tree/28c2f9d1ee016a1721b19627a17b7c9569a7d249/datasets/contextual/v3).
280 train / 280 validation / 280 test, по 14 семей в каждой части, по 140 атак
и benign. Все 252 исторических примера — development, не обучение или test.
Пять вариантов языка **документов**: EN/RU/ES/ZH и mixed EN/RU; задачи английские.
70 authorization pairs в test: одинаковый документ, разные разрешённые задачи.
Зависимые переводы и варианты одной семьи не считаются независимыми примерами.
Review выполнен моделью-автором; это не слепая независимая оценка.

`encoder.sources.lock.json` закрепляет review, holdout, сценарии, исторический
корпус и Go tools того же commit. При подготовке, перед обучением и сравнением
повторяются канонические review/history/freeze gates и сравнение prepared hashes.
Никаких автоматических re-freeze. Вход длиннее 512 tokens останавливает весь
прогон; скрытого усечения или исключения неудобных строк нет. В этом прогоне
исключений не было. Старый v1 lock и его workflow сохранены.

## Что именно сравнивается

Shipped native и `protectai/deberta-v3-base-prompt-injection-v2` — готовые модели
приложения, без дообучения на v3. Они получают каждый исходный документ отдельно,
score сценария — максимум по документам; ни задачу, ни trust они не видят.
Их пороги перекалиброваны по тому же validation и тому же правилу. Это сравнение
прикладных конфигураций на одинаковых строках, **не рейтинг архитектур с равным
обучением**. Парные документы без контекста неразличимы для обеих моделей.

Подробные числа, SHA, версии, family intervals, языки, типы и пары:
[`reports/contextual-encoder-v3.json`](../reports/contextual-encoder-v3.json).
PR-AUC здесь означает non-interpolated average precision; tied scores входят
в кривую целым блоком. Интервалы — exploratory bootstrap целых семей, 1000
повторов, seed=1. Вырожденные интервалы `null`, не нулевой доверительный предел.

| Вариант | Validation TP / FP | Test TP / 140 | Test FP / 140 | Test AP | Обе строки пары верны / 70 |
| --- | --- | --- | --- | --- | --- |
| Encoder context | 132 / 1 | 117 | 0 | 1.000 | 58 |
| Encoder payload-only | 2 / 1 | 4 | 2 | 0.666 | 0 |
| Encoder task-only | 120 / 0 | 130 | 0 | 1.000 | 65 |
| Shipped native raw | 0 / 0 | 0 | 0 | 0.480 | 0 |
| DeBERTa raw | 2 / 1 | 0 | 1 | 0.490 | 0 |

Recall context 83.6%, exploratory family interval [67.9%, 96.4%]; task-only
92.9%, [78.6%, 100%]. У context и task-only порядок test scores идеален (AP=1),
но порог перенесён с validation, поэтому recall ниже 100%. У shipped native
выбран `no_alerts` (>1) — полезного operating point на validation не найдено.
140 зависимых benign строк и 14 семей недостаточно для гарантии FPR 1%.
Нулевая test FPR не доказывает защиту в реальном приложении.

**Ключевой вывод:** задача сама предсказывает метку лучше полного контекста.
Высокий context AP пока нельзя считать доказательством анализа документа или
понимания полномочий. Нужен новый корпус с одинаковыми задачами и документами,
меняющими допустимость поведения, а также одинаковыми поверхностными шаблонами
задач для обеих меток. Коррекции не должны переписывать уже наблюдённый test v3.
Нет измерения live-agent attack success или сохранения полезного поведения.

## CPU latency и память

Apple M5, 10 CPU cores, 24 GiB RAM. Обучение MPS, оценка качества CPU.
Для latency: resident процесс, batch=1 сценарий, tokenization и JSONL IPC
включены; 5 warmups, три повтора первого примера каждой validation семьи.
Это 14 English attack representatives, не репрезентативный multilingual workload.
PyTorch intra-op=1, OMP_NUM_THREADS=1; Go/ORT использует штатные thread pools.
Framework и thread pools различаются: числа описывают данные конфигурации,
не изолированную скорость архитектур. Нельзя сравнивать их с MPS training speed.

| Вариант | p50 / p95, ms | Max sampled RSS, MiB |
| --- | --- | --- |
| Encoder context | 11.15 / 12.90 | 780.3 |
| Encoder payload-only | 5.60 / 5.89 | 776.8 |
| Encoder task-only | 4.02 / 4.97 | 772.8 |
| Shipped native raw | 0.123 / 0.138 | 17.6 |
| DeBERTa raw | 30.02 / 32.81 | 1919.1 |

RSS снимается после ответов и включает модель и runtime/interpreter; это не
lifetime peak и не память обучения. Startup с hashing и первым inference
записан отдельно в JSON. CPU-инференс без ускорителя у всех вариантов.

## Воспроизведение

```sh
uv sync --locked --extra ml
make bootstrap-encoder prepare-encoder fetch-encoder
make train-encoder compare-encoder
uv run --locked --extra ml python -m saifety_lab_py.encoder_benchmark \
  --bridge bin/encoder/model-bridge
```

Нужны Go/CGo, локальные cached DeBERTa ONNX/tokenizer/runtime, как в
[README](../README.md). Hashes использованных reference artifacts включены в
отчёт. Default protocol требует MPS; для другой машины создайте отдельный
protocol до эксперимента и передайте `--protocol`, не меняя протокол этого прогона.
Weights, downloaded sources и per-row predictions создаются в игнорируемых
`artifacts/` и `data/`. Повторная запись завершённого эксперимента/evaluation
запрещена; новый запуск требует отдельного `--out` и является воспроизведением
уже наблюдённого test, не новым blind результатом. Межплатформенная битовая
идентичность не гарантируется; threshold ties могут измениться.

В Transformers 4.57.6 есть ложное Mistral regex warning для сохранённого XLM-R
config. Regex не меняли: проверено равенство token IDs base/saved tokenizer
на всех 560 train/validation входах каждого view. Веса и tokenizer связаны SHA.

Базовая модель MIT; корпус имеет `NOASSERTION`, права распространения кандидата
требуют отдельного решения. Production-веса этим экспериментом не заменяются.
ONNX export/parity и Go context adapter для нового encoder — отдельные этапы;
их наличие само по себе не исправит обнаруженный shortcut.
