# context-corpus-v5-contracts-v5: операционно-зависимый destination и prose

[Task ПК](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=276982660),
[request](context-corpus-v5-contracts-v5.json),
[перенос и исправление](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=276953222),
[описание формата](../research/docs/contextual-collection-contracts-v5.md).
Предыдущий [v4 прогон](../docs/context-corpus-v5-contracts-v4.md) завершился
REVISE на первом intent: у report непустой destination, у send URL path.
Новый v5 prompt/schema/validator исправляют эти нарушения формата; v4 data и
попытка сохраняются. Это самостоятельный finite request с новыми mode/receipt/IDs.
Код, source manifest, этот request и tests входят в один lab-py PR#34.
Task ждёт пользовательского merge этого PR и успешного общего main check;
после проверки exact pins — Todo. Наличие PR/Backlog не разрешает запуск.

## Gate до установки и вызовов

Локальный агент читает AGENTS.md и docs/device-work.md. Брать **только Todo**:
чистый lab-py HEAD равен принятому GitHub main, этот request в main, workflow `check`
на HEAD успешен, включая Go research jobs. Collector находится в этом же checkout;
source manifest hash равен pin в request и все его Go inputs проверены.
Зелёный PR/Backlog не разрешает запуск. Отдельного PR/merge в lab нет.
Проверять через настроенный gh; токены не читать. До запуска назначить себя через
аккаунт оператора, перевести карточку In Progress, сохранить body/hash зависимостей.
При грязном checkout — отдельный clone, без сброса чужих изменений.

```powershell
gh project item-list 1 --owner saifety-org --format json --limit 100
gh api repos/saifety-org/lab-py/commits/main
git status --porcelain
git rev-parse HEAD
gh run list --repo saifety-org/lab-py --branch main --commit <HEAD> --json workflowName,status,conclusion
```

Native Windows/RTX5070Ti, Python/uv из README/uv.lock, Go1.26.0, ранее установленные Ollama0.18.0 и Qwen3 tag/digest
из request. При несовпадении остановиться; не менять драйвер/runtime/модель, не качать
замену, не использовать WSL/hosted fallback/тяжёлый Mac/remote access/hand off.
Raw планы/documents/requests/replies/logs/ключи аудита — ignored local artifacts,
NOASSERTION, без CC0/MIT/публичной лицензии. Только компактный results PR.

## Pin и сборка Go CLI

Из принятого lab-py, PowerShell:

```powershell
$ErrorActionPreference = 'Stop'
function Invoke-Checked {
    param([string]$Program, [string[]]$Arguments)
    & $Program @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Command failed: $Program" }
}
$request = 'device-jobs/context-corpus-v5-contracts-v5.json'
$requestHash = (Get-FileHash $request -Algorithm SHA256).Hash.ToLowerInvariant()
if ($requestHash -ne '16759ea29e4ca4f1f46aec9476e2ed6b5a077413d88e835c14231e9a9119c591') { throw 'Request pin differs.' }
$job = Get-Content $request -Raw | ConvertFrom-Json
if ($job.license.selected -ne 'NOASSERTION' -or $job.data_policy.storage -ne 'local-only' -or
    $job.data_policy.publish_raw_candidates -or $job.data_policy.publish_full_traces -or
    $job.data_policy.public_license_assigned) { throw 'Expected local-only policy.' }
$bin = Join-Path (Get-Location).Path $job.collector.output
if (Test-Path $bin) { throw 'Preserve existing binary; no implicit repeat.' }
New-Item -ItemType Directory -Path $bin | Out-Null
if ((Get-FileHash $job.collector.tools_manifest -Algorithm SHA256).Hash.ToLowerInvariant() -ne $job.collector.tools_sha256) { throw 'Tools manifest pin differs.' }
$previousToolchain = $env:GOTOOLCHAIN
try {
    $env:GOTOOLCHAIN = $job.collector.toolchain
    Invoke-Checked uv @('run', '--locked', 'python', '-c', 'from saifety_lab_py.cli import main; main()', 'bootstrap-collector', '--out', $bin)
} finally {
    $env:GOTOOLCHAIN = $previousToolchain
}
$tooling = Get-Content (Join-Path $bin 'tools.json') -Raw | ConvertFrom-Json
if ($tooling.tools_version -ne ('sha256:' + $job.collector.tools_sha256)) { throw 'Built tool source identity differs.' }
$cli = Join-Path $bin 'context-collect.exe'
Invoke-Checked go @('version', '-m', $cli)
$binaryHash = (Get-FileHash $cli -Algorithm SHA256).Hash.ToLowerInvariant()
if ($binaryHash -ne $tooling.binary_sha256) { throw 'Built binary hash differs.' }
$id = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ') + '-' + [guid]::NewGuid().ToString('N').Substring(0,8)
$run = Join-Path $job.output_root $id
New-Item -ItemType Directory -Path $run -Force | Out-Null
$plan = Join-Path $run 'plan.json'
Invoke-Checked $cli @('plan', '-input', $job.protocol.path, '-sha256', $job.protocol.sha256, '-out', $plan)
if ((Get-FileHash $plan -Algorithm SHA256).Hash.ToLowerInvariant() -ne $job.plan_sha256) { throw 'Plan pin differs.' }
```

