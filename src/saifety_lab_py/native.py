"""Train the small native classifier in Python using canonical Go features."""

import json
import math
import platform
import random
from dataclasses import dataclass
from pathlib import Path

from .bridge import Bridge
from .io import finite, samples, sha256, write_json


def sigmoid(value: float) -> float:
    if value < -30:
        return 0.0
    if value > 30:
        return 1.0
    return 1 / (1 + math.exp(-value))


@dataclass
class NativeModel:
    weights: list[float]
    bias: float

    @classmethod
    def load(cls, path: Path) -> "NativeModel":
        value = json.loads(path.read_text())
        weights = [finite(float(v)) for v in value["w"]]
        if value["dim"] != len(weights) or not weights:
            raise ValueError("invalid native model dimensions")
        return cls(weights, finite(float(value["b"])))

    def score(self, features: list[tuple[int, float]]) -> float:
        value = self.bias
        for index, feature in features:
            if index >= len(self.weights):
                raise ValueError("incompatible feature dimensions")
            value += self.weights[index] * feature
        return sigmoid(value)


def train(
    path: Path,
    output: Path,
    bridge: Bridge,
    *,
    epochs: int = 40,
    seed: int = 1,
    learning_rate: float = 0.5,
    l2: float = 1e-5,
) -> dict:
    if epochs < 1 or learning_rate <= 0 or l2 < 0:
        raise ValueError("invalid training options")
    rows = samples(path)
    if {row["label"] for row in rows} != {0, 1}:
        raise ValueError("training requires both classes")
    features = bridge.features([row["text"] for row in rows])
    model = NativeModel([0.0] * bridge.identity["dim"], 0.0)
    order = list(range(len(rows)))
    rng = random.Random(seed)
    for _ in range(epochs):
        rng.shuffle(order)
        for index in order:
            vector = features[index]
            gradient = model.score(vector) - rows[index]["label"]
            for column, feature in vector:
                model.weights[column] -= learning_rate * (
                    gradient * feature + l2 * model.weights[column]
                )
            model.bias -= learning_rate * gradient
    write_json(output, {"dim": len(model.weights), "w": model.weights, "b": model.bias})
    metadata = {
        "training_sha256": sha256(path),
        "weights_sha256": sha256(output),
        "samples": len(rows),
        "options": {"epochs": epochs, "seed": seed, "learning_rate": learning_rate, "l2": l2},
        "feature_provider": bridge.identity,
        "implementation": "python-sgd-with-go-features-v1",
        "python": platform.python_version(),
        "dataset_source": json.loads((path.parent / "source.json").read_text())
        if (path.parent / "source.json").exists()
        else "explicit input; no source manifest",
        "metrics": "not measured on training data; use frozen validation/test",
    }
    write_json(Path(str(output) + ".meta.json"), metadata)
    return metadata
