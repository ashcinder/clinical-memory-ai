# 医学模型库来源

BodyParts3D, © The Database Center for Life Science licensed under CC Attribution 4.0 International.

源网格直接取自 BodyParts3D 4.0 官方 IS-A / PART-OF 压缩包及 FMA 映射表。官方许可页自 2025-02-27 标示 CC BY 4.0，存档 OBJ 中保留较早的许可注释。这里采用当前官方存档授权。[官方下载](https://dbarchive.biosciencedbc.jp/en/bodyparts3d/download.html) · [官方许可](https://dbarchive.biosciencedbc.jp/en/bodyparts3d/lic.html)

catalog.json 包含原始输入及导出资产的 SHA256。每项结构列出其 FMA 概念、源层级和组成 OBJ 文件及 SHA256。转换合并同概念网格并应用显示坐标、比例和颜色。比例和颜色用于辨识结构。

首批为成年男性参考解剖数据的局部选集。心室壁是合并网格，左右心室腔显示的是空间表面。气管模型覆盖气道结构。颅骨、膝部和肩臂只展示目录列出的部件。穴位图谱在独立页签中展示。

构建使用 后端/tools/prepare_anatomy_library.py。模型结构和资料来源可用于解剖学习。

全身骨骼总览按源档概念分为十区。四肢区域只取与 IS-A 骨器官 FMA5018 有交集的网格，骶骨归入骨盆，胸廓包含源档肋软骨。所有部件保留在源坐标中的相对位置。
