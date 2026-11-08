# 开源图片检索本地对照

2026-11-07，实验分支 codex/image-retrieval-local，基线 715214a。脚本读取上次冻结评测的 502 个查询位置和相同图库、标签范围。特征提取、排序和几何核验都在本机执行。代码入口是 evaluate.py，输出为逐条 JSON。

## GitHub 项目核对

| 项目 | 许可和当前证据 | 适用位置 |
|---|---|---|
| DINOv2-small | 通用权重 Apache-2.0，已实际加载并跑完对照。仓库的 XRay-DINO 专用权重另有非商用条款。 [仓库及许可](https://github.com/facebookresearch/dinov2) · [本轮模型卡](https://huggingface.co/facebook/dinov2-small) | 从图片提取 384 维向量，用于召回 |
| Faiss | MIT，安装并验证 1.15.1。 [仓库](https://github.com/facebookresearch/faiss) · [发布记录](https://github.com/facebookresearch/faiss/releases/tag/v1.15.1) | IndexFlatIP 对归一化向量精确检索，适合作为索引对照 |
| OpenCV | Apache-2.0，实际运行 5.0.0。 [仓库](https://github.com/opencv/opencv) · [几何匹配文档](https://docs.opencv.org/4.x/d1/de0/tutorial_py_feature_homography.html) | SIFT + RANSAC 核对两张图的局部特征能否对齐 |
| USearch | Apache-2.0，查到 v2.26.6 和 Go 绑定。 [许可](https://github.com/unum-cloud/usearch/blob/main/LICENSE) · [发布](https://github.com/unum-cloud/usearch/releases/tag/v2.26.6) · [Go 安装](https://github.com/unum-cloud/usearch/blob/main/golang/README.md) | Go 可用，需随部署处理原生库。本轮为备选 |
| Qdrant | Apache-2.0，查到 v1.20.0 和按载荷过滤的接口。 [仓库](https://github.com/qdrant/qdrant) · [发布](https://github.com/qdrant/qdrant/releases/tag/v1.20.0) · [过滤文档](https://qdrant.tech/documentation/search/filtering/) | 需要独立管理向量服务时，可评估数据持久化及按租户、标签过滤。本轮为备选 |

## 召回和核验

表中的原流程覆盖数来自上次对照里实际送去 DeepSeek 比对的候选。DINOv2 用官方预处理和归一化 CLS 特征计算余弦相似度。使用冻结的原图 ID 计分，跨来源穴位单列。

| 测试组 | 原流程候选覆盖 | DINOv2 前 5 | DINOv2 前 12 | DINOv2 前 24 |
|---|---:|---:|---:|---:|
| 公开穴位 | 40/40 | 40/40 | 40/40 | 40/40 |
| 公开医学与解剖 | 52/52 | 52/52 | 52/52 | 52/52 |
| 超声 40 张图库 | 73/80 | 77/80 | 80/80 | 80/80 |
| 超声 500 张图库 | 53/80 | 77/80 | 79/80 | 80/80 |
| 超声 500 张图库，排除疑似重复查询 | 33/48 | 45/48 | 47/48 | 48/48 |
| 混合图库 | 24/24 | 24/24 | 24/24 | 24/24 |
| 跨来源同穴位，两组 | 8/8 | 8/8 | 8/8 | 8/8 |

500 张图库的前 12 名找回原流程漏掉的 26 个目标，保留了原流程找回的全部 53 个。剩下一个裁剪查询 busi-train-163-crop 的目标排在第 17。按现有每批 4 张候选计算，12 张候选对应最多 3 批，24 张候选对应最多 6 批。实际接口耗时和最终命中率还需要用候选接入后的链路实测。

使用 NumPy 点积和独立排序核对 Faiss，502/502 条查询的候选 ID 顺序一致，Faiss 返回的相似度也通过绝对误差 1e-5 的复核。两个索引计算使用相同特征，排名一致说明索引结果正确复现了点积排序。第二轮完整复跑复现了 502 条候选顺序、排名和几何判定。向量排序对每张查询图都能给出候选，因此库外拒绝单独由几何核验统计。

几何核验采用前 5 个候选，Lowe 比值 0.75，至少 12 个内点、内点比例 0.65、查询图内点凸包面积占比 0.04，RANSAC 重投影阈值 3 像素。规则在读取结果前设定。

| 测试组 | 几何核验后首条命中 | 库外误接受 |
|---|---:|---:|
| 公开穴位 | 35/40 | 2/8 |
| 公开医学与解剖 | 49/52 | 0/8 |
| 超声 40 张图库 | 77/80 | 0/60 |
| 超声 500 张图库 | 62/80 | 6/20 |
| 超声 500 张图库，排除疑似重复查询 | 45/48 | 0/14 |
| 跨来源同穴位，两组 | 4/8 | — |

穴位的 5 个错误首条涉及 LU10→LU7、LU2→LU1、LU9→PC6。一个 LU10 裁剪查询和 LU7 候选有 89 个内点，内点比例 93.7%。共享底图可以产生很强的几何一致性，点位仍可能不同。OpenCV 的这条规则适合提供同源证据，穴位接受仍需明确核对标记、编号或局部位置。超声 500 张图库的 6 个库外误接受均来自上轮审计标记的近重复查询。排除疑似重复后的 14 个负例只提供小样本观测。合成头部干扰和稳定性重试的结果在 JSON 里单列。

## 效率

Apple M5，macOS ARM64，单请求计时。PyTorch 用两个 CPU 线程，Faiss 和 OpenCV 各用一个。579 张不同图库图完成解码和编码用时 10.94 秒，建立几何特征用时 5.08 秒。

| 500 张图库阶段 | P50 | P95 |
|---|---:|---:|
| 图片解码 | 0.40 ms | 0.49 ms |
| DINOv2 编码 | 17.60 ms | 17.84 ms |
| Faiss 向量检索 | 18.3 μs | 25.8 μs |
| NumPy 点积和排序 | 15.4 μs | 32.3 μs |
| OpenCV 前 5 核验，含查询特征提取 | 11.47 ms | 13.52 ms |
| 解码、编码、Faiss 和 OpenCV 各段用时之和 | 29.49 ms | 31.97 ms |

主要时间花在特征提取和几何核验，Faiss 在这批 500 张图库数据上的优势很小。建议先用 DINOv2 加精确余弦召回验证候选预算，再根据实测图库增长决定是否引入近似索引或独立向量服务。候选阶段的收益来自图片特征和排序，Faiss 与 NumPy 复现了同一排名。

计时取自模型加载后和已有图库特征的阶段，包括查询图解码。临时特征文件交接、进程启动、数据库读写、网络和 DeepSeek 核验在计时范围之外。病例或人体独立性沿用原评测限制，排除重复查询也沿用原审计清单。

## 复跑

在实验 worktree 根目录运行。使用本机已冻结的上轮评测目录，输出路径需在冻结目录之外。

```sh

mkdir -p 后端/output/open-image-retrieval/dinov2-small
python3.13 -m venv 后端/output/open-image-retrieval/venv
后端/output/open-image-retrieval/venv/bin/pip install --only-binary=:all: \
  -r 后端/tools/open_image_eval/requirements.txt
model_dir=后端/output/open-image-retrieval/dinov2-small
revision=ed25f3a31f01632728cabb09d1542f84ab7b0056
for filename in config.json preprocessor_config.json model.safetensors; do
  curl --fail --location "https://huggingface.co/facebook/dinov2-small/resolve/$revision/$filename" \
    --output "$model_dir/$filename" || exit 1
done
printf '%s  %s\n' ae1e99fcefd534ed978cdeb8326f08030c96e28b7a81ffcbc98a857c84d14be1 \
  "$model_dir/model.safetensors" | shasum -a 256 -c -
OPENBLAS_NUM_THREADS=1 VECLIB_MAXIMUM_THREADS=1 OMP_NUM_THREADS=1 \
HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 \
后端/output/open-image-retrieval/venv/bin/python 后端/tools/open_image_eval/evaluate.py \
  --benchmark '/Users/tangyucinder/开发/多模态AI数据库检索/后端/output/image-benchmark-20261107' \
  --model "$model_dir" \
  --output 后端/output/open-image-retrieval/results.json
```

本机试跑发现 PyTorch 和 Faiss 的 wheel 带有不同的 OpenMP 运行库，同进程会中止。实验用独立编码进程，经过临时 NPZ 传递特征。模型权重、环境和评测明细保存在实验 worktree 的 output 下，分支提交包含脚本、依赖版本和这份报告。

## 在实验分支的 Web / Android 检索中启用

先按上节准备本地权重与 Python 环境。把下列选项放到实验分支的 `后端/.env`，或在启动 Go 服务时通过环境变量传入。图片理解服务在临记设置中选 DeepSeek，测试数据目录和 MySQL 库单独配置。

```dotenv
IMAGE_DINO_MODEL=后端/output/open-image-retrieval/dinov2-small
IMAGE_DINO_PYTHON=后端/output/open-image-retrieval/venv/bin/python
IMAGE_VISUAL_CANDIDATES=24
```

配置路径相对于项目根目录，视觉候选上限可设为 1–24。用户开启云端图片搜索且图库超出候选上限时，`POST /api/search` 在原有 SQL 筛选范围内合并指纹、DINOv2 及文字候选，然后通过原来的 DeepSeek 批量原图核验。结构化文字、冲突与相关性门槛沿用原流程。响应中的 `engine` 会标示 DINOv2 是否参与。云端开启且图库可以全量比较时，直接使用原视觉核验流程。关闭「云端 AI」后，已配置的 DINOv2 仍在本机排序，最多显示 5 条外观相似候选，原有指纹匹配优先。候选标记为「DINOv2 本地相似候选」，供用户核对。未配置 DINOv2 时按原流程处理。

运行时用 NumPy 对归一化向量计算精确相似度，多图以各图库图的最高相似度召回。候选优先保留强指纹，再将剩余名额的约四分之三分给 DINOv2，并以原排序补齐。图库向量按原文件 SHA-256、模型和预处理版本写入数据目录下的 `image-embeddings/`，首次搜索会为缺失项计算向量。工作进程在本机串行运行，有 35 秒限时，失败时提示并回退到原候选排序。DeepSeek 仍只接收有限候选和查询图片。

### 低费用真实接口对照

`TestDINODeepSeekSmallLive` 需要显式传入 `LINJI_RUN_DINO_LIVE=1`、`LINJI_DINO_BENCHMARK`（冻结评测目录）、`LINJI_DINO_REPORT_DIR`（独立输出目录）和 `MYSQL_TEST_DSN`（可创建临时库的专用 MySQL）。用 `PROJECT_ROOT` 指向已有 DeepSeek 配置的目录以只读取得账号配置。测试使用 6 个固定查询，两种模式各最多 4 个候选，累计硬上限 12 次付费请求。余额预检失败时停止，遇到 HTTP 402 或 429 停止后续请求。测试只恢复独立数据库与公开样本缓存，报告记录命中、候选覆盖、用量和服务状态。
