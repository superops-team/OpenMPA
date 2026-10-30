# 2026-10-29 桌面交互与设计一致性修复

本轮完成助手编辑器、共享模型选择器、聊天输入框、trace 缓存与指标计算的修复。随后通过与 App 同一账号的现有 AK/SK 核验方舟模型协议，完成真实目录修复，并补齐完整 Reasoning 内容、运行中后台预取和 SSE 本地存档。真实 Runtime 的临时验证任务已完成并删除。TLS 日志来源仍待补充。

## 已交付行为

- 新建助手使用调用方 UUID；配置保存失败保留表单，重试继续同一个助手，避免创建重复项。
- 助手设置和聊天输入框共用模型筛选组件及按连接隔离的内存目录缓存。模型、Skills 目录加载不覆盖用户草稿。
- 模型配置更新不再重建整个 Chat；编辑后保留未发送草稿、历史和订阅。原生菜单操作绑定到被点击的助手，删除期间防止重复提交。
- 助手弹窗使用 640×640 的尺寸 token，内容内部滚动；切换标签、语言和元数据加载不改变弹窗位置。初始焦点不再短暂落到关闭按钮，背景区域 inert，键盘焦点可见。
- 输入框只保留输入表面和内缩 16px 的中性信息条；移除外层底色、圆角和阴影。搜索输入只在容器上绘制聚焦边框。中英文按钮、浅深色主题和 reduced motion 沿用共享设计系统。
- 执行中的 trace 缓存命中后继续后台拉取。显式刷新可以重新读取远端；新快照保留本地已保存的步骤，远端失败不删除本地副本。
- 本地缓存不因投影版本变更失效。清理按每个 turn 的文件更新时间进行；删除助手后，晚到的后台结果无法恢复已删除数据。
- worker 绑定严格匹配 invocation；未知时间保持未知。带 `coverage.workerTrajectory` 的 Runtime 响应直接使用上游轨迹，避免重复请求旧诊断接口。
- `tooling.refresh_check` 不计为工具调用；上游报告的零值不被事件数量覆盖；多条累计 usage 快照不再相加；v1.4 的 `aggregateComplete` 参与完整性判断。
- Reasoning 展示完整多行内容，不再只截首行；仍在流式更新的过程显示 Running。历史持久化暂时返回空列表时，保留已收到的 SSE 步骤。
- 任务接受后即可启动原生后台预取，每 5 秒补全，持续到终态；切换页面不停止。关闭 App、切换连接、删除助手或达到 2 小时上限会终止采集，连续读取失败最多重试 3 次。轨迹选择不再仅限最近 5 个 invocation。
- 模型资格来自 `ListModelChargeItems` 开通状态和精确版本的 `ListModelMetaDatas`：只接受 `Key=api.responses.support`、`Data=["true"]`。删除原先的描述文本推断、LLM/VLM 推断以及不存在的 `ApiSupport` 字段假设；原模型详情请求还错用了 `FoundationModelName`，实际 GetFoundationModel 要求 `Name`，当前链路已移除该不必要请求。
- 后端合并模型目录的并发请求、缓存空列表，并按账号 epoch 隔离。单个弹窗取消不会取消其他调用方共享的加载；metadata 查询并发上限为 4。
- 原生 SSE 在向 WebView 交付前，将工具、推理和终态事件追加至 session/invocation 的 `events.jsonl`。本地 trace 与远端步骤按明确事件或工具身份合并；call/result 去重，重放不重复累加，缺失时间不补造。投影最多显示 1000 步，原始已收到的执行事件仍保留在 JSONL 中。
- 快照的读、合并和写在同一个缓存锁内完成，防止后台预取和显式刷新覆盖各自证据。晚到 running 快照保留终态；删除后不允许重新写入。存档失败在 trace 页面明确显示，本地内容不伪装成完整远端轨迹。
- worker 的重复卡片概览改为摘要和筛选入口，完整步骤保留在时间线中，20 个真实 worker 步骤不会挤掉窄窗口的筛选栏和主列表。原生 milestone 显示为 Event 和时间线位置标记，不再把瞬时事件当作耗时 0ms 的操作。
- 本地命令标题显示实际安全命令摘要，推理详情直接展示完整记录文本。未发生文件变化的最终产物复查使用当前浏览器合成帧；DOM 每个窗口始终只有一个 trace 弹层和一个标题栏。

