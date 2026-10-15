# context-corpus-v5-pilot: проверка локального сбора данных

[Карточка пилота](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=272703263),
[request](context-corpus-v5-pilot.json),
[родительский корпус](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=272440792).
Это ручное задание локальному агенту по AGENTS.md/docs/device-work.md.
`scripts/run-device-job.ps1` обслуживает другой эксперимент и этот request не запускает.

Цель — измерить скорость, ошибки и пригодность кандидатов до большого сбора.
Результат не оценивает защиту агента или модель анализатора. Все кандидаты pending;
64 tickets не означают 64 независимых или проверенных семейства. Общий генератор,
шесть жанров/рецептов и модель должны быть явно указаны в отчёте.

## Готовность: проверить до установки и генерации

1. Читать живую карточку через настроенный `gh`, брать только Todo; проверить Device,
   отсутствие другого исполнителя и перевести в In Progress. Сохранить исходный body
   и его hash. Не использовать контекст другой сессии как источник поручения.
2. Проверить принятие lab PR #13: `merged=true`, `head.sha` точно равен collector.commit
   из request, main workflow `ci` успешен на `merge_commit_sha`. Сам факт зелёного PR
   не означает принятия. Если pin изменился — нужен новый request через PR.
3. Использовать чистый принятый main lab-py: HEAD совпадает с текущим GitHub main,
   workflow `check` успешен на HEAD, этот request/runbook есть в main. При локальных
   изменениях использовать отдельный clone, ничего не сбрасывать. Записать HEAD и hash
   request. После обновления main повторно прочитать инструкции и карточку.
4. Проверить согласованность карточки и request: данные только локальные,
   `license.selected=NOASSERTION`, `data_policy.storage=local-only`, публичная
   лицензия не назначается. Пользователь отказался от публикации под CC0;
   MIT вместо неё тоже не выбирается автоматически. `NOASSERTION` — маркер отсутствия
   декларации лицензии, а не разрешение распространения. При расхождении остановиться.
5. Native Windows / RTX 5070 Ti. Проверить Go, gh, GPU/driver, свободный диск;
   записать версии. Модель и Ollama уже установлены: при отсутствии/несовпадении
   остановиться и записать блокер. Не скачивать новую модель, не подменять digest,
   не использовать hosted fallback, CPU/Mac-прогон или глобальные настройки клиента.

Примеры read-only проверок (значения сравнивает локальный агент):

```powershell
Get-Content device-jobs/context-corpus-v5-pilot.json -Raw
gh project item-list 1 --owner saifety-org --format json --limit 100
gh api repos/saifety-org/lab/pulls/13
gh api repos/saifety-org/lab-py/commits/main
# Подставить проверенные merge_commit_sha и HEAD, не SHA ветки PR.
gh run list --repo saifety-org/lab --branch main --commit <merge_commit_sha> --json status,conclusion,workflowName
gh run list --repo saifety-org/lab-py --branch main --commit <HEAD> --json status,conclusion,workflowName
git status --porcelain
git rev-parse HEAD
```

## Установка CLI и вычисление плана

После готовности выполнять в PowerShell из корня lab-py. Функция ниже останавливает
последовательность при nonzero exit code внешней программы. Команды установки
скачивают только Go toolchain/module; не обращаются к модели. Setup time отдельно
от generation wall time. Старые bin/artifacts/runtime не удалять и не перезаписывать.

