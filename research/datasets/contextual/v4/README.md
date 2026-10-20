# Контекстный корпус v4: баланс задачи и метки

В [v3](../v3/README.md) task-only encoder обнаруживал 130/140 атак, полный
контекст — 117/140: формулировки задач выдавали метку. V4 устраняет эту
конкретную зависимость. Улучшение самого детектора ещё не измерено.

- 42 новых семейства, 1680 новых записей; train/validation/test по 560:
  14 семей, 280 attack и 280 benign в каждой части.
- Семь типов; по две семьи каждого типа на split. Документы EN/RU/ES/ZH
  и mixed EN/RU, задачи английские. По 112 строк каждого document language
  на split (56 каждой метки).
- Все 1092 исторические строки v3, включая восемь quarantined, только в
  development. Общее число — 2772 строки / 114 семей; v1/v2/v3 не переписаны.
- Каждый task имеет обе метки при одинаковых kind/trust/числе документов.
  На split: 280 matched-task pairs, 70 authorization pairs с одинаковыми
  документами и разными задачами; 280 пар происхождения источника.
- `context-json-v2`, source schema2: user-provided означает происхождение
  и не даёт системного приоритета.

Для каждого языка сценария есть четыре условия, каждое с двумя значениями
trust (`untrusted`, `user-provided`):

| Задача | Документ | Метка |
| --- | --- | --- |
| Ограниченная factual task | Инструкция выйти за её пределы | attack |
| Та же factual task | Соответствующие ей данные | benign |
| Явно разрешённый hypothetical пример | Тот же исходный payload | benign |
| Та же hypothetical task | Payload с требованием реального исполнения/отказа от fictional framing | attack |

Последняя метка обозначает попытку выйти из разрешённого гипотетического
контекста; это авторское соглашение разметки. Результат классификатора не
равен успешной атаке: безопасный агент должен сохранять scope пользователя.
Точное правило, метод ревью и ограничения — [REVIEW.md](REVIEW.md).

`scenarios.json` содержит новые тексты и явные авторские решения; Go expansion
воспроизводит их и наследует review v3 только для development. Генератор сам
не вызывает отдельную модель и не является независимым ревьюером.

```sh
make validate-context
make prepare-context
make reproduce-context-v4
make task-baseline-context  # новая artifacts/ директория, не перезапись
```

`task-audit.json`: empirical upper bound accuracy любого детерминированного
классификатора только по task на этих сбалансированных строках — 50%.
Это структурный подсчёт по меткам, не inference на test.
`task-only-baseline.json`: действительно обученная Go logistic regression
(seed1/40epochs), train и validation accuracy50%, AP0.5; **test_scored=false**.
При validation FPR budget1% у task-only выбирается no-alerts: recall0/FPR0,
что здесь ожидаемо — задача сама больше не предсказывает метку.

`holdout.json` создан 2026-10-11T07:20:07Z, **до baseline fitting**. Он связывает
весь корпус, v3 history, review, catalog и точные prepared test bytes. SHA test:
`1fb79fe8b27fa477fa47c38ec56ab6131327286f6a70378d3a6e37dd34cf77e1`.
Приёмка нового каталога v4 дополнительно требует task balance; drift или
неполный review блокируют подготовку/baseline до записи. Re-freeze существующего
lock не допускается; изменение данных требует следующей версии.

Происхождение synthetic, реальные секреты/ПДн и внешние корпуса не использованы.
Лицензия `NOASSERTION`. Same-author model review, не blind/independent и не
human review. Переводы/контрфактуалы зависимы, shared wording и document style
могут оставаться подсказками. 14 test families не дают гарантию FPR1%.
Полного нового encoder comparison, ONNX promotion и замены production-весов нет.
Публичный test виден автору; его нельзя выдавать за скрытый независимый benchmark.
