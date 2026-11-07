#!/usr/bin/env python3
"""Convert selected BodyParts3D 4.0 archive meshes to labelled, local GLB assets."""

import argparse
import csv
import hashlib
import json
import struct
from collections import defaultdict
from pathlib import Path
from zipfile import ZipFile

import numpy as np


ROOT = Path(__file__).resolve().parents[2]
SOURCE = "https://dbarchive.biosciencedbc.jp/en/bodyparts3d/"
CREDIT = "BodyParts3D, © The Database Center for Life Science licensed under CC Attribution 4.0 International"
# FMA concept, Chinese name, display color, and optional source hierarchy.
MODELS = [
    ("skeleton", "全身骨骼总览", "Skeleton overview", "骨骼系统", "成年男性参考骨骼。按颅骨、躯干与四肢分区观察，可分别显示或隐藏。",
     ["全身", "骨架", "骨骼", "人体"],
     [(46565, "颅骨", "#ded4bf"), (7480, "胸廓", "#ded3bb"),
      (72062, "骨盆", "#d9cbb1"), (13478, "脊柱（骶骨归入骨盆）", "#d9cdb6"),
      (24163, "右肩带骨", "#ddd2bb"), (24164, "左肩带骨", "#ddd2bb"),
      (24880, "右上肢骨", "#e1d6bf"), (24881, "左上肢骨", "#e1d6bf"),
      (24882, "右下肢骨", "#ddd1bb"), (24883, "左下肢骨", "#ddd1bb")]),
    ("heart", "心脏与大血管", "Heart & great vessels", "循环系统", "心房壁、合并心室壁及相连大血管。心室腔以空间表面表示，可单独查看。",
     ["心脏", "心血管", "冠状动脉", "心房", "心室"],
     [
         (13884, "心室壁", "#b45053", "isa"), (9457, "右心房壁", "#866c9d"),
         (9531, "左心房壁", "#d38985"), (3736, "升主动脉", "#cd665a"),
         (3768, "主动脉弓", "#e38367"), (8612, "肺动脉干", "#758aa9"),
         (4720, "上腔静脉", "#678199"), (9291, "右心室腔", "#7897b7"),
         (9466, "左心室腔", "#e5ac75"), (50039, "右冠状动脉树", "#e7b872"),
         (50040, "左冠状动脉树", "#f3cd8a"),
     ]),
    ("airways", "气管与支气管树", "Trachea & bronchial trees", "呼吸系统", "沿气管向下观察左右支气管的分叉与肺内走行。",
     ["肺", "肺部", "气道", "呼吸", "lung", "bronchus"],
     [(7394, "气管", "#d6c6a5"), (26661, "右支气管树", "#5b979b"),
      (26662, "左支气管树", "#86a5bf")]),
    ("skull", "颅骨与下颌", "Skull & mandible", "骨骼系统", "分开观察颅盖、颅底部分骨骼和下颌骨。",
     ["头骨", "颅骨", "头部", "骨骼", "下颌"],
     [(52734, "额骨", "#e2d1af"), (52735, "枕骨", "#c4ae91"),
      (52736, "蝶骨", "#b296ac"), (52788, "右顶骨", "#dbbf9b"),
      (52789, "左顶骨", "#c9bea1"), (52738, "右颞骨", "#a9b3a0"),
      (52739, "左颞骨", "#a9b3a0"), (52892, "右颧骨", "#cfb494"),
      (52893, "左颧骨", "#cfb494"), (53649, "右上颌骨", "#d9c7ad"),
      (53650, "左上颌骨", "#d9c7ad"), (52748, "下颌骨", "#d2aa85")]),
    ("knee", "右膝骨性结构", "Right knee bones", "骨骼系统", "查看右侧股骨、髌骨、胫骨与腓骨的相对位置。",
     ["膝关节", "膝盖", "下肢", "骨骼"],
     [(24474, "右股骨", "#decba8"), (24486, "右髌骨", "#c79672"),
      (24477, "右胫骨", "#c1bd9a"), (24480, "右腓骨", "#aebcae")]),
    ("shoulder", "右肩与上臂肌群", "Right shoulder & upper arm", "肌肉系统", "观察肩带骨骼与三角肌、肱二头肌、肱三头肌的相对位置。",
     ["肩关节", "肩臂", "肌肉", "上肢", "biceps", "deltoid"],
     [(13322, "右锁骨", "#e2d2b5"), (13395, "右肩胛骨", "#ddcdb1"),
      (23130, "右肱骨", "#e3d5b8"), (34680, "三角肌锁骨部", "#d19481", "isa"),
      (34682, "三角肌肩峰部", "#ba6a67", "isa"), (34684, "三角肌肩胛冈部", "#ab6569", "isa"),
      (37684, "肱二头肌短头", "#d08675", "isa"), (37686, "肱二头肌长头", "#b9615e", "isa"),
      (37695, "肱三头肌内侧头", "#8e656c", "isa"),
      (37697, "肱三头肌外侧头", "#a16d80", "isa"),
      (37699, "肱三头肌长头", "#ad7b86", "isa")]),
    ("kidneys", "双肾", "Kidneys", "泌尿系统", "对照左右肾的外形与相对位置，可分别选中。",
     ["肾脏", "腰腹", "肾"],
     [(7204, "右肾", "#b26d72"), (7205, "左肾", "#c8817a")]),
]