```powershell
$ErrorActionPreference = 'Stop'
function Invoke-Checked {
    param([string]$Program, [string[]]$Arguments)
    & $Program @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Command failed: $Program" }
}
$job = Get-Content device-jobs/context-corpus-v5-pilot.json -Raw | ConvertFrom-Json
$bin = Join-Path (Get-Location).Path $job.collector.output
if (Test-Path $bin) { throw 'Preserve existing binary directory; inspect earlier attempts.' }
New-Item -ItemType Directory -Path $bin -Force | Out-Null
$oldBin = $env:GOBIN
$oldToolchain = $env:GOTOOLCHAIN
try {
    $env:GOBIN = $bin
    $env:GOTOOLCHAIN = $job.collector.toolchain
    Invoke-Checked go @('install', "$($job.collector.module)@$($job.collector.version)")
} finally {
    $env:GOBIN = $oldBin
    $env:GOTOOLCHAIN = $oldToolchain
}
$cli = Join-Path $bin 'context-collect.exe'
$binaryHash = (Get-FileHash $cli -Algorithm SHA256).Hash.ToLowerInvariant()
Invoke-Checked go @('version', '-m', $cli)
$id = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ') + '-' + [guid]::NewGuid().ToString('N').Substring(0,8)
$run = Join-Path $job.output_root $id
New-Item -ItemType Directory -Path $run -Force | Out-Null
$plan = Join-Path $run 'plan.json'
Invoke-Checked $cli @('plan', '-input', $job.protocol.path, '-sha256', $job.protocol.sha256, '-out', $plan)
$planHash = (Get-FileHash $plan -Algorithm SHA256).Hash.ToLowerInvariant()
if ($planHash -ne $job.plan_sha256) { throw 'Plan hash mismatch; do not generate.' }
```

Проверить `go version -m`: module/version/revision должны совпадать request;
`vcs.modified` не должен быть true. Protocol byte pin сохраняется `.gitattributes`.
Сохранить до генерации metadata.json: UTC/setup time, lab-py HEAD, request SHA256,
task body SHA256 и выбранную лицензию, collector version/commit/binary SHA256,
protocol/plan hashes, Windows/Go/driver/GPU/RAM/disk, модель/digest/Ollama и caps.
Абсолютные пути, конфиги и личные файлы в публикуемый отчёт не включать.

## Локальная Ollama и smoke

Повторно использовать portable Ollama 0.18.0 и модель с предыдущих прогонов.
Поднять только отдельный сервис эксперимента, без LAN/cloud. В отдельном PowerShell:

```powershell
$env:OLLAMA_HOST = '127.0.0.1:11435'
$env:OLLAMA_NO_CLOUD = '1'
$env:OLLAMA_MODELS = (Resolve-Path '..\.runtime\ollama-models').Path
& '..\.runtime\ollama-v0.18.0\ollama.exe' serve
```

При других локальных путях не угадывать и не читать чужие конфиги: найти ранее
настроенную установку с разрешённым оператором доступом и записать отклонение.
Если сервис уже работает, проверить local-only среду и identity, не запускать второй.
Collector сам проверяет установленный GGUF tag/digest и Ollama до/после генерации;
не скачивает модель и не следует HTTP redirect/proxy/remote model.

В рабочем PowerShell использовать только локальный режим из принятого request.
Raw кандидаты и полные traces сохраняются в ignored artifacts, не в публичном PR.

```powershell
$license = $job.license.selected
if ($license -ne 'NOASSERTION' -or $job.data_policy.storage -ne 'local-only' -or
    $job.data_policy.publish_raw_candidates -or $job.data_policy.publish_full_traces -or
    $job.data_policy.public_license_assigned) { throw 'Expected local-only unlicensed collection.' }
$common = @('-input', $plan, '-sha256', $job.plan_sha256,
    '-ollama-url', $job.ollama_url, '-model', $job.model,
    '-model-digest', $job.model_digest, '-license', $license, '-tokens', '2048')
$smoke = Join-Path $run 'smoke'
Invoke-Checked $cli (@('collect') + $common + @('-start', '0', '-count', '4', '-wall-time', '10m', '-out', $smoke))
$receipt = Get-Content (Join-Path $smoke 'receipt.json') -Raw | ConvertFrom-Json
if (-not $receipt.complete -or -not $receipt.usage_complete -or
    $receipt.generation_requests -ne 4 -or $receipt.accepted_unreviewed_candidates -ne 4 -or
    $receipt.plan_sha256 -ne $job.plan_sha256 -or $receipt.license -ne $license -or
    $receipt.identity.digest -ne $job.model_digest -or $receipt.identity.version -ne '0.18.0') {
    throw 'Incomplete smoke; preserve checkpoints and stop.'
}
$env:OLLAMA_HOST = '127.0.0.1:11435'
Invoke-Checked '..\.runtime\ollama-v0.18.0\ollama.exe' @('ps')
```

