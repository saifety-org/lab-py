# v5: сбор кандидатов контекстного корпуса

Это новая стадия сбора, **не reviewed/frozen training dataset**. v1–v4 не меняются.
Численные квоты задаёт принятый [протокол lab-py](https://github.com/saifety-org/lab-py/blob/d023d19d59e62f6c5e23f8e68651179ab3748b19/docs/analyzer-evaluation-protocol.md).
Его точная копия — `protocol.json`, SHA256
`c5bd48ef63338196746a9c0b95f228ab9129283d056db9ab67327bdaba8c1676`.

[Инструкция сбора](../../../docs/contextual-collection-v5.md) описывает Go CLI,
формат кандидата, лицензирование, checkpoints и отдельное задание GPU-ПК.
После STOP первого пилота двухэтапный paired-v2 получил REVISE: исходный payload
не стал разрешённым под задачей B. Следующий ограниченный эксперимент использует
[явный план действий и контрактов](../../../docs/contextual-collection-contracts-v3.md)
перед генерацией текста; семантическое качество ещё не подтверждено.

`make plan-context-v5` работает offline. 6 380 authoring tickets резервируют:
700 train, 2 400 validation, 2 400 confirmation, 140 development,
560 exploratory, 160 mixed/unknown/code/encoded и 20 over-limit stress.
Это **цели сбора**, а не уже созданные независимые семьи или назначенные splits.
Данные, придуманные по одному шаблону, не становятся разными семьями из-за ticket ID.

Новый raw contract `saifety-context-candidates-v1` намеренно отличается от
подготовленных `context-json-v2` samples. Старый `context-corpus`/training pipeline
не принимает его как прошедший review. Подготовка train/validation/confirmation
и token-window coverage принадлежат отдельной задаче аудита/freeze.

Новые данные пока собираются только локально с явным `-license NOASSERTION`.
Публичная лицензия им не назначается; CC0 не выбрана. `NOASSERTION` означает
отсутствие декларации лицензии, а не готовность к распространению или обучению.
`collect`/`merge`/`check` допускают этот режим; `publish` откажет до записи
выходного каталога. Raw данные и traces не коммитить. Возможная публикация требует
отдельного решения об условиях; ни CLI, ни модель не выбирают их автоматически.
Декларация лицензии не является проверкой происхождения/прав; это тоже часть аудита.

Исходные authored/model-generated кандидаты не получают метку `model-reviewed`.
В coverage всегда `training_ready=false`, `reviewed_families=0`, пока это raw
collection stage. Общие source/template/derivation roots и точные нормализованные
копии связываются транзитивно; семантический аудит может уменьшить число групп.
Языки/типы/каналы пока декларативные; это не проверка смысла и переводов.