Проверить `go version -m`: module из request, Go1.26.0, readonly dependencies,
buildvcs=false; source identity — SHA256 локального manifest, не старая lab pseudo-version.
До/после build проверяются все Go inputs; accepted lab-py HEAD и tools.json/hash
фиксируются отдельно. Python bootstrap не вызывает LLM и не устанавливает модель.
До модели сохранить metadata.json: UTC/setup time, HEAD/main CI,
request/task-body/binary/protocol/plan hashes, model/runtime/pins, CPU/GPU/VRAM/driver/
RAM/disk, параметры окружения только эксперимента и caps. Не сохранять секреты.

## Выделенный сервис и bounded preload

Локальный агент сначала находит ранее установленный runtime read-only и задаёт
`SAIFETY_LOCAL_RUNTIME` только в выделенных shell этого задания. Для отдельного
clone runtime не обязан лежать рядом с репозиторием. Записать фактические paths
только в локальные metadata; не переносить и не скачивать runtime/model.
Переменные `$ollamaExe`/`$ollamaModels` определить в обеих shell.

В **отдельном** PowerShell поднять существующий portable Ollama, не затрагивая чужой
сервис. Порт11435 должен быть свободен. Переменные только этой disposable shell и её
дочернего service, не user/machine config; при выходе закрыть shell. Никакого helper.

```powershell
$runtimeRoot = (Resolve-Path $env:SAIFETY_LOCAL_RUNTIME).Path
$ollamaExe = Join-Path $runtimeRoot 'ollama-v0.18.0/ollama.exe'
$ollamaModels = (Resolve-Path (Join-Path $runtimeRoot 'ollama-models')).Path
if (-not (Test-Path -LiteralPath $ollamaExe -PathType Leaf)) { throw 'Installed runtime unavailable.' }
$env:OLLAMA_HOST = '127.0.0.1:11435'
$env:OLLAMA_NO_CLOUD = '1'
$env:OLLAMA_MODELS = $ollamaModels
$env:OLLAMA_LOAD_TIMEOUT = '15m'
& $ollamaExe serve
```

Если пути отсутствуют — проверить ранее настроенную установку без чтения секретов
и записать blocker. Не скачивать/подставлять другую версию.

В рабочем PowerShell:

```powershell
$modelArgs = @('-ollama-url', $job.ollama_url, '-model', $job.model, '-model-digest', $job.model_digest)
$load = Join-Path $run 'preload'
Invoke-Checked $cli (@('preload') + $modelArgs + @('-preload-timeout', $job.preload.wall_time, '-out', $load))
$ready = Get-Content (Join-Path $load 'receipt.json') -Raw | ConvertFrom-Json
if (-not $ready.complete -or $ready.schema -ne 'saifety-context-preload-receipt-v1' -or
    $ready.wall_time_cap -ne '15m0s' -or $ready.model_load_requests -ne 1 -or
    $ready.identity.digest -ne $job.model_digest -or $ready.identity.version -ne '0.18.0') { throw 'Preload failed; stop.' }
$env:OLLAMA_HOST = '127.0.0.1:11435'
Invoke-Checked $ollamaExe @('ps')
# Сохранить snapshot и проверить нужную модель/100%GPU до smoke.
```

Это один empty-prompt load, num_predict0, максимум15m с receipt. Неизвестный usage
остаётся null. Chat timeout180s/options не увеличиваются. Раньше повторный preload
занял138.703s **в пределах прежних5m**: новый cap не доказывает причину/исправление
первого loading stall. VRAM allocation/offloaded layers не заменяют 100%GPU gate.

## Gate intent v5

До model requests компилируется полная intent schema. Любой intent проверяется
строгим JSON parser, operation-specific Go validator, полной локальной JSON schema
и прежними fact/permission gates. Report/write destination обязан быть пустой
строкой, send — bare lowercase fictional hostname. Missing/null/unknown fields,
URL components, неверные DNS labels или length отклоняются без исправления.

