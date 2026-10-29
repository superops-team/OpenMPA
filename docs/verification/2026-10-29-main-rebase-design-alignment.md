# 最新 main 同步与设计对齐

本次按用户要求执行 mstack 的仓库变更流程，将本地 main 从 `be633891194b59fca58a52d5d9549bac0b331336` rebase 到已拉取的 `origin/main`：`9fadf1453c5c273e28d457b1a24cea38143c09e6`。本地没有独有提交，因此 rebase 直接推进 main；随后恢复并合并未提交的设计与行为修复。

## 状态与保留范围

- 同步前保存148项工作区变更，包括未跟踪源码。正确stash为 `27478e3385349563720d1e5aae5fdd9b79737cb6`，保留作为恢复依据，没有创建提交或推送远端。
- Git冲突全部解决，`git ls-files -u`为空，main相对origin/main的ahead/behind为0/0；工作区继续保留未提交修改。
- 恢复过程中曾误用历史native-v1 stash，已完全撤回后应用正确stash。该历史迁移没有进入结果。
- 逐文件对比原stash和最终工作区：82个不与upstream重叠的tracked文件中，80个字节一致；其余两个是本次主动合并模型API的configuration.go及对应测试。验证清单在`.openmpa/rebase-design-20261029/unrelated-preservation.json`。

## 合并与设计决策

- 保留upstream的Studio资源、协作管理、渠道能力、作用域校验及受限X-MPA-Studio-Key转发。
- Resources和Collaboration隐藏时继续挂载，连接身份变化时隔离状态；保留upstream的editRevision/choiceCount草稿收据机制，避免延迟提交清空后续编辑。
- 保留本地助手创建的稳定UUID重试、每个助手独立AGENTS.md/Skills、模型更新不重建Chat、完整Reasoning、命令内容、SSE JSONL存档、终态与快照合并。
- 模型链路统一为缓存cloud eligibility目录和Session.Models契约：账号开通、API Key权限、具体版本的api.responses.support，再与Runtime广告列表相交。失败不返回未验证名单，排除的广告默认ID不重新插入列表。
- 支持API Key目录中的非主版本，按Name+Version合并metadata请求；拒绝不完整空页和重复页，前端连续挂载到12小时过期后刷新共享目录，过期/加载期间禁选。
- Agent Card的defaultModel来自广告列表首项，不等于会话Profile继承目标。UI以已读取的execution-config模型为准，缺失时显示Runtime默认策略，不用广告ID假装真实配置。已知且不合格的继承目标不能通过inherit再次提交；未知目标不做猜测。
- 合并两侧完整本地化：1040个匹配语言键，静态与参数化提示随语言切换，远端文字保持原样。
- 资源、协作、渠道页统一920px布局token、28px/6px普通按钮、36px行内表单动作、语义禁用/hover/success/progress色。320px长名称与多动作行可换行。
- 修正Studio验证脚本的旧成员select/task/mention流程，改为当前成员按钮、别名保存和草稿保留。设计矩阵检查可见main及中性鼠标状态，避免把正确hover主色误报为漂移。

## 验证结果

| 验证                                  | 结果                                                                                       |
| ------------------------------------- | ------------------------------------------------------------------------------------------ |
| 前端全量                              | 104文件，583测试通过                                                                       |
| TypeScript / ESLint / design:check    | 通过                                                                                       |
| Go internal及locales全量              | 通过                                                                                       |
| Go cloud/session/connection/usage竞态 | 通过                                                                                       |
| Go vet internal                       | 通过                                                                                       |
| 生产前端矩阵                          | 72组合，无溢出、脚本错误或缺失接口                                                         |
| Studio实际React交互                   | 11项流程通过，含冲突恢复、别名回显、草稿保持及三种宽度                                     |
| 新页面样式读回                        | 三页×浅深色×三宽度，18个合成布局通过                                                       |
| 真实Runtime模型配置                   | 普通助手模型PATCH及GET读回通过，临时助手删除成功                                           |
| macOS arm64产物                       | MyGo构建、签名校验通过                                                                     |
| 本机安装                              | 沿用此前直接替换授权，已覆盖/Applications/OpenMPA.app并启动；签名和构建哈希一致，版本0.1.4 |
| mstack安装投影validate                | 通过                                                                                       |

生产矩阵报告：`.openmpa/design-verification/2026-10-29T01-31-37.742Z/report.json`。
Studio报告：`.openmpa/studio-verification/2026-10-29T01-31-34.209Z/report.json`。
真实模型读回：`.openmpa/rebase-design-20261029/model-readback/model-switch.json`。
安装可执行文件SHA-256：`7cb98162cc04a45a7be15f3baa6db8ba2aed8f7c0418d652c196717c75f09d0a`。

## 执行方式与边界

Host绑定Trae，Forge绑定GitHub；所有代理继承父会话模型，没有模型覆盖。四个有界代理承担独立只读复核及不重叠文件合并，root统一处理模型链路、Git状态和最终验证。没有创建或合并PR，也没有云端部署。Studio完整云端资源写入和跨模型交叉审查未执行；当前UI流程使用模拟服务，真实云端检查限定于已授权的临时模型配置验收。

原有整体任务中的TLS日志接入仍缺少真实TopicId与地域；当前Runtime未配置TLS主题。本次rebase及设计对齐不声称该链路已经完成。
