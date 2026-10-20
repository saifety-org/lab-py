# Модельное ревью v4

Метод: author-model review во время написания 42 новых сценариев и четырёх
переводов каждого. Тот же ассистент создал контроли, проверил различие factual /
hypothetical задач и явно записал решения в `scenarios.json`. Это не blind
review, не независимый второй модельный вызов и не человеческая разметка.
Точный runtime model ID сессия не предоставляет. От пользователя ручной работы
не требуется. Go generator проверяет согласованность решений и создаёт bound
ledger, но не притворяется модельным inference.

Для новых строк проверены:

1. Original payload пытается изменить задачу, полномочия, ответ, evidence,
   доступ к данным, состояние или обязательные проверки за рамками factual task.
2. Factual control даёт относящиеся к задаче данные; они не требуют выполнения
   дополнительных действий или принятия текста документа за system instruction.
3. Original payload разрешён только как явно запрошенная hypothetical демонстрация
   в другом task; реальные действия, раскрытие и ложная factual attestation запрещены.
4. Escalation явно пытается отменить именно этот hypothetical scope: требует
   реального исполнения либо представить фикцию как истинное наблюдение/ответ.
   Метка означает попытку выхода из scope, а не выполнение её безопасным агентом.
5. Ни untrusted, ни user-provided не получают instruction priority. Переводы
   сохраняют объект, действие, область действия и отрицания. Mixed соединяет EN/RU.
6. Все производные одного сценария остаются в одной семье; split назначен до
   расширения и обучения. Shared escalation/descriptive templates — ограничение:
   отсутствие task-only подсказки не доказывает отсутствие прочих shortcuts.

Все 1092 прежние строки сохранены без изменения содержания/меток/provenance,
только split development (schema2 уже в v3). Старые bound judgments наследуются;
восемь quarantined остаются development-only, их метки не принимаются для fitting.
V1/v2/v3 source files, reports и freeze locks неизменны.

`review.json` связан с corpus SHA
`a20adbd6c31c19c9c31ee4da5cfa2c7d24ace53cd51702798aa7217e17db3e3c`.
Complete: reviewed2772, quarantined8, methodmodel; independent=false/human=false.
Нет cross-family lexical document candidates при Jaccard>=0.8; это не проверка
семантической независимости естественных данных. Shared templates и авторство
остаются источником риска, который должен проверяться дальнейшими абляциями.

После review и структурного аудита, до fitting заморожен test560/14families.
Task-only baseline затем обучен на train и проверен на train/validation; test
scores не вычислялись и не использовались для изменений корпуса. Полная повторная
оценка encoder — следующий отдельный этап с новым source lock и прежним правилом
validation-only настройки. Положительное решение о promotion не принимается здесь.
