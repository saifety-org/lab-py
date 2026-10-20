"""The default research path must work without contacting the historical lab repo."""

import json
from pathlib import Path

import pytest

from saifety_lab_py import research, sources
from saifety_lab_py.harness import pins
from saifety_lab_py.io import sha256


def no_download(*_args, **_kwargs):
    pytest.fail("checkout research data must not trigger a download")


def test_default_sources_are_local_and_preserve_locked_bytes(tmp_path, monkeypatch):
    monkeypatch.setattr("urllib.request.urlopen", no_download)
    for name in ("sources", "context.sources", "encoder.sources", "encoder-v4.sources"):
        path = Path(name + ".lock.json")
        pinned = sources.sync(path, tmp_path / name)
        historical = json.loads((Path("docs/history/pre-unification-locks") / path).read_text())
        assert pinned["schema"] == 2 and pinned["repository"] == "saifety-org/lab-py"
        assert pinned["tools_version"] == research.identity()
        assert [(f["path"], f["sha256"]) for f in pinned["files"]] == [
            (f["path"], f["sha256"]) for f in historical["files"]
        ]
        for item in pinned["files"]:
            assert sha256(tmp_path / name / item["path"]) == item["sha256"]
        assert sources.lock(Path("docs/history/pre-unification-locks") / path)["schema"] == 1


def test_harness_sources_are_local_with_unchanged_core(tmp_path, monkeypatch):
    monkeypatch.setattr("urllib.request.urlopen", no_download)
    for name in ("agent-harness.sources.lock.json", "agent-harness-refusal-v2.sources.lock.json"):
        value = pins.prepare(Path(name), tmp_path / name)
        historical = json.loads((Path("docs/history/pre-unification-locks") / name).read_text())
        assert value["schema"] == "agent-harness-sources-v2"
        assert value["core"] == historical["core"]
        assert [(f["name"], f["sha256"]) for f in value["lab"]["files"]] == [
            (f["name"], f["sha256"]) for f in historical["lab"]["files"]
        ]


def test_frozen_and_regression_files_match_import_manifest():
    manifest = json.loads(Path("research/import.json").read_text())
    assert manifest["source_commit"] == "c7130bdb3aba0acbee69872c22f4988eec9c255b"
    for entry in manifest["files"]:
        if entry["source_path"].startswith(("datasets/", "testdata/")):
            assert sha256(Path(entry["destination"])) == entry["source_sha256"]


def test_collector_request_pins_checkout_without_changing_experiment():
    path = Path("device-jobs/context-corpus-v5-contracts-v4.json")
    job = json.loads(path.read_text())
    old = json.loads(Path("device-jobs/context-corpus-v5-contracts-v3.json").read_text())
    assert job["collector"]["repository"] == "saifety-org/lab-py"
    assert job["collector"]["tools_sha256"] == sha256(Path("research/tools.lock.json"))
    assert job["collector"]["operation"] == "collect-contracts-v4"
    assert sha256(path) in path.with_suffix(".md").read_text(encoding="utf-8")
    for key in (
        "generation",
        "preload",
        "total_budget",
        "protocol",
        "plan_sha256",
        "model",
        "model_digest",
        "ollama_version",
        "license",
        "data_policy",
        "sample_audit",
    ):
        assert job[key] == old[key]


@pytest.mark.parametrize(
    "path", ["../outside", "research/../../outside", "C:/outside", "research\\outside"]
)
def test_local_source_path_cannot_escape_checkout(path):
    with pytest.raises(ValueError, match="unsafe"):
        research.local_file(path)


def test_local_source_symlink_cannot_escape_checkout(tmp_path, monkeypatch):
    (tmp_path / "research").mkdir()
    try:
        (tmp_path / "research/link").symlink_to(tmp_path, target_is_directory=True)
    except OSError:
        pytest.skip("symlinks unavailable on this host")
    monkeypatch.setattr(research, "root", lambda: tmp_path)
    with pytest.raises(ValueError, match="escapes"):
        research.local_file("research/link/other")


def test_build_uses_checkout_and_attests_all_go_inputs(tmp_path, monkeypatch):
    checkout = tmp_path / "repo"
    (checkout / "research/cmd").mkdir(parents=True)
    for name in ("go.mod", "go.sum"):
        (checkout / "research" / name).write_text("fixture")
    source = checkout / "research/cmd/main.go"
    source.write_text("fixture code")
    manifest = {
        "schema": "local-research-tools-v1",
        "module": "github.com/saifety-org/lab-py/research",
        "files": [
            {"path": p.relative_to(checkout).as_posix(), "sha256": sha256(p)}
            for p in (checkout / "research/go.mod", checkout / "research/go.sum", source)
        ],
    }
    (checkout / "research/tools.lock.json").write_text(json.dumps(manifest))
    monkeypatch.setattr(research, "root", lambda: checkout)
    seen = []

    def run(args, **kwargs):
        seen.append(args)
        assert args[:2] == ["go", "build"] and not any("@" in arg for arg in args)
        assert "-mod=readonly" in args and "-buildvcs=false" in args
        assert kwargs["cwd"] == checkout / "research" and kwargs["env"]["GOWORK"] == "off"
        Path(args[args.index("-o") + 1]).write_text("fixture executable")

    monkeypatch.setattr(research.subprocess, "run", run)
    assert research.build("context-collect", tmp_path / "bin").exists()
    source.write_text("changed input")
    with pytest.raises(ValueError, match="checksum mismatch"):
        research.build("context-collect", tmp_path / "bin")
    assert len(seen) == 1
