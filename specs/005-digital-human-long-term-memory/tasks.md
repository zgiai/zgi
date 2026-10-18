# 任务：数字人长期记忆按需召回

## 阶段 1：设计与契约

- [X] T001 完成 `spec.md`，明确只修改召回侧和内部知识库不可见边界
- [X] T002 完成 `research.md`，记录当前绑定暴露路径和现有检索链路
- [X] T003 完成 `data-model.md` 与 `contracts.md`

## 阶段 2：运行时召回基础能力

- [X] T004 [US1] 在 `api/internal/modules/digitalhuman/memory_runtime.go` 移除最近 50 条系统提示词注入
- [X] T005 [US1] 在 `api/internal/modules/digitalhuman/memory_runtime.go` 增加 `retrieve_digital_human_memory` 工具契约和受限参数解析
- [X] T006 [US1] 在 `api/internal/modules/digitalhuman/memory_runtime.go` 增加数字人专用检索接口契约，后端补齐数字人身份和授权 scope
- [X] T007 [US1] 将数字人专用检索入口接入现有知识库检索服务，复用持久化 retrieval config

## 阶段 3：内部知识库关联与投影

- [X] T008 [US2] 在数字人内部记忆配置中保存内部知识库 ID，不写入 `digital_human_resource_bindings`
- [X] T009 [US2] 在资源绑定和候选接口增加防回归过滤，确保内部知识库不会返回给前端
- [X] T010 [US3] 在召回边界实现 active memory 到知识库投影的幂等同步
- [X] T011 [US3] 处理 revision、status、scope 和当前 active 视图的陈旧投影清理

## 阶段 4：验证

- [X] T012 [US1] 增加工具输入、无结果和权限隔离测试
- [X] T013 [US3] 增加投影结果权威校验与稳定 ID 测试
- [X] T014 检查业务源码未新增中文硬编码，并运行 `quickstart.md` 中的 Go 测试
