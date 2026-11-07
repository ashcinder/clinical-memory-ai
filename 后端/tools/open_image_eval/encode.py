"""DINOv2 inference worker. Separate process keeps native runtimes isolated."""

import argparse
import json
import time
from pathlib import Path

import numpy as np
from PIL import Image, ImageOps


def read_image(path):
    with Image.open(path) as image:
        return ImageOps.exif_transpose(image).convert("RGB")


def main():
    import torch
    from transformers import AutoImageProcessor, AutoModel

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", required=True, type=Path)
    parser.add_argument("--model", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    plan = json.loads(args.plan.read_text())

    torch.set_num_threads(2)
    processor = AutoImageProcessor.from_pretrained(
        args.model, local_files_only=True, trust_remote_code=False
    )
    model = AutoModel.from_pretrained(
        args.model, local_files_only=True, trust_remote_code=False, use_safetensors=True
    ).eval()

    @torch.inference_mode()
    def encode(image):
        inputs = processor(images=image, return_tensors="pt")
        vectors = model(**inputs).last_hidden_state[:, 0]
        return torch.nn.functional.normalize(vectors, dim=1).numpy().astype(np.float32)

    encode(Image.new("RGB", (224, 224)))
    started = time.perf_counter()
    gallery = np.concatenate([encode(read_image(path)) for path in plan["gallery"]])
    gallery_seconds = time.perf_counter() - started
    print(f"Encoded {len(gallery)} gallery images in {gallery_seconds:.2f} s", flush=True)

    queries = []
    decode_ms = []
    encode_ms = []
    for number, path in enumerate(plan["queries"], 1):
        started = time.perf_counter()
        image = read_image(path)
        decoded = time.perf_counter()
        queries.append(encode(image))
        finished = time.perf_counter()
        decode_ms.append((decoded - started) * 1000)
        encode_ms.append((finished - decoded) * 1000)
        if number % 100 == 0 or number == len(plan["queries"]):
            print(f"Encoded {number}/{len(plan['queries'])} queries", flush=True)
    np.savez(
        args.output,
        gallery=gallery,
        queries=np.concatenate(queries),
        gallery_seconds=gallery_seconds,
        decode_ms=decode_ms,
        encode_ms=encode_ms,
        torch_version=torch.__version__,
    )


if __name__ == "__main__":
    main()
