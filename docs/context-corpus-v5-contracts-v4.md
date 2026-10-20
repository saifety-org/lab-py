# Исправленный collector v4: остановка на первом плане

**REVISE.** На native Windows/RTX5070Ti получено 0 кандидатов: первый intent
декодируется как JSON, но нарушает контракт действий. Documents и pilot не запущены.
Это не результат проверки качества языка, prose guards, классификатора или защиты.

[Задача Project](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=276945651), [принятый PR#33](https://github.com/saifety-org/lab-py/pull/33),
[машинный отчёт](../reports/context-corpus-v5-contracts-v4.json).
Accepted main `d37d12f13592e924c074b3275fffe90dca621c64`;
[main CI: все10jobs SUCCESS](https://github.com/saifety-org/lab-py/actions/runs/39377865779).

## Что произошло

1. Собран локальный collector из принятого checkout с проверкой source manifest
   `2523d0d91159e91ae36dc4a2b5691292760bb093ce2014202a2597351b73a07e` и binary SHA256 `1fa40a847c2c8d99873cde636707b53aed6bd5cd6236dca8d38edfa123e1c032`.
2. Прежние Ollama0.18.0 и qwen3:8b-q4_K_M, model digest
   `500a1f067a9f782620b40bee6f7b0c89e17ae61f686b92c24933e4ca4b2b8b41`. Драйвер610.88, контекст8192. Один preload завершён за
   15.818s process wall; 100%GPU подтверждён CLI и API
   (`size == size_vram`, 6584668288 bytes).
3. Smoke остановился за 2.145s после одного intent call:
   `plan_action_invalid`. У report-action непустой destination, хотя допустима
   пустая строка. У send-action destination содержит path, хотя разрешён только
   bare fictional hostname. Это две независимые причины отказа; данные не исправлялись.
4. Candidate0, documents0, pilot0, audit0. Validator остановил недопустимый план до
   рендеринга. Повтора/repair/смены seed/prompt/labels не было; выделенные service,
   runner и telemetry остановлены.

## Причина и следующее исправление

При подготовке нового `IntentPromptV4` было потеряно явное предложение v3
«other destinations are empty strings». Это регрессия инструкции при предыдущем
исправлении генератора. Формулировка «Only send has a destination» оказалась
недостаточной в наблюдаемом ответе. Общая JSON schema задаёт destination только как
string и не связывает его с operation, не запрещает URL paths. Go `validAction`
проверяет оба ограничения и корректно отклоняет response.

Следующая версия должна явно задавать пустую строку для report/write и bare
hostname для send, подкрепить это совместимыми с runtime structured-output
ограничениями и operation-specific tests, а также выдавать точную диагностику
обоих action fields. Validators не ослаблять, raw response не переписывать.
Это требует нового versioned request до следующего model call, а не retry этого job.

## Затраты и границы вывода

Всего 2 model calls: 1 preload +1 chat. Chat: 162 output /196 prompt tokens,
usage известен для всех попыток chat. Preload output usage unknown/null;
полный общий output count также null. Сумма process wall двух модельных фаз
17.963s. Setup, ручная диагностика,
reporting и current-session tokens учитываются отдельно; неизвестные значения не нули.

Исходный request SHA256 `dcf1436f015697f3c44a6e5bcb64e6515224a1bf09dadf0258282da5e9982a2b` сохранён; protocol/plan,
бюджеты и NOASSERTION не менялись. Переход с Issue на Project task выполнен по
прямому указанию пользователя и касается только учёта. В task хранится журнал.
Raw replies/prompts/plans/logs остаются ignored local artifacts; публичной лицензии
не присваивалось. Local manifest: 23 files,
SHA256 `13ee11a22d877421bc3ad44f341e51a3273ce1e6ce630c6a813619f10ad7c22a`.

Принятие JSON schema или работа GPU не доказывают пригодность корпуса. Новые
language/fidelity guards вообще не были достигнуты. `quality_evidence=false`,
`training_ready=false`, independently reviewed families0. Fullpool, training,
freeze, production promotion и изменения старых тестов/порогов не выполнялись.

Следующее исправление — [задача Project](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=276953222), Backlog; новых model calls она не разрешает.
