#!/usr/bin/env python3
"""Offline candidate retrieval and geometry evaluation on the frozen image benchmark."""

import argparse
import hashlib
import json
import platform
import subprocess
import sys
import tempfile
import time
from collections import defaultdict
from pathlib import Path

import cv2
import faiss
import numpy as np
from PIL import Image

from encode import read_image


TOP_K = 24
VERIFY_K = 5
MAX_FEATURES = 400
MAX_EDGE = 640
LOWE_RATIO = 0.75
MIN_INLIERS = 12
MIN_INLIER_RATIO = 0.65
MIN_QUERY_COVERAGE = 0.04
RANSAC_PIXELS = 3.0


def read_json(path):
    return json.loads(path.read_text())


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


class Geometry:
    def __init__(self):
        self.detector = cv2.SIFT_create(nfeatures=MAX_FEATURES)
        self.matcher = cv2.BFMatcher(cv2.NORM_L2)

    def describe(self, image):
        image = image.copy()
        image.thumbnail((MAX_EDGE, MAX_EDGE), Image.Resampling.LANCZOS)
        gray = np.asarray(image.convert("L"))
        points, descriptors = self.detector.detectAndCompute(gray, None)
        coordinates = np.asarray([p.pt for p in points], dtype=np.float32)
        return coordinates, descriptors, image.width * image.height

    def compare(self, query, candidate):
        query_points, query_desc, query_area = query
        candidate_points, candidate_desc, _ = candidate
        if query_desc is None or candidate_desc is None or len(candidate_desc) < 2:
            return {"accepted": False, "inliers": 0}

        pairs = self.matcher.knnMatch(query_desc, candidate_desc, k=2)
        good = [a for a, b in pairs if a.distance < LOWE_RATIO * b.distance]
        # Allow at most one correspondence per candidate keypoint.
        unique = {}
        for match in sorted(good, key=lambda item: item.distance):
            unique.setdefault(match.trainIdx, match)
        good = list(unique.values())
        if len(good) < MIN_INLIERS:
            return {"accepted": False, "inliers": 0, "matches": len(good)}

        source = query_points[[item.queryIdx for item in good]]
        target = candidate_points[[item.trainIdx for item in good]]
        transform, mask = cv2.findHomography(source, target, cv2.RANSAC, RANSAC_PIXELS)
        if transform is None or mask is None or not np.isfinite(transform).all():
            return {"accepted": False, "inliers": 0, "matches": len(good)}

        inlier_points = source[mask.ravel().astype(bool)]
        inliers = len(inlier_points)
        ratio = inliers / len(good)
        coverage = 0.0
        if inliers >= 3:
            coverage = cv2.contourArea(cv2.convexHull(inlier_points)) / query_area
        return {
            "accepted": inliers >= MIN_INLIERS
            and ratio >= MIN_INLIER_RATIO
            and coverage >= MIN_QUERY_COVERAGE,
            "inliers": inliers,
            "matches": len(good),
            "inlier_ratio": ratio,
            "query_coverage": coverage,
        }


class Gallery:
    def __init__(self, rows, vectors, geometry):
        self.ids = [row["id"] for row in rows]
        self.vectors = np.concatenate([vectors[row["file"]] for row in rows])
        self.geometry = [geometry[row["file"]] for row in rows]
        self.index = faiss.IndexFlatIP(self.vectors.shape[1])
        self.index.add(self.vectors)

    def numpy_search(self, vector, count):
        scores = self.vectors @ vector[0]
        subset = np.argpartition(-scores, count - 1)[:count]
        ranked = subset[np.argsort(-scores[subset], kind="stable")]
        return scores, ranked

    def search(self, vector):
        count = min(TOP_K, len(self.ids))
        started = time.perf_counter_ns()
        scores, indices = self.index.search(vector, count)
        faiss_us = (time.perf_counter_ns() - started) / 1000
        started = time.perf_counter_ns()
        numpy_scores, numpy_indices = self.numpy_search(vector, count)
        numpy_us = (time.perf_counter_ns() - started) / 1000
        indices = indices[0]
        if not np.allclose(scores[0], numpy_scores[indices], atol=1e-5, rtol=0):
            raise RuntimeError("Faiss and NumPy inner products disagree")
        return indices, {
            "faiss_us": faiss_us,
            "numpy_us": numpy_us,
            "same_top24_order": bool(np.array_equal(indices, numpy_indices)),
            "same_top24_set": set(indices) == set(numpy_indices),
        }


