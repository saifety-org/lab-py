# Рабочая таблица Unicode

`datasets/unicode/confusables.txt` — исходные данные Unicode Security Mechanisms
(UTS #39), версия 18.0.0. Файл перенесён из
`internal/scan/normalize/gen/confusables.txt` приложения без изменения байтов.
Исходный коммит, пути и SHA256 сохранены в `migration.json`. Уведомление Unicode
и ссылка на условия использования сохранены в заголовке самого файла.

`cmd/gen-confusables` оставляет соответствия, целиком состоящие из ASCII-букв
или цифр, и создаёт Go-таблицу для пакета `normalize`. Это вспомогательный
инструмент разработки: сборка приложения использует уже готовую таблицу.

## Воспроизведение

Из корня lab:

```sh
GOWORK=off make confusables
cmp artifacts/confusables_table.go ../saifety/internal/scan/normalize/confusables_table.go
```

Укажите реальный путь к клону приложения во второй команде. Для сохранённого
источника результат совпадает с рабочей таблицей побайтово. Генератор сортирует
ключи и форматирует Go-код; `artifacts/` исключён из Git.

Другие пути задаются флагами:

```sh
go run ./cmd/gen-confusables -in datasets/unicode/confusables.txt -out artifacts/confusables_table.go
```

Обновление источника Unicode и замена таблицы приложения оформляются отдельными
PR с проверкой изменения соответствий и тестами нормализации. Генерация в lab
сама не меняет файлы или поведение установленного приложения.
