"""Rank the supplied local gallery with frozen DINOv2 image features."""

import argparse
import hashlib
import json
import os
import re
import sys
import tempfile
from pathlib import Path

import numpy as np
from PIL.Image import DecompressionBombError

from encode import load_encoder, read_image


def cache_namespace(model, cache):
    # Changing a weight or preprocessing file invalidates previous embeddings.
    signature = hashlib.sha256(b"linji-dinov2-cls-v1")
    for name in ("model.safetensors", "config.json", "preprocessor_config.json"):
        with (model / name).open("rb") as file:
            signature.update(hashlib.file_digest(file, "sha256").digest())
    directory = cache / signature.hexdigest()
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    return directory


def rank(plan, model, cache):
    directory = cache_namespace(model, cache)
    encode, _ = load_encoder(model)
    query_vectors = np.concatenate([encode(read_image(item["path"])) for item in plan["queries"]])
    ranked = []
    skipped = 0
    for item in plan["gallery"]:
        if not re.fullmatch(r"[a-f0-9]{64}", item["sha"]):
            skipped += 1
            continue
        path = directory / (item["sha"] + ".npy")
        try:
            vector = np.load(path, allow_pickle=False)
            if vector.shape != query_vectors.shape[1:] or not np.isfinite(vector).all():
                raise ValueError("invalid cached vector")
        except (OSError, ValueError, EOFError, TypeError):
            try:
                vector = encode(read_image(item["path"]))[0]
            except (OSError, ValueError, DecompressionBombError):
                skipped += 1
                continue
            with tempfile.NamedTemporaryFile(dir=directory, delete=False) as tmp:
                temporary = Path(tmp.name)
                try:
                    np.save(tmp, vector, allow_pickle=False)
                    tmp.flush()
                    os.replace(temporary, path)
                finally:
                    temporary.unlink(missing_ok=True)
        score = float(np.max(query_vectors @ vector))
        if np.isfinite(score):
            ranked.append((item["id"], score))
        else:
            skipped += 1
    ranked.sort(key=lambda row: -row[1])
    return {"ids": [identifier for identifier, _ in ranked], "skipped": skipped}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--model", required=True, type=Path)
    parser.add_argument("--cache", required=True, type=Path)
    args = parser.parse_args()
    print(json.dumps(rank(json.load(sys.stdin), args.model, args.cache)))