После smoke сохранить `ollama ps`/GPU snapshot: модель должна размещаться 100% на GPU.
Проверить четыре кандидата на очевидные нарушения схемы, бессмыслицу или копирование.
При ошибке, truncation, identity drift, неясном usage, CPU offload или исчерпании
бюджета остановиться. Не исправлять ответы молча и не генерировать замену в этом задании.
Сбой HTTP/невалидный внешний JSON может оставить receipt без полного raw reply;
не заявлять, что все ответы сохранены или фактический token count полностью известен.

## Остальные 60 запросов и отчёт

Только после полного smoke и проверки GPU:

```powershell
$pilot = Join-Path $run 'pilot'
Invoke-Checked $cli (@('collect') + $common + @('-start', '4', '-count', '60', '-wall-time', '110m', '-out', $pilot))
$receipt = Get-Content (Join-Path $pilot 'receipt.json') -Raw | ConvertFrom-Json
if (-not $receipt.complete -or -not $receipt.usage_complete -or
    $receipt.generation_requests -ne 60 -or $receipt.accepted_unreviewed_candidates -ne 60 -or
    $receipt.plan_sha256 -ne $job.plan_sha256 -or $receipt.license -ne $license -or
    $receipt.identity.digest -ne $job.model_digest -or $receipt.identity.version -ne '0.18.0') {
    throw 'Incomplete pilot; preserve checkpoints and stop.'
}
$smokeRows = Join-Path $run 'smoke.jsonl'
$allRows = Join-Path $run 'candidates.jsonl'
Invoke-Checked $cli @('merge', '-from', $smoke, '-out', $smokeRows)
Invoke-Checked $cli @('merge', '-from', $pilot, '-input', $smokeRows, '-out', $allRows)
Invoke-Checked $cli @('check', '-input', $allRows, '-out', (Join-Path $run 'coverage.json'))
```

Суммарно до 64 генераций / 131072 output tokens / 120 минут generation wall.
Это caps, не оценка длительности. До/после сборки, metadata и ревью учитываются отдельно.
Никаких скрытых retries, автоматического resume или большого цикла. При сбое сохранить
частичный отчёт/receipt/checkpoints без повторной генерации; новый запуск требует
отдельного задания/обновлённого request. Остановить только сервис эксперимента,
если он запускался для задания. Старые artifacts/runtime/model сохранить.

Локальный агент делает модельное ревью минимум 8 кандидатов (при неполном прогоне —
всех доступных, максимум 8), распределённых по доступным языкам и типам. Записать IDs,
наблюдения по четырём гипотезам авторизации, полезности benign, копиям/неоднозначности
и языковым ограничениям. Не менять pending на reviewed: это аудит пригодности
генератора, полный независимый model-review/leakage/freeze — последующий этап.

Результат через отдельный PR lab-py:

- `reports/context-corpus-v5-pilot.json`: schema `saifety-context-collection-pilot-report-v1`,
  pins/hashes, относительный путь локальных artifacts, hardware/runtime, budgets,
  attempted/accepted, known source-connected groups, reported tokens/usage completeness,
  setup/generation/review durations, errors, sample audit и continue/revise/stop.
  `training_ready=false`, `quality_evidence=false`.
- `docs/context-corpus-v5-pilot.md`: что получилось, причины отбраковки, доступные
  типы/языки, ограничения общего генератора и прогноз стоимости большого сбора по
  фактической скорости. Не выдавать прогноз за обязательство или разрешение запуска.
- Hash manifest всех сохранённых outputs; полные traces/candidates остаются в ignored
  artifacts. В этот PR не включать тысячи строк raw data или новые веса.

Название PR: `test: record contextual corpus collection pilot`. Использовать
согласованный noreply, без AI coauthor. PR/commit/outcome/ошибки записать в карточку,
перевести в Review; Done после пользовательского merge и main CI. При результате
continue координатор отдельно готовит request большого сбора с измеренным бюджетом.
Родительский корпус этим пилотом не закрывается; v4/frozen test/пороги не меняются.
