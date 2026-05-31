---
name: property-utility-b-custom
description: Fill property water/electric confirmation Excel files with a custom script skill.
runtime_type: prompt
max_calls_per_turn: 5
timeout_seconds: 60
---

# 物业水电确认单处理 Skill

当用户上传物业水电确认单 Excel，并需要生成填好用量、单价、金额的 `.xlsx` 文件时，使用本 skill。

## 核心原则

- 只考虑 Agent 场景。
- 必须通过 `agent-database` 查询 Agent 绑定数据库中的水电话目表。
- 脚本不直接访问数据库、知识库或外部网络。
- 不要假设固定模板。必须先通过 `operation=inspect` 理解工作簿的实际单元格，再生成单元格编辑计划。
- 数值计算交给脚本的 `row_calculations` / `sum_updates` 完成；Agent 负责把 Excel 单元格映射清楚。
- 不要编造单价、价目记录、数据库 ID 或表 ID。

## 必需参数

调用最终写入脚本前，先收集这些字段：

- `confirmation_file_id`: 用户上传的 Excel 文件 ID。
- `confirmation_file_ids`: 多文件场景下的 Excel 文件 ID 数组。
- `payer_name`: 用水/电方名称、用电方名称或用水方名称。
- `billing_period`: 时间或计费周期；批处理时每个 job 可不同。
- `meter_reader`: 抄表人。
- `prepared_by`: 制表人。
- `price_records`: Agent 绑定数据库中查询到的水电话目记录。
- `sheet_name`: 可选。单 sheet 写入或每个批处理 job 中填写。

## 数据库查询流程

在调用最终写入前，必须先加载并使用 `agent-database`：

1. 调用 `list_accessible_databases`，查看当前 Agent 绑定的数据库。
2. 调用 `list_database_tables`，查找水电话目表、收费标准表或物业价格表。
3. 字段含义不清楚时，调用 `describe_database_table`。
4. 调用 `query_table_records` 拉取完整价目记录。
5. 将查询结果整理为 `price_records`。

`price_records` 结构：

```json
[
  {"name": "18号楼客梯", "utility_type": "electricity", "unit_price": 0.92},
  {"name": "18号楼水费", "utility_type": "water", "unit_price": 4.45}
]
```

`utility_type` 必须映射为 `electricity` 或 `water`，`unit_price` 必须是数字。

## 工作簿检查

不要直接调用默认填表模式。先让脚本检查工作簿：

```json
{
  "operation": "inspect",
  "confirmation_file_id": "...",
  "inspect_max_rows": 80,
  "inspect_max_cols": 30
}
```

检查结果会返回每个 sheet 的非空单元格、坐标、合并区域和文本。根据这些信息确定：

- 要处理的 `sheet_name`。
- 用水/电方和时间要写入哪个单元格。
- 抄表人和制表人要写入哪个单元格。若标签在合并单元格中，优先使用 `right_of_cell` 写入标签右侧值区域，而不是覆盖标签本身。
- 每条计费行的期初读数、期末读数、倍率、用量、单价、金额所在单元格。
- 合计金额所在单元格。

## 单 Sheet 写入

只处理一个 sheet 时，调用 `operation=apply_edits`。脚本会修改该 sheet 并生成一个 `.xlsx` 文件。

```json
{
  "operation": "apply_edits",
  "confirmation_file_id": "...",
  "sheet_name": "室内水电费使用确认单",
  "payer_name": "吾道文化创意有限公司",
  "billing_period": "2025-8-1 至 2025-8-31",
  "meter_reader": "张三",
  "prepared_by": "李四",
  "cell_updates": [
    {"cell": "A3", "value": "用水/电方名称：吾道文化创意有限公司    时间：2025-8-1 至 2025-8-31"},
    {"right_of_cell": "A16", "value": "张三"},
    {"right_of_cell": "D16", "value": "李四"}
  ],
  "row_calculations": [
    {
      "row_index": 5,
      "base_cell": "D5",
      "reading_cell": "E5",
      "multiplier_cell": "F5",
      "usage_cell": "G5",
      "unit_price_cell": "H5",
      "amount_cell": "I5",
      "unit_price": 0.92
    }
  ],
  "sum_updates": [
    {"cell": "I14", "cells": ["I5", "I6", "I7", "I8", "I9"]}
  ]
}
```

## 多 Sheet / 多时间段写入同一个文件

当用户要求“一次处理两个 sheet/两个时间段/全部确认单”时，优先使用 `operation=batch_apply_edits`。

注意：`batch_apply_edits` 的含义是**在同一个原始 workbook 中修改多个 sheet，最终只生成一个 `.xlsx` 文件**。不要把每个 sheet 拆成单独文件，除非用户明确要求分别导出。

