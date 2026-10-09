# sAIfety lab-py

Python-инструменты обучения, оценки и ONNX-экспорта для sAIfety.
Приложение остаётся на Go; готовому приложению Python не требуется.
Датасеты имеют один источник — [lab](https://github.com/saifety-org/lab).
Готовые production-модели публикуются отдельно в
[prompt-injection-model](https://github.com/saifety-org/prompt-injection-model).

## Воспроизводимый native baseline

Нужны Python 3.12.13, uv 0.11.21 и Go 1.26+. Команды выполнять из корня репозитория:

```sh
uv sync --locked
make bootstrap  # установить закреплённые Go-команды, без модели DeBERTa
make prepare    # загрузить проверенные данные; повторить разбиение из lab
make train      # обучить Python-кандидата на канонических Go-признаках
make compare    # проверить Python/Go parity и оценить кандидат и shipped model
make misses     # воспроизвести известные пропуски настоящего Go-сканера
make check      # Ruff, тесты, wheel/sdist
```

`bootstrap` закрепляет версию Go-команд; `sources.lock.json` задаёт коммит lab,
пути и SHA256. Загрузки выполняются только явными bootstrap/prepare/sync-командами.
Исходные данные кэшируются в исключённом из git `data/source`; подготовленные
выборки — в `data/comparison`. Код генератора там нужен для проверки provenance,
Python его не исполняет. Признаки и сканер вызываются через JSONL `model-bridge`;
копии Go-алгоритмов извлечения признаков здесь нет.

Веса, метаданные, predictions и отчёт пишутся в `artifacts/`. Python-кандидат
не заменяет production-веса. Его SGD воспроизводим в закреплённом Python-окружении;
одинаковый seed не означает побайтовое совпадение обучения Python и Go.
Проверяется совпадение **инференса одного и того же кандидата**.

### DeBERTa и переводы

Для DeBERTa нужен уже подготовленный runtime/model bundle приложения:

```sh
uv run --locked saifety-lab bootstrap --onnx
uv run --locked saifety-lab compare-native --deberta
```

Эксперимент завершается при отсутствии модели, ошибке инференса или несовместимости.
Fallback и правила детектора не используются при сравнении классификаторов.
Примеры вне общего token window исключаются для всех сравниваемых моделей;
ID и причина каждого исключения сохраняются в отчёте.

## Трансформеры и ONNX

Тяжёлые зависимости подключаются отдельно:

```sh
uv sync --locked --extra ml
uv run --locked --extra ml saifety-lab train-transformer \
  --model /path/to/local/checkpoint \
  --train /path/to/train.jsonl --validation /path/to/validation.jsonl \
  --out artifacts/transformer/checkpoint
uv run --locked --extra ml saifety-lab export-onnx \
  --checkpoint artifacts/transformer/checkpoint --out artifacts/transformer/onnx
uv run --locked --extra ml saifety-lab bootstrap --onnx
uv run --locked --extra ml saifety-lab check-onnx \
  --bundle artifacts/transformer/onnx --data /path/to/parity.jsonl \
  --library /path/to/libonnxruntime --out artifacts/transformer/parity.json
uv run --locked --extra ml saifety-lab compare-onnx \
  --bundle artifacts/transformer/onnx --parity artifacts/transformer/parity.json \
  --library /path/to/libonnxruntime --deberta
```

Используются локальные safetensors checkpoints, без remote code и автоматической
загрузки моделей. Обучение работает на CPU. Для текущего Go backend поддерживаются
бинарные классификаторы с входами `input_ids`, `attention_mask` и выходом `logits`;
экспорт с дополнительными входами отклоняется. Это исследовательский text-only
контракт; анализ задачи пользователя и типов атак — следующий этап.

Перед сравнением ONNX-кандидата обязателен успешный parity report, относящийся
к тем же модели, manifest и runtime. Проверяются tokenizer length, оценки Python
ONNX Runtime и Go. Обычные CI тесты используют tiny checkpoint, созданный локально;
они проверяют технологический путь, а не качество новой production-модели.

Для эксперимента с переводом используется локальный NLLB-style checkpoint и
явно заданный язык исходной выборки:

```sh
uv run --locked --extra ml saifety-lab translate \
  --model /path/to/local/translation-checkpoint --data /path/to/evaluation.jsonl \
  --source-language rus_Cyrl --out artifacts/translations.jsonl
uv run --locked saifety-lab compare-native --translations artifacts/translations.jsonl
```

Оригинал сохраняется; оценки исходного и переведённого текста объединяются через
max для одного native-кандидата, после чего отдельно калибруется порог на validation.
Автоматическое определение языка и смешанные языки здесь ещё не реализованы.
Перевод не передаётся агенту и не исполняется.

## Протокол оценки

- Разбиение и удаление близких дублей повторяются Go-командой из lab; переводы
  не создают новые splits.
- Пороги выбираются отдельно для каждой модели только по validation при общем
  бюджете FPR (по умолчанию 1%); test используется для окончательного отчёта.
- Отчёт содержит per-language/per-category recall/FPR, 95% интервалы и количество примеров,
  версии/хеши компонентов и пропущенные из-за окна примеры. FPR budget на validation
  не гарантирует такой же FPR на test. Если полезный порог не найден и модель
  не сигнализирует ни на одном входе, отчёт помечает operating point `no_alerts`.
- Старые BIPIA labels не содержат полный контекст задачи/источника: результаты —
  диагностическая оценка классификаторов. Метрики защиты реального агента требуют
  отдельного стенда. Десять известных пропусков не считаются независимым test.
- Результаты Python-кандидата сравниваются с настоящим Go-инференсом. Performance
  измерять Go benchmark из lab, а не по времени запуска subprocess из Python.

## Разработка

```sh
make bootstrap
make check
uv run --locked --extra ml saifety-lab bootstrap --onnx
make test-ml
```

CI выполняет отдельные lint/test/build jobs и offline ML smoke с tiny transformer.
Go interoperability обязателен в CI; локально тест пропускается, если bootstrap
ещё не запускался. Полное обучение и скачивание production-моделей в CI не выполняются.
Связанный Go-мост: [lab PR #4](https://github.com/saifety-org/lab/pull/4).

Первый диагностический результат и его ограничения: [docs/first-run.md](docs/first-run.md).

## Контекстная модель

[Контекстный workflow](docs/contextual.md): закреплённый многоязычный корпус из lab,
обучение context/payload-only/task-only native ablations и отдельная оценка сканера.
[Первый прогон](docs/contextual-first-run.md) показал ограничения линейной модели;
это ещё не production multilingual detector.