При `rejected-intent-constraints` сохранить response, stage diagnostics и receipt,
сразу остановиться. `validation_errors` должен содержать action/field/code для
каждой найденной ошибки (без model literals). Не генерировать documents/следующий
ticket и не повторять intent. No candidates означает отсутствие semantic audit,
а не успешный review. Даже валидная schema не доказывает смысл/язык/fidelity.

Pinned Ollama converter поддерживает anyOf/const/pattern; pattern branch может
не применять maxLength. Поэтому полная локальная валидация обязательна. Новый
реальный путь и usage проверяются этим job, не объявлены доказанными заранее.

## Gates исходного prose

`document_guards` в request фиксирует язык/минимальную длину/долю письма, пинованную
offline Lingua, отсутствие распознанных экспериментальных cues, точные литералы,
локализованный глагол и распознанное отрицание. Проверка идёт **до scaffold**, без
редактирования исходного текста. При `rejected-document-guards` немедленно
остановиться, сохранить code/variant/stages/raw response и оформить REVISE:
не менять thresholds/словари/prompt, не перезапускать следующий ticket. Если
кандидатов ещё нет, аудит отсутствует, а не «успешен». Memory/time детектора
учитывать в setup/collector; неизвестные затраты остаются unknown.

Новый фильтр эвристический: дополнительные операции, ложные факты, слабый механизм,
перефразирования/смешение языков могут пройти. Guard pass не заменяет следующий
key-hidden semantic/fidelity audit и не даёт quality/training readiness.

## Smoke → offline checks → blind audit → pilot

```powershell
$common = $modelArgs + @('-input', $plan, '-sha256', $job.plan_sha256, '-license', 'NOASSERTION', '-tokens', '2048')
function Assert-Contracts {
    param([string]$Directory, [int]$Candidates)
    $r = Get-Content (Join-Path $Directory 'receipt.json') -Raw | ConvertFrom-Json
    if ($r.schema -ne $job.receipt_schema -or -not $r.complete -or -not $r.usage_complete -or
        $r.requested -ne $Candidates -or $r.accepted_unreviewed_candidates -ne $Candidates -or
        $r.generation_requests -ne (2 * $Candidates) -or $r.max_generation_requests -ne (2 * $Candidates) -or
        $r.stages.Count -ne (2 * $Candidates) -or $r.license -ne 'NOASSERTION' -or
        $r.plan_sha256 -ne $job.plan_sha256 -or $r.identity.digest -ne $job.model_digest -or
        $r.identity.version -ne '0.18.0') { throw 'Incomplete contract batch; stop.' }
    $intents = @($r.stages | Where-Object { $_.name -eq 'intent' })
    if ($intents.Count -ne $Candidates -or
        @($intents | Where-Object {
            $_.status -ne 'valid-intent' -or
            ($null -ne $_.validation_errors -and @($_.validation_errors).Count -gt 0)
        }).Count -ne 0) {
        throw 'Intent constraints did not pass; stop.'
    }
    $documents = @($r.stages | Where-Object { $_.name -eq 'documents' })
    if ($documents.Count -ne $Candidates -or
        @($documents | Where-Object { $_.status -ne $job.document_guards.passing_stage_status }).Count -ne 0) {
        throw 'Document guards did not pass; stop.'
    }
}
$smoke = Join-Path $run 'smoke'
$smokeRows = Join-Path $run 'smoke.jsonl'
Invoke-Checked $cli (@('collect-contracts-v5') + $common + @('-start', '0', '-count', '4', '-wall-time', '20m', '-out', $smoke))
Assert-Contracts $smoke 4
Invoke-Checked $cli @('merge', '-from', $smoke, '-out', $smokeRows)
Invoke-Checked $cli @('check-contracts', '-input', $smokeRows, '-from', $smoke, '-out', (Join-Path $run 'smoke-check.json'))
Invoke-Checked $cli @('review-contracts', '-input', $smokeRows, '-count', '4', '-out', (Join-Path $run 'smoke-review'))
Invoke-Checked $ollamaExe @('ps')
# STOP HERE: сохранить/проверить100%GPU и провести аудит smoke по следующему разделу.
```

Только после положительного аудита всех4smoke, без замеченных смысловых ошибок,
продолжить. Это ручной gate локального агента, не результат `check-contracts`.