## 验证证据

| 检查                     | 结果                                                                                                            |
| ------------------------ | --------------------------------------------------------------------------------------------------------------- |
| 全量前端测试             | 93 文件 / 485 测试通过                                                                                          |
| TypeScript               | `npm run typecheck` 通过                                                                                        |
| 设计 token/schema/对比度 | `npm run design:check` 通过                                                                                     |
| Go internal/locales      | 通过                                                                                                            |
| trace/session 竞态检查   | `go test -race` 通过，包括后台抓取终态、JSONL重放、多part、在途删除和并发快照合并；cloud竞态检查此前通过        |
| 生产前端视觉矩阵         | 12 页面 × 2 主题 × 3 宽度，共 72 组合；无横向溢出、脚本错误、缺失接口                                           |
| 生产前端助手交互         | 新建失败重试、模型同步、草稿保留、单层边框、弹窗稳定尺寸、中英文切换通过                                        |
| 实际渲染文字对比度       | 554 处测量通过；检查可见文字和输入占位符，透明度/背景图无法可靠求值的项跳过，不等同于完整无障碍认证             |
| 方舟线上模型目录         | 与 App 同账号；34 个已开通基础模型，17 个主版本明确支持 Responses，实测约 1.2 秒                                |
| Runtime 线上只读核验     | 当前 OpenAPI 路径及 4 份历史 trace 读回；2 份 workerTrajectory=unavailable，2 份 not_applicable                 |
| Runtime 真实执行验收     | 临时助手执行 printf 验证任务，65条执行事件本地存档，2个工具步骤，远端20个worker步骤；合并31步。临时助手删除成功 |
| 真实轨迹生产渲染         | 上述31步数据，在浅/深色 × 1280/768/320px 共6组合中命令出现一次、推理可读、筛选栏及列表可达、无重复弹层          |
| macOS arm64 App          | MyGo 构建、签名校验通过；安装后哈希与构建产物一致                                                               |

视觉矩阵：`.openmpa/design-verification/2026-10-28T23-42-31.651Z/report.json`。
助手交互：`.openmpa/design-review-20261029/assistant-lifecycle/result.json`。
实际文字对比度：`.openmpa/contrast-review/report.json`。
模型与 Runtime 协议：`.openmpa/ark-protocol/`；真实 metadata 的最小脱敏样例保留在 `desktop/internal/cloud/testdata/ark-responses-metadata.json`。凭据未写入产物。
真实执行数据与渲染：`.openmpa/live-worker-archive/archived-trace.json`、`merged-trace.json`、`visual/result.json`。临时助手删除后，其按 session 保存的缓存目录也已经删除；验收产物保留明确的验证记录。

安装位置：`/Applications/OpenMPA.app`，版本仍为 `0.1.4`，没有发布新版本。
最新可执行文件 SHA-256：`984a2aa26112ddf0402b155956789c572a5808ea3b33453efb53be67fc9987fb`。

## 仍需完成

1. **worker 完整性边界。** 真实任务已验收，SSE已收到的执行事件会直接落盘，能补充远端很快消失的命令。App关闭或取消SSE订阅后，尚未到达客户端的事件仍只能依赖后台trace读取和上游持久化；目前远端提供的是best_effort、最近20条worker片段，不能声称客户端拥有全部worker原始事件。
2. **TLS 日志。** 真实 GetRuntime 返回 `TlsConfiguration:null`；Envs 中也没有 TLS/日志主题配置。当前存档的诊断 JSON 不等于云端 TLS 日志；实际 TLS 异步拉取尚未接入。已请求用户提供独立日志主题 TopicId 和地域（如果存在），没有擅自开通云端日志服务。

补充：所查旧 Python Runtime 源码中，`/runtime-console/sessions/{id}/diagnostics` 仅返回会话概况；`worker_live` 的最近 20 条元数据来自另一个 MCP 诊断服务。不能将这两个返回契约混用。当前应用缓存中实际存在的 v1.4 trace 应优先作为数据入口。

本轮全量前端第一次运行有一个 telemetry 探针因 5s 外层超时失败。该探针实际使用15s子进程限制，单独复跑通过；修正为20s测试等待时间以覆盖子进程启动后，全量485项通过，没有修改生产telemetry行为。
