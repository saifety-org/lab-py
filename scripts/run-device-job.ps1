# Explicit local execution only. -Plan is offline and never generates model requests.
[CmdletBinding()]
param(
    [switch]$Plan,
    [string]$OllamaExe = 'ollama'
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
if ((Get-Location).Path -ne $root) { throw 'Run from the lab-py repository root.' }
$jobPath = Join-Path $root 'device-jobs/proxy-refusal-v2.json'
$job = Get-Content $jobPath -Raw | ConvertFrom-Json
if ($job.schema -ne 'saifety-device-job-v1' -or $job.id -ne 'proxy-refusal-v2') {
    throw 'Unknown device job.'
}
if ($Plan) { $job | ConvertTo-Json -Depth 20; exit 0 }
if ($env:OS -ne 'Windows_NT') { throw 'This request targets native Windows / RTX 5070 Ti.' }

function Invoke-Checked {
    param([string]$Program, [string[]]$Arguments)
    $result = & $Program @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Command failed: $Program" }
    return $result
}
function Assert-MainCI {
    param([string]$Repository, [string]$Commit)
    $runs = Invoke-Checked -Program 'gh' -Arguments @('run', 'list', '--repo', $Repository, '--branch', 'main',
        '--commit', $Commit, '--limit', '20', '--json', 'status,conclusion,workflowName')
    $expected = if ($Repository -eq 'saifety-org/sAIfety') { 'ci' } else { 'check' }
    $accepted = @($runs | ConvertFrom-Json | Where-Object {
        $_.workflowName -eq $expected -and $_.status -eq 'completed' -and $_.conclusion -eq 'success'
    })
    if ($accepted.Count -eq 0) { throw "No successful main CI for $Repository at $Commit" }
}
function Invoke-Lab {
    param([string[]]$Arguments)
    Invoke-Checked -Program 'uv' -Arguments (@('run', '--locked', 'python', '-c',
        'from saifety_lab_py.cli import main; main()') + $Arguments)
}
function Assert-Run {
    param([string]$Directory, [int]$Pairs)
    $report = Get-Content (Join-Path $Directory 'report.json') -Raw | ConvertFrom-Json
    if (-not $report.identity_stable -or -not $report.llm_run -or $report.quality_evidence -or
        -not $report.proxy_decisions_enabled -or $report.metrics.paired.complete -ne $Pairs -or
        $report.metrics.paired.incomplete -ne 0 -or $report.metrics.arms.baseline.errors -ne 0 -or
        $report.metrics.arms.protected.errors -ne 0 -or
        $report.metrics.arms.baseline.budget_exhausted -ne 0 -or
        $report.metrics.arms.protected.budget_exhausted -ne 0) {
        throw 'Run invalid; inspect saved reports/traces before any further generation.'
    }
    return $report
}

# Fail before setup or inference on an unaccepted/dirty checkout or unmet dependency.
$dirty = Invoke-Checked -Program 'git' -Arguments @('status', '--porcelain')
if ($dirty) { throw 'Use a clean accepted checkout; preserve local changes.' }
$sha = (Invoke-Checked -Program 'git' -Arguments @('rev-parse', 'HEAD')).Trim()
$main = Invoke-Checked -Program 'gh' -Arguments @('api', 'repos/saifety-org/lab-py/commits/main') | ConvertFrom-Json
$comparison = Invoke-Checked -Program 'gh' -Arguments @('api', "repos/saifety-org/lab-py/compare/$sha...$($main.sha)") | ConvertFrom-Json
if ($comparison.status -notin @('identical', 'ahead')) { throw 'lab-py commit is not accepted on main.' }
Assert-MainCI 'saifety-org/lab-py' $sha
$lock = Get-Content $job.source_lock -Raw | ConvertFrom-Json
$pull = Invoke-Checked -Program 'gh' -Arguments @('api', "repos/saifety-org/sAIfety/pulls/$($job.core_pull_request)") | ConvertFrom-Json
if (-not $pull.merged -or $pull.head.sha -ne $lock.core.commit) { throw 'Pinned core PR not accepted.' }
Assert-MainCI 'saifety-org/sAIfety' $pull.merge_commit_sha

$env:GOTOOLCHAIN = 'go1.26.0+auto'
Invoke-Checked -Program 'uv' -Arguments @('sync', '--locked') | Out-Host
Invoke-Lab -Arguments @('bootstrap-agent-harness', '--lock', $job.source_lock, '--out', 'bin/agent-harness-refusal-v2') | Out-Host
Invoke-Lab -Arguments @('prepare-agent-harness', '--lock', $job.source_lock) | Out-Host
$modelCheck = @'
import sys
from saifety_lab_py.harness.agents import OllamaAgent
agent = OllamaAgent(sys.argv[1], sys.argv[2])
if agent.identity['digest'] != sys.argv[3] or agent.identity['version'] != '0.18.0':
    raise SystemExit('Model digest or Ollama version differs from accepted experiment')
'@
Invoke-Checked -Program 'uv' -Arguments @('run', '--locked', 'python', '-c', $modelCheck,
    $job.ollama_url, $job.model, $job.model_digest) | Out-Host

$runId = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ') + '-' + [guid]::NewGuid().ToString('N').Substring(0, 8)
$output = Join-Path $job.output_root $runId
New-Item -ItemType Directory -Path $output | Out-Null
$metadata = @{
    job = $job.id; lab_py_commit = $sha
    job_sha256 = (Get-FileHash $jobPath -Algorithm SHA256).Hash.ToLowerInvariant()
    source_lock_sha256 = (Get-FileHash $job.source_lock -Algorithm SHA256).Hash.ToLowerInvariant()
    started_utc = (Get-Date).ToUniversalTime().ToString('o')
}
$metadata | ConvertTo-Json | Set-Content (Join-Path $output 'device.json') -Encoding utf8
$gpu = Invoke-Checked -Program 'nvidia-smi' -Arguments @('--query-gpu=name,driver_version,memory.total', '--format=csv')
$gpu | Set-Content (Join-Path $output 'gpu.csv') -Encoding utf8
if (-not ($gpu -match 'RTX 5070 Ti')) { throw 'Expected GPU not found; no inference started.' }
$common = @('--lock', $job.source_lock, '--binary', 'bin/agent-harness-refusal-v2/saifety.exe',
    '--agent', 'ollama', '--model', $job.model, '--ollama-url', $job.ollama_url, '--proxy-decisions')
Invoke-Lab -Arguments (@('run-agent-harness') + $common + @('--preset', 'smoke', '--out', "$output/smoke")) | Out-Host
$smoke = Assert-Run "$output/smoke" 4
$env:OLLAMA_HOST = $job.ollama_url
$residency = Invoke-Checked -Program $OllamaExe -Arguments @('ps')
$residency | Set-Content (Join-Path $output 'ollama-ps.txt') -Encoding utf8
if (-not ($residency | Where-Object { $_ -match 'qwen3:8b-q4_K_M' -and $_ -match '100% GPU' })) {
    throw 'Model not fully on GPU after smoke; inspect residency, do not continue.'
}
Invoke-Lab -Arguments (@('run-agent-harness') + $common + @('--preset', 'full', '--out', "$output/full")) | Out-Host
$full = Assert-Run "$output/full" 48
Invoke-Checked -Program 'uv' -Arguments @('run', '--locked', 'python', '-m', 'saifety_lab_py.harness.device_report',
    '--run', $output) | Out-Host
Write-Output "Saved $output. Review candidate-report.json and traces; publish via task/PR per runbook."
