# Границы Python-лаборатории

`lab-py` принимает закреплённые данные lab и локальные checkpoints, обучает
кандидатов и формирует артефакты. Production-приложение не зависит от Python.

Путь кандидата: данные lab → обучение в lab-py → артефакт с manifest → Python/Go
parity → свежая оценка → отдельное решение о включении → явное обновление
закреплённой модели приложения. Ни одна команда не публикует веса автоматически.

В manifest сохраняются источник модели, хеши данных и токенизатора, контракт
входа/выхода, индекс attack, окно/aggregation, threshold/calibration, параметры
обучения, права распространения и проверка совместимости. При изменении схемы
приложение должно явно поддержать её.

## Принятая основа

Контекстный корпус v4 и модельное ревью, context/payload/task ablations,
исследовательский E5 ONNX/export/parity и Go `pkg/contextscan` уже реализованы.
Contextual inference в приложении — opt-in shadow, без изменения текущей защиты:
observed v4 FPR 1,43% превышает ориентир 1%. Structured MCP scanning, blocklist
проверки и изолированный agent harness также существуют.

Приняты [первый локальный LLM-прогон](agent-harness-ollama-first-run.md) и
[повторный прогон отказа](agent-harness-ollama-refusal-v2.md). Нейтральный отказ
устранил наблюдавшиеся запрещённые виртуальные отправки; safe completion не выросло.
Малый development-набор и baseline ASR=0 не доказывают качество защиты.

## Следующий этап

[Протокол качества](analyzer-evaluation-protocol.md) и
[машинная спецификация](../analyzer-evaluation.protocol.json) задают следующие
квоты, calibration и численные gates. Они не реализованы полностью текущими CLI.

1. Новые разнообразные контекстные данные и agent-сценарии, модельное ревью,
   provenance/leakage checks и свежие confirmation families. Старые увиденные
   test-примеры и переводы их производных остаются development/regression.
2. Доверенный task/permissions contract с session/update semantics; существующий
   operator `-task` не заменять угадыванием задачи из tool args/results или файлов.
   Документы остаются недоверенными независимо от языка/источника.
3. Whole-input обучение и calibration с реальным Go windowing; новые manifests,
   parity и отдельный экспериментальный candidate-policy arm. Определение языка
   и повелительного наклонения не служат обязательными фильтрами детектора.
4. Scope карантина и проверяемая доставка полезных данных: классификация документа
   не локализует безопасный span, ошибки/partial/unknown никогда не означают clean.
5. Свежая модельная/агентная оценка, CPU budgets, реальная клиентская интеграция
   и reviewable go/no-go. Перевод как fallback — отдельный эксперимент после
   основного пути, с сохранением оригинала и без внешней передачи по умолчанию.

Задачи и точные зависимости — в
[GitHub Project](https://github.com/orgs/saifety-org/projects/1).
[Device workflow](device-work.md) хранит задания ПК в репозитории; подготовка кода
и обычный CI не запускают обучение или LLM на Mac. Данные/Go research остаются
в lab, runtime в sAIfety, model artifacts в prompt-injection-model.
