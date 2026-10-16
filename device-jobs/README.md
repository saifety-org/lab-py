# Задания для устройств

Статус работы и зависимости — в [GitHub Project](https://github.com/orgs/saifety-org/projects/1).
Инструкция, pins, команды и ожидаемые результаты — в этом репозитории.
Так локальный агент на ПК получает поручение из GitHub, без переноса текста чата.

| Задание | Устройство | Карточка | Инструкция |
| --- | --- | --- | --- |
| `proxy-refusal-v2` | Windows / RTX 5070 Ti | [Повторить LLM-прогон](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=269257271) | [runbook](proxy-refusal-v2.md), [pins](proxy-refusal-v2.json) |
| `context-corpus-v5-pilot` | Windows / RTX 5070 Ti | [Проверить сбор кандидатов](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=272703263) | [runbook локального агента](context-corpus-v5-pilot.md), [pins и caps](context-corpus-v5-pilot.json) |
| `context-corpus-v5-paired-v2` | Windows / RTX 5070 Ti | [Проверить исправленный генератор](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=273503121) | [runbook](context-corpus-v5-paired-v2.md), [pins и caps](context-corpus-v5-paired-v2.json) |

Первый `context-corpus-v5-pilot` завершён с [STOP/0 кандидатов](../docs/context-corpus-v5-pilot.md).
Его request не разрешает третью попытку. Новое задание `paired-v2` ждёт принятия
исправлений/main CI и статуса Todo; старые inputs/два результата не перезаписывать.

Таблица не объявляет готовность запуска: перед работой проверить живой статус
карточки и merge/CI. Созданное задание может ждать зависимостей в Backlog.

Новому заданию нужны собственные ID/карточка, устройство, зависимости, immutable
источники и модель, лимиты, команды, критерии остановки и публикации. Изменение
эксперимента — новый versioned request/PR; принятые результаты не перезаписываются.
Порядок работы: [docs/device-work.md](../docs/device-work.md).
