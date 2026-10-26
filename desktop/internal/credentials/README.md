# 系统凭据适配器

本模块实现 M02 §2 / M08 §2 / PRD NFR-2 的主进程秘密存储边界；它是 Go 内部依赖，不能作为 `mygo.Bind` 的服务。平台登录、可信主体核验、Runtime 选择、普通状态文件与 IPC DTO 由调用方负责。

`New()` 仅构造 Store 并持有 `mygo.Secrets`，不访问系统仓库，也不初始化 native framework。真正的读取、写入、删除发生在 `Load`、`Save(SystemStore)`、`Delete`；MyGo 使用当前应用 identifier，开发包的 `.dev` 与发行包天然隔离。适配器不覆盖 service，不创建明文后备文件。

| Go API | 合同 |
| --- | --- |
| `Save(ctx, Profile, []Entry, SystemStore)` | 更新明确槽位，成功才返回 `persisted`；失败返回固定安全错误；每槽最大 2560 字节 |
| `Save(ctx, Profile, []Entry, SessionMemory)` | 用户明确选择仅本次会话；完全不访问 native 仓库，每槽最大 64 KiB；返回 `memoryOnly` |
| `Load(ctx, Profile)` | 延迟恢复精确 profile/scope；返回 Go 内部 `Bundle`；秘密的 JSON 字段全部排除 |
| `Status(ctx, Profile)` | 仅 `hasCredential`、存储状态、槽位与到期元信息；不返回秘密 |
| `ForgetSession(Profile)` | 清除该进程内的凭据副本，不访问 native 仓库 |
| `Delete(ctx, Profile)` | 删除精确 profile/scope 的全部支持槽位；不枚举其他账号、Runtime 或文档 |
| `NewWithBackend(SecretsBackend)` | 故障注入适配口；方法为 `Get(string)`、`Set(string, []byte)`、`Delete(string)` |

`Profile.ID` 由 Go 的稳定 credential profile registry 提供，最长 96 字节，只允许字母数字、`_`、`-`；`Profile.Scope` 是 Go 已核验的非秘密 namespace，最长 512 字节，另允许 `. / : @ +`，拒绝控制字符、query 和 `..`。系统键仅使用 ID/scope 的 SHA-256 摘要与固定槽名；renderer 不能传任意 secret key。普通 profile/Runtime 选择状态文件应只保存已核验 scope 与非秘密引用，不能序列化 `Entry.Value`。

支持 `cloudAK`、`cloudSK`、`securityToken`、`cloudRefresh`、`runtimeGateway`、`applicationJWT`、`taskJWT`、`channelManagement`、`channelManagementJWT`、`trustedOwner`。每个秘密在独立键保存；小型元信息仅含 profileId、scope、slot、expiresAt，profile/scope 只出现一次，最大合法 namespace 和全部槽位也能保持在单值 2560 字节以内。零长度 Value 表示清除槽位。

更新云 AK/SK 必须同时提供两槽；完整 STS 还包括 securityToken。替换云 AK/SK 而省略 securityToken 会清除旧 STS，防止将新 AK/SK 与旧 token 混用。单独更新非云槽位不广播其他槽位；调用方仍按平台、网关、应用、任务或渠道请求族选择材料。

持久写入先读取旧值，再建立 durable pending 标记，写各槽与元信息，最后移除标记。任何失败（包括 native 实际写入后才返回错误）都恢复旧值；回滚期间忽略已经取消的 epoch context，只修复仓库，不发布旧登录状态。回滚失败保留标记，跨重启 `Load` 返回 `credentialRecoveryRequired`，不会返回混合的 AK/SK。用户可重试精确 profile 删除后重新登录。应用内所有 Store 对同一 profile 的 native 事务串行执行；宿主负责应用的单实例约束。

调用方把 cloud/connection 当前 epoch context 传入仓库操作；native 仓库可能弹出系统授权或解锁提示，无法从 Go context 强制打断该 OS 调用。它返回后再次检查 context，取消后的材料不进入内存缓存；调用方还须在激活登录/Binding 前核验 epoch。登录过程中自行推进 epoch 时，持久化使用完成登录后的当前 context。

存储成功仅证明材料已保存。`Load` 不验证 issuer/audience、STS 到期、owner 或动作授权；恢复时必须再次核验到期和实际云主体，再核验同一 Runtime/业务身份。`trustedOwner` 只接收调用方已经验证的 owner 材料及来源，不通过 profile ID、昵称、云 IAM ID 或持久化行为创造可信业务身份。

显式 `SessionMemory` 不删除该 profile 曾经保存的系统凭据。调用方若把“取消记住”定义为清除历史保存，应另执行明确的 `Delete`；不能把 `memoryOnly` 宣称为历史 native 材料已经删除。失败不会自动改用 SessionMemory。

固定安全错误为 `credentialMissing`、`credentialInvalidInput`、`credentialTooLarge`、`credentialStoreUnavailable`、`credentialStoreFailed`、`credentialRecoveryRequired`、`credentialOperationCanceled`；不包装原 native error，不将原始错误或其 causes 送入日志/IPC。

验证：Go 1.27.3，`CGO_ENABLED=0 go test -v ./internal/credentials`、`CGO_ENABLED=0 go vet ./internal/credentials`。测试使用 fake backend，覆盖 lazy factory、跨重启、多槽隔离、字节上限、显式内存、原始错误含秘密、7 个 mutation-then-error 故障点、回滚失败、晚到 epoch、metadata/JSON、profile/scope 删除、注入、STS 和最大 namespace。实际 macOS Keychain、开发/发行 identifier 隔离及授权弹窗由宿主真实 GUI 验证单独记录；此测试套件不访问用户 Keychain。
