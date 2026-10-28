# OpenMPA TOS 公共下载与 macOS 一键安装设计

日期：2026-10-28  
状态：已批准并进入实现

## 目标

为 OpenMPA 提供不含账户标识的公共 TOS 下载域名，并提供可通过
`curl | bash` 使用的 macOS Apple Silicon 一键安装器。安装器需要解决
ad-hoc signed、未 notarize 开源 App 的 quarantine 首开阻断，但不得关闭
Gatekeeper、重新签名发布包或降低系统全局安全策略。

## 固定资源与边界

- Bucket：`openmpa-downloads`
- Region：`cn-beijing`
- 公共基址：`https://openmpa-downloads.tos-cn-beijing.volces.com`
- Bucket ACL 保持 `private`。
- Bucket Policy 仅向匿名主体开放对象级 `tos:GetObject` 和
  `tos:GetObjectVersion`，资源范围为 `trn:tos:::openmpa-downloads/*`。
- 不向匿名主体开放 `ListBucket`、上传、覆盖、ACL、策略或删除动作。
- 开启 Bucket 版本控制。
- TOS 是 GitHub Release 既有、已验证资产的镜像层，不重新构建版本制品。

对象布局：

```text
install.sh
releases/latest.json
releases/v0.1.3/OpenMPA-v0.1.3-macos-arm64.dmg
releases/v0.1.3/OpenMPA-v0.1.3-macos-arm64.zip
releases/v0.1.3/SHA256SUMS
releases/v0.1.3/build-verification.json
```

`releases/v*` 是不可变路径，发布时必须使用禁止覆盖语义；`install.sh` 与
`releases/latest.json` 是可变入口，仅在所有版本制品完成上传和匿名回读后更新。

## 方案选择

| 方案 | 结果 | 原因 |
| --- | --- | --- |
| DMG 校验、只读挂载、暂存安装、精确删除 quarantine | 采用 | 与现有发布契约一致，完全使用 macOS 原生工具并保留发布签名 |
| ZIP 解包安装 | 保留为未来降级路径 | 故障面较小，但当前公开入口已有可验证 DMG，无需增加分支 |
| 安装后执行 `codesign --force --deep --sign -` | 拒绝 | 改写发布证据，且 ad-hoc 重签不提供可信发布者身份 |
| 关闭 Gatekeeper 或全局清除 quarantine | 拒绝 | 扩大到整台机器，与 OpenMPA 安装目标无关且不可接受 |
| Developer ID + notarization | 长期方向 | 能提供正式发布者身份，但需要独立 Apple 凭证和发布流水线 |

## 安装器信任模型

当前 v0.1.3 安装器固定以下事实：

- 版本：`0.1.3`
- 平台：`darwin/arm64`
- Bundle ID：`com.superops.openmpa`
- DMG：`OpenMPA-v0.1.3-macos-arm64.dmg`
- DMG SHA-256：
  `c16fbb4e36eba977686c228f81e1f885e594c5da942e27580b163cfdd4b561b4`

固定 digest 能防止下载截断、镜像漂移或错误制品替换。它不能在远端安装器本身被
篡改时提供独立身份，因此 `curl | bash` 明确是便利入口；仓库源码和 GitHub commit
仍是安装器审计来源。后续若需要抵御 Bucket 写权限被盗，应增加独立 release 签名公钥，
而不是依赖同桶的裸 checksum 文件。

安装状态机：

```text
PREFLIGHT
  -> DOWNLOAD
  -> VERIFY_DMG_SHA256
  -> VERIFY_DMG_CONTAINER
  -> MOUNT_READ_ONLY
  -> VERIFY_SOURCE_APP
  -> STAGE_IN_INSTALL_VOLUME
  -> VERIFY_STAGED_APP
  -> REMOVE_STAGED_QUARANTINE
  -> VERIFY_STAGED_APP_AGAIN
  -> QUIESCE_EXISTING_APP
  -> BACKUP_EXISTING_APP
  -> ACTIVATE_STAGED_APP
  -> VERIFY_INSTALLED_APP
  -> COMMIT_AND_CLEANUP
```

每次 App 校验至少包含：

- `codesign --verify --deep --strict`
- Bundle ID 精确等于 `com.superops.openmpa`
- `CFBundleShortVersionString` 精确等于安装器固定版本
- 主可执行文件包含 `arm64`
- Bundle 根路径不是符号链接

quarantine 处理只能发生在完整校验后的暂存 OpenMPA.app：若
`com.apple.quarantine` 存在，则只对该暂存 bundle 执行
`xattr -dr com.apple.quarantine`，保留其他扩展属性，并再次校验代码签名。

## 安装与回滚

- 默认目标为 `/Applications/OpenMPA.app`，允许通过显式参数安装到一个绝对目录，
  以支持用户目录安装和隔离测试。
- 安装器本身不得以 root 运行；仅在默认目标不可写时对精确的文件操作调用 `sudo`。
- 所有下载、挂载和源 App 校验在停止旧 App 之前完成。
- 新 App 先复制到目标卷稳定隐藏暂存路径；已有目标先重命名为稳定备份路径，再激活暂存。
- 激活或最终校验失败时恢复备份；成功后删除临时备份。
- 安装目录内使用 macOS `lockf` 的 target-wide advisory lock，并在安装进程整个生命周期
  持有文件描述符；内核会在退出、SIGKILL 或断电后释放锁。稳定暂存/备份路径用于恢复
  中断事务，不实现自定义 PID 锁或陈旧锁删除协议。
- 这是事务式、可恢复的双重 rename，不宣称严格原子交换。
- 安装器不触碰 `Application Support`、数据库、凭据或 Runtime，不宣称用户数据回滚。

明确禁止：

- `spctl --global-disable` 或 `spctl --master-disable`
- `csrutil disable`、AMFI boot args 或系统 allowlist 绕过
- `xattr -cr /Applications` 或对 `/Applications`、`$HOME`、Downloads 的递归处理
- `codesign --remove-signature` 或本机 ad-hoc 重签发布 App
- `curl ... | sudo bash`
- 在 checksum、Bundle ID、版本、架构或签名校验失败后继续安装

## 发布顺序

1. 从 GitHub Release 下载 DMG、ZIP、`SHA256SUMS` 和
   `build-verification.json`。
2. 校验文件名、version/tag/commit/platform 和所有 digest。
3. 使用禁止覆盖语义上传 `releases/v0.1.3/*`。
4. 从匿名 HTTPS URL 回读每个对象，验证状态、长度与 SHA-256。
5. 上传并验证 `releases/latest.json`。
6. 最后上传并验证 `install.sh`。
7. 匿名列桶必须继续返回 403。

## 验收

- Bucket readback：ACL 私有、versioning Enabled、Policy 仅含两个 Get 动作。
- 匿名对象 HEAD/GET 成功；匿名 ListBucket、PUT 和 DELETE 失败。
- `bash -n` 和可用时的 `shellcheck` 通过。
- 安装器拒绝非 Darwin、非 arm64、错误 digest、错误 Bundle ID/版本和符号链接目标。
- 从真实公共 URL 在隔离安装目录完成一次首次安装和一次重复安装。
- 安装结果继续通过 `codesign --verify --deep --strict`，版本为 `0.1.3`。
- Gatekeeper 全局状态不被修改；只移除目标 App 的 quarantine。
- 仓库格式、提交秘密、token-limit 与既有相关门禁通过。