def group_name(row):
    if row["task"] in {"mixed_gallery", "named_point_cross_source", "stability"}:
        return row["task"]
    return row["source_type"]


def aggregate(rows):
    positive = [r for r in rows if r["expected_source_ids"]]
    negative = [r for r in rows if not r["expected_source_ids"]]
    return {
        "queries": len(rows),
        "positive": len(positive),
        "negative": len(negative),
        "previous_candidate_hits": sum(r["previous_candidate_hit"] for r in positive),
        "previous_top1_hits": sum(r["previous_top1"] for r in positive),
        "candidate_hits": {
            str(k): sum(0 < r["rank"] <= k for r in positive) for k in (1, 5, 12, 24)
        },
        "geometry_top1_hits": sum(r["geometry_top1"] for r in positive),
        "geometry_false_accepts": sum(bool(r["accepted_ids"]) for r in negative),
        "geometry_returns_wrong_top1": sum(
            bool(r["accepted_ids"]) and not r["geometry_top1"] for r in positive
        ),
        "positive_rank_by_variant": {
            variant: {
                "queries": sum(r["variant"] == variant for r in positive),
                "hit24": sum(r["variant"] == variant and 0 < r["rank"] <= 24 for r in positive),
            }
            for variant in sorted({r["variant"] for r in positive})
        },
        "timing": {
            key: {f"p{p}": float(np.percentile([r[key] for r in rows], p)) for p in (50, 95)}
            for key in (
                "decode_ms", "encode_ms", "faiss_us", "numpy_us", "geometry_ms", "local_total_ms"
            )
        },
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--benchmark", required=True, type=Path)
    parser.add_argument("--model", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    benchmark = args.benchmark.resolve()
    if args.output.resolve().is_relative_to(benchmark):
        parser.error("Write the experiment outside the frozen benchmark directory.")

    cv2.setNumThreads(1)
    cv2.setRNGSeed(20261107)
    faiss.omp_set_num_threads(1)
    old_results = read_json(benchmark / "final-selected-results.json")["results"]
    suites = {}
    for row in old_results:
        suite = Path(row["suite_dir"]).relative_to(benchmark)
        if suite not in suites:
            manifest_path = benchmark / suite / "manifest.json"
            manifest = read_json(manifest_path)
            original = read_json(benchmark / suite / "results.json")
            if digest(manifest_path) != original["manifest_sha256"]:
                raise ValueError(f"Frozen manifest checksum failed: {suite}")
            suites[suite] = manifest

    paths = list(dict.fromkeys(
        row["file"] for manifest in suites.values() for row in manifest["gallery"]
    ))
    with tempfile.TemporaryDirectory(prefix="linji-open-image-") as directory:
        plan = Path(directory) / "plan.json"
        cache = Path(directory) / "vectors.npz"
        plan.write_text(json.dumps({
            "gallery": paths,
            "queries": [row["query_file"] for row in old_results],
        }))
        subprocess.run([
            sys.executable, str(Path(__file__).with_name("encode.py")),
            "--plan", str(plan), "--model", str(args.model), "--output", str(cache),
        ], check=True)
        with np.load(cache) as data:
            gallery_vectors = data["gallery"]
            vectors = {path: gallery_vectors[i:i + 1] for i, path in enumerate(paths)}
            query_vectors = data["queries"]
            decode_times = data["decode_ms"]
            encode_times = data["encode_ms"]
            gallery_encode_seconds = float(data["gallery_seconds"])
            torch_version = str(data["torch_version"])

    geometry = Geometry()
    started = time.perf_counter()
    features = {path: geometry.describe(read_image(path)) for path in paths}
    gallery_geometry_seconds = time.perf_counter() - started

    galleries = {}
    cases = {}
    results = []
    groups = defaultdict(list)
    for index, old in enumerate(old_results, start=1):
        suite = Path(old["suite_dir"]).relative_to(benchmark)
        scope = old["scope"]
        key = suite, scope
        manifest = suites[suite]
        if suite not in cases:
            cases[suite] = {case["name"]: case for case in manifest["cases"]}
        case = cases[suite][old["name"]]
        if case["expected_source_ids"] != old["expected_source_ids"] or case["scope"] != scope:
            raise ValueError(f"Previous result and frozen manifest disagree: {old['name']}")
        if key not in galleries:
            rows = [r for r in manifest["gallery"] if not scope or r["cohort"] == scope]
            galleries[key] = Gallery(rows, vectors, features)
        gallery = galleries[key]
        expected = set(case["expected_source_ids"])
        if not expected.issubset(gallery.ids):
            raise ValueError(f"Expected image outside the original search scope: {case['name']}")

        if case["file"] != old["query_file"]:
            raise ValueError(f"Query file differs from frozen manifest: {case['name']}")
        image = read_image(case["file"])
        decode_ms = float(decode_times[index - 1])
        encode_ms = float(encode_times[index - 1])
        vector = query_vectors[index - 1:index]
        indices, search = gallery.search(vector)
        candidates = [gallery.ids[i] for i in indices]
        rank = next((i for i, candidate in enumerate(candidates, 1) if candidate in expected), 0)

        started = time.perf_counter_ns()
        query_geometry = geometry.describe(image)
        evidence = [
            {"source_id": gallery.ids[i], **geometry.compare(query_geometry, gallery.geometry[i])}
            for i in indices[:VERIFY_K]
        ]
        geometry_ms = (time.perf_counter_ns() - started) / 1e6
        accepted = [item["source_id"] for item in evidence if item["accepted"]]
        result = {
            "name": old["name"],
            "suite": str(suite),
            "task": old["task"],
            "source_id": case["source_id"],
            "variant": old["variant"],
            "gallery_size": len(gallery.ids),
            "expected_source_ids": case["expected_source_ids"],
            "candidate_ids": candidates,
            "rank": rank,
            "previous_candidate_hit": bool(old["target_in_visual_candidates"]),
            "previous_top1": bool(old["top1_hit"]),
            "previous_search_seconds": old["search_seconds"],
            "geometry_evidence": evidence,
            "accepted_ids": accepted,
            "geometry_top1": bool(accepted and accepted[0] in expected),
            "decode_ms": decode_ms,
            "encode_ms": encode_ms,
            "geometry_ms": geometry_ms,
            "local_total_ms": decode_ms + encode_ms + search["faiss_us"] / 1000 + geometry_ms,
            **search,
        }
        results.append(result)
        groups[group_name(old)].append(result)
        if index % 50 == 0 or index == len(old_results):
            print(f"Evaluated {index}/{len(old_results)} queries", flush=True)

    duplicate_audit = read_json(benchmark / "large-gallery" / "near-duplicate-audit.json")
    large = groups["clinical_ultrasound_large"]
    flagged = {
        True: set(duplicate_audit["flagged_gallery_source_ids"]),
        False: set(duplicate_audit["flagged_heldout_source_ids"]),
    }
    sensitivity = [
        row for row in large if row["source_id"] not in flagged[bool(row["expected_source_ids"])]
    ]
    summary = {group: aggregate(rows) for group, rows in groups.items()}
    summary["large_gallery_duplicate_sensitivity"] = aggregate(sensitivity)
    report = {
        "platform": platform.platform(),
        "versions": {"torch": torch_version, "opencv": cv2.__version__, "faiss": faiss.__version__},
        "model_sha256": digest(args.model / "model.safetensors"),
        "baseline_sha256": digest(benchmark / "final-selected-results.json"),
        "gallery_images_encoded": len(vectors),
        "gallery_encode_seconds": gallery_encode_seconds,
        "gallery_geometry_seconds": gallery_geometry_seconds,
        "query_count": len(results),
        "faiss_numpy_same_top24_order": sum(r["same_top24_order"] for r in results),
        "faiss_numpy_same_top24_set": sum(r["same_top24_set"] for r in results),
        "groups": summary,
        "results": results,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_suffix(".tmp")
    temporary.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    temporary.replace(args.output)


if __name__ == "__main__":
    main()