批处理的顶层参数放公共值，每个 `jobs[]` 放该 sheet 自己的单元格和账期。脚本会在同一个 workbook 上依次应用所有 job，最后保存为一个文件。

```json
{
  "operation": "batch_apply_edits",
  "confirmation_file_id": "...",
  "payer_name": "吾道文化创意有限公司",
  "meter_reader": "张三",
  "prepared_by": "李四",
  "output_filename": "吾道文化创意有限公司_水电费确认单_2024-11和2025-08.xlsx",
  "jobs": [
    {
      "sheet_name": "地下总电减充卡",
      "billing_period": "2024-11-1 至 2024-11-30",
      "cell_updates": [
        {"cell": "A3", "value": "用水/电方名称：吾道文化创意有限公司    时间：2024-11-1 至 2024-11-30"},
        {"right_of_cell": "A16", "value": "张三"},
        {"right_of_cell": "D16", "value": "李四"}
      ],
      "row_calculations": [],
      "sum_updates": []
    },
    {
      "sheet_name": "Sheet1",
      "billing_period": "2025-8-1 至 2025-8-31",
      "cell_updates": [],
      "row_calculations": [],
      "sum_updates": []
    }
  ]
}
```

批处理最多 10 个 job，但只生成 1 个文件。每个 job 必须包含该 sheet 自己的：

- `sheet_name`
- `billing_period`
- `cell_updates`
- `row_calculations`
- `sum_updates`

## 多文件与多 Sheet 混合处理

如果用户上传多个 Excel，按文件分组处理：

- 每个输入 Excel 对应一个 `file_job`。
- 每个 `file_job.jobs[]` 对应该文件内部要处理的 sheet/时间段。
- 每个输入 Excel 最终生成一个输出 `.xlsx` 文件。
- 不要按 sheet 拆文件，除非用户明确要求每个 sheet 单独导出。

多文件场景先用 `confirmation_file_ids` 传入所有文件 ID，再调用 `operation=inspect`。`inspect` 会返回 `workbooks[]`，每个元素对应一个输入文件。

最终调用：

```json
{
  "operation": "batch_files_apply_edits",
  "confirmation_file_ids": ["file-a", "file-b"],
  "payer_name": "吾道文化创意有限公司",
  "meter_reader": "张三",
  "prepared_by": "李四",
  "file_jobs": [
    {
      "confirmation_file_id": "file-a",
      "output_filename": "A文件_已处理.xlsx",
      "jobs": [
        {
          "sheet_name": "Sheet1",
          "billing_period": "2025-8-1 至 2025-8-31",
          "cell_updates": [],
          "row_calculations": [],
          "sum_updates": []
        }
      ]
    },
    {
      "confirmation_file_id": "file-b",
      "output_filename": "B文件_已处理.xlsx",
      "jobs": [
        {
          "sheet_name": "Sheet1",
          "billing_period": "2025-8-1 至 2025-8-31",
          "cell_updates": [],
          "row_calculations": [],
          "sum_updates": []
        },
        {
          "sheet_name": "Sheet2",
          "billing_period": "2025-9-1 至 2025-9-30",
          "cell_updates": [],
          "row_calculations": [],
          "sum_updates": []
        }
      ]
    }
  ]
}
```

决策规则：

- 一个 Excel，一个 sheet：`apply_edits`，生成 1 个文件。
- 一个 Excel，多个 sheet：`batch_apply_edits`，生成 1 个文件。
- 多个 Excel：`batch_files_apply_edits`，每个输入文件生成 1 个文件。
- 多个 Excel 且每个 Excel 有多个 sheet：每个 `file_job` 内部用 `jobs[]` 处理多个 sheet，最终仍然每个输入文件生成 1 个文件。

## 失败处理

- 如果 Agent 没有绑定数据库、数据库没有价目表、或查不到匹配单价，不要直接运行最终写入，先向用户说明缺少价目数据并追问。
- 如果 `inspect` 返回多个可能 sheet，而用户没有要求全部处理，先让用户确认 sheet。
- 如果无法确定具体单元格，不要猜；先说明需要用户确认模板位置。
- 如果脚本返回某行 `missing_number`，说明对应读数、倍率或单价缺失，提示用户补充或确认。

## 最终回复

成功后简洁说明：

- 已生成几个确认单文件。
- 每个文件中处理了哪些 sheet/时间段。
- 每个 sheet 计算了多少行。
- 是否有跳过项或需要复核项。
- 提醒用户从生成文件卡片下载 `.xlsx`。
