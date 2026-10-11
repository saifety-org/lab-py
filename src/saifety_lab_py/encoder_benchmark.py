"""Batch-one, resident CPU inference including tokenization and JSONL IPC."""

import argparse
import json
import os
import platform
import statistics
import subprocess
import sys
import time
from pathlib import Path

from .contextual import partitions, view_text
from .io import text_hash, write_json


def worker(path: Path) -> None:
    import torch

    from .encoder import load_candidate, predict

    torch.set_num_threads(1)
    model, tokenizer, metadata = load_candidate(path)
    print(json.dumps({"ready": True}), flush=True)
    for line in sys.stdin:
        text = json.loads(line)["text"]
        score = predict(
            model, tokenizer, [metadata["prefix"] + text], metadata["protocol"]["max_tokens"]
        )[0]
        print(json.dumps({"score": score, "text_sha256": text_hash(text)}), flush=True)


def resident(command: list[str], documents: list[list[str]], warmup: int = 5) -> dict:
    import psutil

    env = dict(os.environ, TOKENIZERS_PARALLELISM="false", OMP_NUM_THREADS="1")
    started = time.perf_counter()
    with tempfile_stderr() as errors:
        process = subprocess.Popen(
            command,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=errors,
            text=True,
            bufsize=1,
            env=env,
        )
        try:
            observed = psutil.Process(process.pid)

            # Both workers initialize lazily/before first response; the first request
            # accounts for loading and is reported separately from warm latency.
            def request(docs):
                values = []
                for text in docs:
                    process.stdin.write(json.dumps({"text": text}) + "\n")
                    process.stdin.flush()
                    line = process.stdout.readline()
                    if line and json.loads(line).get("ready"):
                        line = process.stdout.readline()
                    if not line:
                        errors.seek(0)
                        raise RuntimeError(errors.read())
                    result = json.loads(line)
                    if result.get("text_sha256") != text_hash(text):
                        raise ValueError("benchmark response mismatch")
                    values.append(result["score"])
                return max(values)

            request(documents[0])
            startup = time.perf_counter() - started
            for i in range(warmup):
                request(documents[i % len(documents)])
            latencies, rss = [], []
            for _ in range(3):
                for docs in documents:
                    before = time.perf_counter()
                    request(docs)
                    latencies.append((time.perf_counter() - before) * 1000)
                    rss.append(observed.memory_info().rss)
            ordered = sorted(latencies)
            return {
                "requests": len(latencies),
                "warmup_requests": warmup,
                "startup_first_request_seconds": startup,
                "latency_ms_p50": statistics.median(latencies),
                "latency_ms_p95": ordered[int(0.95 * (len(ordered) - 1))],
                "sampled_resident_rss_bytes_max": max(rss),
            }
        finally:
            if process.stdin:
                process.stdin.close()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()


def tempfile_stderr():
    import tempfile

    return tempfile.TemporaryFile(mode="w+")


def benchmark(data: Path, models: Path, bridge: Path, output: Path) -> dict:
    _, rows, _ = partitions(data)
    # One fixed first example per validation family: 14 requests, three repetitions.
    # Never use test examples or labels for performance workload selection.
    representatives = {}
    for row in rows:
        if row["split"] == "validation":
            representatives.setdefault(row["group"], row)
    workload = [representatives[key] for key in sorted(representatives)]
    results = {}
    for view in ("context", "payload_only", "task_only"):
        command = [
            sys.executable,
            "-m",
            "saifety_lab_py.encoder_benchmark",
            "--worker",
            str((models / view).resolve()),
        ]
        results[f"encoder_{view}"] = resident(command, [[view_text(row, view)] for row in workload])
    for name, backend in (("shipped_raw", "native"), ("deberta_raw", "onnx")):
        command = [str(bridge.resolve()), "-backend", backend, "-mode", "score"]
        results[name] = resident(command, [row["documents"] for row in workload])
    report = {
        "hardware": platform.platform(),
        "processor": platform.processor(),
        "logical_cpus": os.cpu_count(),
        "models": results,
        "workload_ids": [row["id"] for row in workload],
        "protocol": "resident CPU process; batch=1 scenario; sequential docs max; "
        "tokenization + JSONL IPC included; 5 warmups, 3 repeats of 14 validation families",
        "memory": "maximum sampled RSS after replies, includes framework/interpreter/model; "
        "not peak training memory or process lifetime high-water mark",
        "threads": "PyTorch intra-op=1; OMP_NUM_THREADS=1; Go/ORT default thread pools",
        "limitations": "different frameworks/thread pools; not isolated kernel timings; "
        "startup includes model integrity hashing and first inference",
    }
    write_json(output, report)
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--worker", type=Path)
    parser.add_argument("--data", type=Path, default=Path("data/encoder"))
    parser.add_argument("--models", type=Path, default=Path("artifacts/encoder"))
    parser.add_argument("--bridge", type=Path, default=Path("bin/model-bridge"))
    parser.add_argument("--out", type=Path, default=Path("artifacts/encoder-benchmark.json"))
    args = parser.parse_args()
    if args.worker:
        worker(args.worker)
    else:
        benchmark(args.data, args.models, args.bridge, args.out)