def read_obj(raw):
    points, triangles = [], []
    for line in raw.decode("ascii", errors="replace").splitlines():
        if line.startswith("v "):
            points.append([float(value) for value in line.split()[1:4]])
        elif line.startswith("f "):
            face = [int(value.split("/")[0]) - 1 for value in line.split()[1:]]
            triangles.extend((face[0], face[i], face[i + 1]) for i in range(1, len(face) - 1))
    vertices, faces = np.array(points, dtype="<f4"), np.array(triangles, dtype="<u4")
    if not len(vertices) or not len(faces) or faces.max() >= len(vertices) or not np.isfinite(vertices).all():
        raise ValueError("Invalid source mesh")
    return vertices, faces


def export_glb(parts, path):
    binary = bytearray()
    gltf = {
        "asset": {"version": "2.0", "copyright": CREDIT},
        "scene": 0, "scenes": [{"nodes": list(range(len(parts)))}],
        "nodes": [], "meshes": [], "accessors": [], "bufferViews": [],
    }

    def accessor(values, kind, code, target, bounds=False):
        view = len(gltf["bufferViews"])
        raw = values.tobytes()
        gltf["bufferViews"].append({"buffer": 0, "byteOffset": len(binary), "byteLength": len(raw), "target": target})
        binary.extend(raw)
        index = len(gltf["accessors"])
        data = {"bufferView": view, "componentType": code, "count": len(values), "type": kind}
        if bounds:
            data.update(min=values.min(axis=0).tolist(), max=values.max(axis=0).tolist())
        gltf["accessors"].append(data)
        return index

    all_positions = np.concatenate([vertices for _, vertices, _ in parts])
    center = (all_positions.min(axis=0) + all_positions.max(axis=0)) / 2
    scale = 2 / np.ptp(all_positions, axis=0).max()
    for number, (part, vertices, faces) in enumerate(parts):
        vertices = np.ascontiguousarray((vertices - center) * scale, dtype="<f4")
        points = vertices[faces]
        surface = np.cross(points[:, 1] - points[:, 0], points[:, 2] - points[:, 0])
        normals = np.zeros_like(vertices)
        for corner in range(3):
            np.add.at(normals, faces[:, corner], surface)
        normals /= np.maximum(np.linalg.norm(normals, axis=1, keepdims=True), 1e-9)
        position = accessor(vertices, "VEC3", 5126, 34962, bounds=True)
        normal = accessor(normals, "VEC3", 5126, 34962)
        indices = accessor(faces.ravel(), "SCALAR", 5125, 34963)
        gltf["nodes"].append({"name": part["id"], "mesh": number, "extras": {"fma": part["id"]}})
        gltf["meshes"].append({"primitives": [{"attributes": {"POSITION": position, "NORMAL": normal}, "indices": indices}]})
    gltf["buffers"] = [{"byteLength": len(binary)}]
    data = json.dumps(gltf, separators=(",", ":")).encode()
    data += b" " * (-len(data) % 4)
    path.write_bytes(
        struct.pack("<4sII", b"glTF", 2, 28 + len(data) + len(binary))
        + struct.pack("<I4s", len(data), b"JSON") + data
        + struct.pack("<I4s", len(binary), b"BIN\0") + binary
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archives", type=Path, help="Directory containing the two official archives and element tables")
    args = parser.parse_args()
    dest = ROOT / "前端Web/anatomy/library"
    dest.mkdir(parents=True, exist_ok=True)
    tables = {}
    archives = {}
    input_hashes = {}
    for hierarchy, archive in [("isa", "bodyparts3d.zip"), ("partof", "partof.zip")]:
        table_path = args.archives / f"{hierarchy}_element_parts.txt"
        groups = defaultdict(list)
        with table_path.open() as file:
            for row in csv.DictReader(file, delimiter="\t"):
                groups[row["concept id"]].append((row["name"], row["element file id"]))
        tables[hierarchy] = groups
        archive_path = args.archives / archive
        with archive_path.open("rb") as file:
            input_hashes[archive] = hashlib.file_digest(file, "sha256").hexdigest()
        input_hashes[table_path.name] = hashlib.sha256(table_path.read_bytes()).hexdigest()
        zip_file = ZipFile(archive_path)
        archives[hierarchy] = zip_file, {Path(name).stem: name for name in zip_file.namelist() if name.endswith(".obj")}

    catalog = []
    bone_meshes = {mesh for _, mesh in tables["isa"]["FMA5018"]}
    for model_id, name, english, category, summary, keywords, specs in MODELS:
        parts, used = [], set()
        for fma, label, color, *preferred in specs:
            hierarchy = preferred[0] if preferred else "partof"
            concept = f"FMA{fma}"
            group = tables[hierarchy][concept]
            if model_id == "skeleton" and fma in (24880, 24881, 24882, 24883):
                group = [(f"bones of {name}", mesh) for name, mesh in group if mesh in bone_meshes]
            if model_id == "skeleton" and fma == 13478:
                group = [(f"{name} (sacrum grouped with pelvis)", mesh)
                         for name, mesh in group if mesh not in used]
            if not group:
                raise ValueError(f"Missing source mapping: {concept}")
            zip_file, entries = archives[hierarchy]
            arrays, triangles, elements = [], [], []
            offset = 0
            for english_name, mesh_id in group:
                if mesh_id in used:
                    raise ValueError(f"{model_id}: duplicate mesh {mesh_id} in {concept}")
                used.add(mesh_id)
                raw = zip_file.read(entries[mesh_id])
                vertices, faces = read_obj(raw)
                arrays.append(vertices[:, [0, 2, 1]] * [1, 1, -1])
                triangles.append(faces + offset)
                offset += len(vertices)
                elements.append({"id": mesh_id, "sha256": hashlib.sha256(raw).hexdigest()})
            part = {"id": concept, "name": label, "english": english_name.capitalize(),
                    "color": color, "hierarchy": hierarchy, "elements": elements}
            if fma in (9291, 9466):
                part["cavity"] = True
            parts.append((part, np.concatenate(arrays), np.concatenate(triangles)))
        path = dest / f"{model_id}.glb"
        export_glb(parts, path)
        print(f"{name}: {len(parts)} groups, {len(used)} source meshes, {path.stat().st_size:,} bytes", flush=True)
        catalog.append({
            "id": model_id, "name": name, "english": english, "category": category, "summary": summary,
            "keywords": keywords, "author": "Database Center for Life Science", "url": SOURCE + "desc.html",
            "license": "CC BY 4.0", "asset": path.name, "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
            "parts": [part for part, _, _ in parts],
        })
    (dest / "catalog.json").write_text(json.dumps({
        "provider": "bodyparts3d", "version": "4.0", "credit": CREDIT, "license_url": SOURCE + "lic.html",
        "inputs_sha256": input_hashes, "items": catalog,
    }, ensure_ascii=False, indent=2) + "\n")


if __name__ == "__main__":
    main()
