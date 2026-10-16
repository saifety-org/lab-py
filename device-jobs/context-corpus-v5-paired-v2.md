# context-corpus-v5-paired-v2: повторный пилот двухэтапного генератора

[Карточка](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=273503121),
[request](context-corpus-v5-paired-v2.json),
[задача исправления](https://github.com/orgs/saifety-org/projects/1?pane=issue&itemId=273503006).
Это отдельный manual runbook локального агента. Старый `context-corpus-v5-pilot`
завершился STOP; его request/два результата сохраняются. Старый PowerShell runner
`scripts/run-device-job.ps1` обслуживает другой эксперимент и это задание не запускает.

## Готовность

Брать только Todo через AGENTS.md/docs/device-work.md и `gh project item-list`.
До установки/модельных вызовов проверить exact head и merge collector PR из request,
успешный main workflow `ci` на его merge commit, чистый HEAD lab-py, совпадающий
с GitHub main, и успешный workflow `check` на HEAD. Этот job должен быть принят в main.
Backlog/зелёный PR без merge не разрешает запуск. Если изменения пользователя мешают
обновлению, использовать отдельный clone; ничего не сбрасывать. Назначить себя через
аккаунт оператора, перевести карточку In Progress и сохранить исходный task body/hash.

Native Windows/RTX5070Ti; Go1.26.0, существующие Ollama0.18.0 и Qwen3 tag/digest
из request. При отсутствии/несовпадении остановиться, не скачивать другую модель,
не менять драйвер/runtime по гипотезе о причине старого timeout. Только loopback11435,
OLLAMA_NO_CLOUD=1, без hosted fallback/WSL/Mac inference/hand off.

Raw requests/responses/candidates/full traces — только локальные ignored artifacts,
`NOASSERTION`; это отсутствие публичной лицензии. CC0/MIT не назначать. Компактный
results PR не содержит исходные документы, промпты/ответы или полный sample content.

Примеры read-only проверок (локальный агент сверяет значения):

```powershell
Get-Content device-jobs/context-corpus-v5-paired-v2.json -Raw
gh project item-list 1 --owner saifety-org --format json --limit 100
# PR number и проверенный merge commit берутся из request/GitHub.
gh api repos/saifety-org/lab/pulls/<collector_pr>
gh run list --repo saifety-org/lab --branch main --commit <merge_commit> --json workflowName,status,conclusion
gh api repos/saifety-org/lab-py/commits/main
git status --porcelain
git rev-parse HEAD
gh run list --repo saifety-org/lab-py --branch main --commit <HEAD> --json workflowName,status,conclusion
```

## Установка и локальный сервис

Из корня принятого lab-py в PowerShell:

```powershell
$ErrorActionPreference = 'Stop'
function Invoke-Checked {
    param([string]$Program, [string[]]$Arguments)
    & $Program @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Command failed: $Program" }
}
$job = Get-Content device-jobs/context-corpus-v5-paired-v2.json -Raw | ConvertFrom-Json
if ($job.license.selected -ne 'NOASSERTION' -or $job.data_policy.storage -ne 'local-only' -or
    $job.data_policy.publish_raw_candidates -or $job.data_policy.publish_full_traces -or
    $job.data_policy.public_license_assigned) { throw 'Expected local-only policy.' }
$bin = Join-Path (Get-Location).Path $job.collector.output
if (Test-Path $bin) { throw 'Preserve existing attempt/binary; no implicit repeat.' }
New-Item -ItemType Directory -Path $bin -Force | Out-Null
$previousBin = $env:GOBIN
$previousToolchain = $env:GOTOOLCHAIN
try {
    $env:GOBIN = $bin
    $env:GOTOOLCHAIN = 'go1.26.0'
    Invoke-Checked go @('install', "$($job.collector.module)@$($job.collector.version)")
} finally {
    $env:GOBIN = $previousBin
    $env:GOTOOLCHAIN = $previousToolchain
}
$cli = Join-Path $bin 'context-collect.exe'
Invoke-Checked go @('version', '-m', $cli)
$binaryHash = (Get-FileHash $cli -Algorithm SHA256).Hash.ToLowerInvariant()
$id = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ') + '-' + [guid]::NewGuid().ToString('N').Substring(0,8)
$run = Join-Path $job.output_root $id
New-Item -ItemType Directory -Path $run -Force | Out-Null
$plan = Join-Path $run 'plan.json'
Invoke-Checked $cli @('plan', '-input', $job.protocol.path, '-sha256', $job.protocol.sha256, '-out', $plan)
if ((Get-FileHash $plan -Algorithm SHA256).Hash.ToLowerInvariant() -ne $job.plan_sha256) { throw 'Plan pin differs.' }
```

В `go version -m` проверить module/version/toolchain; отсутствие отдельного vcs.revision
допустимо для versioned module install. Commit привязан pseudo-version и точному head
принятого PR. `vcs.modified=true` недопустим. До модели сохранить metadata.json:
UTC/setup time, HEAD/request/task-body/binary/protocol/plan hashes, model/runtime/pins,
Windows/CPU/GPU/driver/RAM/disk и budgets. Setup отдельно от model wall time.
Существующие bin/runtime/артефакты прошлого пилота не удалять.

Поднять отдельную существующую portable Ollama в другом PowerShell, если она ещё
не работает с нужной local-only средой. Не затрагивать чужую установку/сервис:

```powershell
$env:OLLAMA_HOST = '127.0.0.1:11435'
$env:OLLAMA_NO_CLOUD = '1'
$env:OLLAMA_MODELS = (Resolve-Path '..\.runtime\ollama-models').Path
& '..\.runtime\ollama-v0.18.0\ollama.exe' serve
```

Если прежние пути отсутствуют, найти ранее настроенную установку без чтения секретов,
записать отклонение или блокер. Не подставлять другую модель/версию.

## Preload, smoke и pilot

1. Один `preload`: пустой `/api/generate`, num_predict0, максимум5мин whole load.
   Это **один дополнительный model call** с отдельным request/response/receipt.
   Отсутствующий eval_count остаётся null; не объявлять неизвестный usage нулём.
2. После полного preload проверить `ollama ps`/snapshot: нужная модель,100%GPU.
   Выделенная VRAM/37offloaded layers сами по себе этот gate не заменяют.
3. Smoke4 кандидата = до8 `/api/chat`, затем pilot12 = до24. На каждого кандидата
   base→escalation,2048outputtokens/этап, без retry/починки/смены seed/prompt.
   Любая ошибка/неполнота/identity drift/невалидный counterfactual — stop.

В рабочем PowerShell:

```powershell
$modelArgs = @('-ollama-url', $job.ollama_url, '-model', $job.model, '-model-digest', $job.model_digest)
$load = Join-Path $run 'preload'
Invoke-Checked $cli (@('preload') + $modelArgs + @('-out', $load))
$ready = Get-Content (Join-Path $load 'receipt.json') -Raw | ConvertFrom-Json
if (-not $ready.complete -or $ready.schema -ne 'saifety-context-preload-receipt-v1' -or
    $ready.model_load_requests -ne 1 -or $ready.identity.digest -ne $job.model_digest -or
    $ready.identity.version -ne '0.18.0') { throw 'Preload failed; preserve outputs and stop.' }
$env:OLLAMA_HOST = '127.0.0.1:11435'
Invoke-Checked '..\.runtime\ollama-v0.18.0\ollama.exe' @('ps')
# Сохранить/проверить100%GPU до следующей команды.
$common = $modelArgs + @('-input', $plan, '-sha256', $job.plan_sha256, '-license', 'NOASSERTION', '-tokens', '2048')
function Assert-Paired {
    param([string]$Directory, [int]$Candidates)
    $r = Get-Content (Join-Path $Directory 'receipt.json') -Raw | ConvertFrom-Json
    if ($r.schema -ne 'saifety-context-collection-receipt-v2' -or -not $r.complete -or
        -not $r.usage_complete -or $r.requested -ne $Candidates -or
        $r.accepted_unreviewed_candidates -ne $Candidates -or
        $r.generation_requests -ne (2 * $Candidates) -or $r.max_generation_requests -ne (2 * $Candidates) -or
        $r.stages.Count -ne (2 * $Candidates) -or $r.license -ne 'NOASSERTION' -or
        $r.plan_sha256 -ne $job.plan_sha256 -or $r.identity.digest -ne $job.model_digest -or
        $r.identity.version -ne '0.18.0') { throw 'Paired batch incomplete; stop.' }
}
$smoke = Join-Path $run 'smoke'
Invoke-Checked $cli (@('collect-paired') + $common + @('-start', '0', '-count', '4', '-wall-time', '20m', '-out', $smoke))
Assert-Paired $smoke 4
Invoke-Checked '..\.runtime\ollama-v0.18.0\ollama.exe' @('ps')
# Сохранить/проверить100%GPU и четыре кандидата на очевидную бессмыслицу/копирование.
$pilot = Join-Path $run 'pilot'
Invoke-Checked $cli (@('collect-paired') + $common + @('-start', '4', '-count', '12', '-wall-time', '40m', '-out', $pilot))
Assert-Paired $pilot 12
$smokeRows = Join-Path $run 'smoke.jsonl'
$allRows = Join-Path $run 'candidates.jsonl'
Invoke-Checked $cli @('merge', '-from', $smoke, '-out', $smokeRows)
Invoke-Checked $cli @('merge', '-from', $pilot, '-input', $smokeRows, '-out', $allRows)
Invoke-Checked $cli @('check', '-input', $allRows, '-out', (Join-Path $run 'coverage.json'))
```

Прерывание любой команды не разрешает продолжать ниже, повторять или чинить response.
Сохранить частичный receipt/diagnostics/error и оформить stop-результат. Остановить
только сервис эксперимента, если он запускался для задания. При HTTP/outer JSON сбое
raw response/полный token accounting могут отсутствовать; не утверждать обратное.

Всего максимум16 pending кандидатов,32 generation calls +1preload,65536 outputtokens,
65мин model wall (5load+20smoke+40pilot). Input context8192/вызов. GoCLI делает не более
108metadata HTTP calls tags/show/version при полном успехе; они не генерации.
Фактический расход фиксировать отдельно, лимиты не являются прогнозом времени.

## Ревью и публикация результата

Модельный аудит минимум8 принятых кандидатов, распределённых по доступным языкам/типам
(при неполном прогоне — всех доступных до8). Проверить все4гипотезы разрешённости,
сохранение base, новое нарушение permitted scope в escalation, полезность benign,
языковую корректность/неоднозначность/очевидные копии. Сам автор/маленькая выборка
не доказывают независимость или качество будущей модели; все записи остаются pending.

`continue` допустим только при полном smoke/pilot, identity/GPU/usage gates и отсутствии
замеченных семантических ошибок/копий в sample audit. При недостаточных данных или
неясных контрактах — revise/stop. Измерять throughput **структурно принятых pending**
кандидатов, а не reviewed independent families. Большой сбор этим не разрешается.

Новый PR lab-py: `test: record paired contextual corpus pilot`, согласованный noreply,
без AI coauthor. В `reports/context-corpus-v5-paired-v2.json` (schema
`saifety-context-collection-paired-pilot-report-v1`) и `docs/context-corpus-v5-paired-v2.md`:
pins/hashes/metadata, load и оба batches отдельно, attempted/accepted/rejected/stages,
reported usage/nulls, actual time, GPU, ошибки, агрегированный coverage/known groups,
audit IDs и краткие категориальные замечания без raw текста, continue/revise/stop.
Сохранить относительный каталог локальных artifacts и file hash manifest. Не включать
полные logs/prompts/responses/candidates/weights или личные абсолютные пути.

Записать PR/commit/outcome в карточку, Review; Done после usermerge/mainCI.
После принятого continue координатор отдельно готовит measured finite fullpool request.
Никакого automatic training/freeze/promotion/изменения v4/threshold по этому пилоту.