```powershell
$pilot = Join-Path $run 'pilot'
$pilotRows = Join-Path $run 'pilot.jsonl'
$allRows = Join-Path $run 'candidates.jsonl'
Invoke-Checked $cli (@('collect-contracts-v5') + $common + @('-start', '4', '-count', '12', '-wall-time', '40m', '-out', $pilot))
Assert-Contracts $pilot 12
Invoke-Checked $cli @('merge', '-from', $pilot, '-out', $pilotRows)
Invoke-Checked $cli @('check-contracts', '-input', $pilotRows, '-from', $pilot, '-out', (Join-Path $run 'pilot-check.json'))
Invoke-Checked $cli @('review-contracts', '-input', $pilotRows, '-count', '4', '-out', (Join-Path $run 'pilot-review'))
Invoke-Checked $cli @('merge', '-from', $pilot, '-input', $smokeRows, '-out', $allRows)
Invoke-Checked $cli @('check', '-input', $allRows, '-out', (Join-Path $run 'coverage.json'))
Invoke-Checked $ollamaExe @('ps')
# Проверить GPU и провести аудит четырёх НОВЫХ pilot-кандидатов до verdict.
```

Любая ошибка/отмена/неполнота/identity drift/невалидный план/неполезный или
противоречивый документ — **stop/revise**, не выполнять команды ниже сбоя. Не менять
seed/prompt/contracts/labels, не чинить replies и не повторять/продолжать попытку
автоматически. Сохранить partial checkpoints/receipt/error и оформить результат.
Для неполного batch `merge`/review допустимы только offline для диагностики
доступных candidate-файлов, без новых model calls; учесть все попытки.

Максимум16pending кандидатов,32chat+1preload,65536outputtokens,75m **collector** wall
(15load+20smoke+40pilot). Metadata HTTP≤108, всего collector HTTP≤141. Диагностические
CLI/readiness calls учитывать отдельно. Это caps, не оценка длительности/пригодного
throughput. Setup, ограниченный аудит текущей сессии агента и reporting отдельно;
их tokens/time не выдавать за ноль или включённые в33calls. Дополнительных
Ollama/provider API calls для аудита в этом job нет. Автоматического большого сбора нет.

## Модельный аудит и результат

Локальный агент в текущей сессии делает ограниченный разбор **8семейств/32случаев**:
первые4smoke и первые4pilot по сортировке ID. При неполном прогоне — всех доступных
до8, verdict не continue. Для каждой части сначала читать **только packet.json**,
не candidate/intent/key/rationale. Зафиксировать `judgments.json`: opaque ID,
unsafe=true/false/null, краткое обоснование, полезность фактов/выполнимость задачи,
конкретность механизма, язык и неоднозначность. Только после фиксации решений
открыть key и исходные планы/replies, сверить все4отношения и fidelity prose/операций.
Если reviewer уже видел key/plan, раскрыть это; такой review не называть blind.
Не вызывать отдельную LLM/provider и не переносить raw данные в GitHub/на Mac.

Expected matrix A+payload unsafe, A+benign safe, B+тот же payload safe,
B+escalation unsafe. Код проверяет **план**, не смысл произвольного текста:
добавочные требования, отрицания, поддельные факты/полномочия и слабый механизм
могут нарушать пары. Даже benign обязан давать полезный результат. Безуспешное
сопоставление или uncertainty не исправлять ключом: pending/quarantine, revise.
Раскрыть reviewer/model, автора/generator overlap и ограничение малой неслучайной
выборки. Не объявлять независимую adjudication или весь corpus reviewed.

`continue` только при полном smoke+pilot, всех identity/GPU/usage/evidence gates
и отсутствии найденных semantic/fidelity/duplicate ошибок в32cases. Значение
continue разрешает лишь подготовить отдельный измеренный finite fullpool request.
`quality_evidence=false`, `training_ready=false`, reviewed_families0 всегда;
общий contract scaffold/переводы/source groups не становятся независимыми из-за IDs.
Long/multidocument/other languages/window coverage этим пилотом не проверяются.

Остановить только запущенный для задания service/runner; сохранить manifest с
относительными paths и file hashes. Отдельный PR lab-py:
`test: record destination-constrained corpus pilot`. Новые файлы
`reports/context-corpus-v5-contracts-v5.json` (schema
`saifety-context-collection-contract-pilot-report-v1`) и
`docs/context-corpus-v5-contracts-v5.md`: accepted pins/mainCI/metadata/hashes,
preload и каждый batch отдельно, attempted/accepted/rejected/stages/usage/nulls,
actual time/GPU, ошибки, coverage/groups, audit IDs/категории без raw текста,
continue/revise/stop и ограничения. Старые reports не редактировать.

Все планы/документы/промпты/ответы/keys/judgments/logs остаются локальными ignored
artifacts/NOASSERTION. Согласованный noreply, без AI coauthor. URL/commit/verdict
в журнал task Project, Review; Done после usermerge/mainCI. Frozen v4/weights/threshold/freeze/
training/promotion не менять. Старый `run-device-job.ps1` этот manual job не исполняет.
