# Native PowerShell control-flow regression. Every external command is mocked;
# fixtures marked llm_run are synthetic and never published as model evidence.
$ErrorActionPreference = 'Stop'
$global:DeviceTestCalls = [System.Collections.Generic.List[string]]::new()
$global:DeviceTestMode = 'unmerged'
$global:DeviceTestOutput = $null
function global:git {
    $global:LASTEXITCODE = 0
    if ($args[0] -eq 'rev-parse') { return ('a' * 40) }
}
function global:gh {
    $global:LASTEXITCODE = 0
    if ($args[0] -eq 'run') {
        return '[{"workflowName":"ci","status":"completed","conclusion":"success"},{"workflowName":"check","status":"completed","conclusion":"success"}]'
    }
    $path = $args[1]
    if ($path -match '/commits/main$') { return '{"sha":"main"}' }
    if ($path -match '/compare/') { return '{"status":"identical"}' }
    if ($path -match '/pulls/10$') {
        $lock = Get-Content 'agent-harness-refusal-v2.sources.lock.json' -Raw | ConvertFrom-Json
        return @{ merged = ($global:DeviceTestMode -ne 'unmerged'); head = @{sha = $lock.core.commit}; merge_commit_sha = 'merge' } | ConvertTo-Json
    }
    throw "Unexpected GitHub request: $path"
}
function global:nvidia-smi {
    $global:LASTEXITCODE = 0
    return 'NVIDIA GeForce RTX 5070 Ti, fixture, 16303 MiB'
}
function global:ollama {
    $global:LASTEXITCODE = 0
    return 'qwen3:8b-q4_K_M fixture 100% GPU'
}
function global:uv {
    $global:LASTEXITCODE = 0
    $global:DeviceTestCalls.Add(($args -join ' '))
    if ($args -contains 'run-agent-harness') {
        $values = @($args)
        if (-not ($values -contains '--proxy-decisions') -or -not ($values -contains '--lock')) {
            throw 'Harness arguments lost by PowerShell binding.'
        }
        $preset = $values[[array]::IndexOf($values, '--preset') + 1]
        $out = $values[[array]::IndexOf($values, '--out') + 1]
        $global:DeviceTestOutput = Split-Path $out -Parent
        New-Item -ItemType Directory -Path $out | Out-Null
        $pairs = if ($preset -eq 'smoke') { 4 } else { 48 }
        $errors = if ($global:DeviceTestMode -eq 'bad-smoke') { 1 } else { 0 }
        $job = Get-Content 'device-jobs/proxy-refusal-v2.json' -Raw | ConvertFrom-Json
        @{
            schema = 'agent-harness-report-v2'; budget = @{episodes = ($pairs * 2)}
            agent = @{model = $job.model; digest = $job.model_digest; version = '0.18.0'}
            identity_stable = $true; llm_run = $true; quality_evidence = $false
            proxy_decisions_enabled = $true
            metrics = @{ paired = @{complete = $pairs; incomplete = 0}
                arms = @{ baseline = @{errors = $errors; budget_exhausted = 0}
                    protected = @{errors = 0; budget_exhausted = 0} } }
        } | ConvertTo-Json -Depth 20 | Set-Content (Join-Path $out 'report.json') -Encoding utf8
    }
}
try {
    try { & ./scripts/run-device-job.ps1; throw 'Unmerged core was accepted.' }
    catch { if ($_.Exception.Message -ne 'Pinned core PR not accepted.') { throw } }
    if ($global:DeviceTestCalls.Count -ne 0) { throw 'Setup or inference occurred before merge gates.' }
    $global:DeviceTestMode = 'bad-smoke'
    try { & ./scripts/run-device-job.ps1; throw 'Failed smoke was accepted.' }
    catch { if ($_.Exception.Message -notlike 'Run invalid;*') { throw } }
    if (@($global:DeviceTestCalls | Where-Object { $_ -match '--preset full' }).Count -ne 0) {
        throw 'Full ran after invalid smoke.'
    }
    $global:DeviceTestMode = 'success'
    $global:DeviceTestCalls.Clear()
    & ./scripts/run-device-job.ps1
    $generations = @($global:DeviceTestCalls | Where-Object { $_ -match 'run-agent-harness' })
    if ($generations.Count -ne 2 -or $generations[0] -notmatch '--preset smoke' -or
        $generations[1] -notmatch '--preset full' -or
        -not ($global:DeviceTestCalls | Where-Object { $_ -match 'saifety_lab_py.harness.device_report' })) {
        throw 'Expected smoke/full and report export did not execute.'
    }
    Write-Output 'Windows device runner gates and argument binding passed (mocked, no LLM).'
}
finally {
    Remove-Variable DeviceTestCalls, DeviceTestMode, DeviceTestOutput -Scope Global -ErrorAction SilentlyContinue
    foreach ($name in @('git', 'gh', 'nvidia-smi', 'ollama', 'uv')) {
        Remove-Item "Function:global:$name" -ErrorAction SilentlyContinue
    }
}
