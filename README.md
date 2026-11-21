# TG 打卡考勤

可直接编辑、构建的源码：Go 后端 + Vue 后台 + Telegram 机器人。包含多标签后台、子账号本人权限、`/bind 后台子账号`、新成员入群 ID 提示，以及打卡文案、班次、活动、罚款、审批和报表。

## 部署

| 部署位置 | 使用文件 | 指南 |
|---|---|---|
| Render 免费 Web Service + Aiven | Dockerfile、render.yaml | [Render 变量一键复制](#render-变量一键复制) |
| 自有服务器 + 本地 PostgreSQL | Dockerfile、compose.yaml、Caddyfile | [服务器部署与备份](docs/SERVER.md) |
| 自有服务器 + Aiven | Dockerfile、compose.aiven.yaml、Caddyfile | [外部数据库部署](docs/SERVER-AIVEN.md) |

- **服务器**：填写 `.env` 后运行 `docker compose up -d --build`，数据库自动创建，数据在 `data/postgres/`；备份运行 `sh backup.sh`。Render 不使用 compose，HTTPS 由平台提供。
- **上传 GitHub**：仓库根目录应直接看到 Dockerfile、render.yaml、compose.yaml、backend、frontend、docs。网页上传别漏掉隐藏文件 `.gitignore`、`.dockerignore`、`.env*.example`；不要上传 node_modules、dist、真实 `.env` 或 CA 证书。必须使用本包的 Dockerfile。
- **CA 证书（`aiven-ca.pem`）不进 GitHub**（`.gitignore` 已排除 `*.pem`），部署时单独放置：Render 在 Environment → Secret Files 添加文件名 `aiven-ca.pem`、内容为 Aiven 下载的完整 PEM，会挂载为 `/etc/secrets/aiven-ca.pem`，并设置 `PGSSLROOTCERT` 指向它；自有服务器 + Aiven 把证书放在服务器项目根目录并命名 `aiven-ca.pem`（见 [外部数据库部署](docs/SERVER-AIVEN.md)）；使用 `compose.yaml` 自带本地 PostgreSQL 则不需要 CA。缺少或放错证书时服务无法连接数据库，不要用关闭校验的方式绕过。
- **已部署的 Render 升级**：用本目录覆盖仓库根目录文件；旧的 `*.tar.gz` 可删除；保留 Render 上已有的环境变量、Secret File 和 Aiven 数据库；**不清库，不更换 `TELEGRAM_TOKEN_SECRET`**（迁移兼容初版、v2、v3）。若之前未启用入群通知，把机器人设为群管理员，并在「打卡规则 → 基础配置 → 界面与权限」重新注册 Webhook。

## 环境变量

仓库只放示例值，真实密码、数据库连接和 Bot Token 填到平台环境变量或服务器 `.env`，不要提交。

模板：

- 本地 [`.env.example`](.env.example)
- 服务器 [`.env.server.example`](.env.server.example)
- 服务器 + Aiven [`.env.aiven.example`](.env.aiven.example)
- Render 见 [render.yaml](render.yaml)

**变量配置说明：**

每个变量的配置项由变量名、变量值组成，格式为 `变量名=变量值`。Render 上推荐使用下方「全部变量一次复制」，一次粘贴全部变量；也可以按变量逐个复制。

类型说明：

- **必填**：必须自己设置。
- **可选**：有默认值，按需修改。
- **预设**：Dockerfile 已预设，一般不用动。
- **仅服务器**：只有自有服务器部署才用。

### Render 部署需要设置的变量

| # | 变量 | 值 | 类型 | 说明 |
|---|---|---|---|---|
| 1 | `ENV` | `production` | 预设 | 运行环境。Render 上必须是 `production`：登录 Cookie 只走 HTTPS，数据库连接强制校验证书（`sslmode=verify-full`）。Dockerfile 已预设该值，只有本地开发才改成 `development`。 |
| 2 | `TZ` | `Asia/Shanghai` | 预设 | 进程时区，决定业务日、班次、日重置和报表时间怎么算。团队不在东八区时改成所在地时区。 |
| 3 | `DATABASE_URL` | 见下方 | 必填 | Aiven 控制台复制的完整 Service URI。数据库使用专用空库（如 `tg_attendance`，或新服务里还没有业务表的 `defaultdb`）。密码含特殊字符时直接使用控制台提供的 URI，不要手工转义。生产连接强制 `verify-full`。 |
| 4 | `PGSSLROOTCERT` | `/etc/secrets/aiven-ca.pem` | 必填 | Aiven CA 证书的路径，对应下方 CA 证书中创建的 Secret File。证书缺失或路径不正确，服务无法连接数据库。 |
| 5 | `ADMIN_USERNAME` | `admin` | 可选 | 初始管理员账号，默认 `admin`，也可以自定义。只在首次创建管理员时使用。 |
| 6 | `ADMIN_PASSWORD` | `REPLACE_WITH_A_UNIQUE_PASSWORD` | 必填 | 初始管理员密码，至少 6 字节，建议使用更长的随机密码。每次启动都必须设置，但只用于创建初始管理员；已有账号请在后台修改密码，修改此变量不会重置已有账号密码。 |
| 7 | `TELEGRAM_TOKEN_SECRET` | 见下方 | 必填 | 加密保存 Bot Token 和 Webhook Secret 的固定密钥，至少 32 字符。部署后不要更换，否则数据库中已保存的 Token 无法解密。 |
| 8 | `DB_MAX_CONNECTIONS` | `5` | 可选 | 数据库连接池上限，可填 3–10，建议 5。Aiven 免费版最多 20 个连接且不带连接池，不要设置过大。 |
| 9 | `GOMEMLIMIT` | `256MiB` | 预设 | Go 内存软上限。免费实例内存有限，接近上限时 Go 会更积极回收内存，减少因内存超限被重启。Dockerfile 已预设。 |
| 10 | `GOGC` | `70` | 预设 | Go 垃圾回收触发比例（Go 默认 100）。调低更省内存，代价是多一点 CPU。Dockerfile 已预设。 |
| 11 | `DATABASE_QUOTA_BYTES` | `0` | 可选 | 数据库容量额度，单位字节。`0` 表示未知，此时无法判断是否超过 80%。Aiven 免费版 1 GB 可填 `1073741824`（1 GiB = 1073741824，5 GiB = 5368709120）。后台「设置 → 清理内存」保存过策略后，以后台值为准，修改这个变量无法覆盖后台已保存的值。 |
| 12 | `RESOURCE_DISK_PATH` | `/app` | 可选 | 容量监控所检查的路径，监控该路径所在的文件系统。Render 上使用 `/app`（容器工作目录）；它不代表 Aiven 数据库服务器的磁盘。 |
| 13 | `TELEGRAM_RESET_EXPORT_DIR` | `/tmp/tg-attendance-exports` | 可选 | 日重置导出的临时副本目录，仅用于发送。不要指向数据库目录或备份目录，也不要使用含符号链接的路径。 |
| 14 | `TELEGRAM_INBOX_CONCURRENCY` | `2` | 可选 | 同时处理的互不冲突的打卡请求数，默认 2，范围 1–4。实际并发还受 `DB_MAX_CONNECTIONS` 限制，不超过 `(DB_MAX_CONNECTIONS-1)/2`（连接数为 5 时最多 2 路）。出现异常时可先改为 `1` 回到串行处理。 |

### Render 变量一键复制

#### 全部变量一次复制（推荐）

复制下面整个框，一次粘贴全部变量（每行 `变量名=变量值`）：

```text
ENV=production
TZ=Asia/Shanghai
DATABASE_URL=postgresql://USER:PASSWORD@HOST:PORT/DATABASE?sslmode=verify-full
PGSSLROOTCERT=/etc/secrets/aiven-ca.pem
ADMIN_USERNAME=admin
ADMIN_PASSWORD=REPLACE_WITH_A_UNIQUE_PASSWORD
TELEGRAM_TOKEN_SECRET=REPLACE_WITH_A_RANDOM_SECRET_AT_LEAST_32_CHARACTERS
DB_MAX_CONNECTIONS=5
GOMEMLIMIT=256MiB
GOGC=70
DATABASE_QUOTA_BYTES=0
RESOURCE_DISK_PATH=/app
TELEGRAM_RESET_EXPORT_DIR=/tmp/tg-attendance-exports
TELEGRAM_INBOX_CONCURRENCY=2
```

使用方法：

1. 打开 Render → 你的服务 → **Environment**，点击 **Add from .env**。
2. 粘贴上面的内容，**先把下面 3 项换成真实值再保存**，否则占位符会被当成真实配置，服务连不上数据库：
   - `DATABASE_URL`：Aiven 控制台复制的完整 Service URI，值中不要加空格。
   - `ADMIN_PASSWORD`：你自己的管理员密码。
   - `TELEGRAM_TOKEN_SECRET`：随机密钥，至少 32 字符，生成命令 `openssl rand -hex 32`。部署后不要更换。
3. 另外在 **Secret Files** 创建 `aiven-ca.pem`，见下方「CA 证书」，否则 `PGSSLROOTCERT` 指向的证书不存在。
4. 保存并部署。

说明：

- 值里没有空格时不需要加引号；Render 的 `.env` 语法规定，值含空格才需要加引号。
- `DATABASE_QUOTA_BYTES=0` 表示容量未知；Aiven 免费版 1 GiB 可改成 `DATABASE_QUOTA_BYTES=1073741824`。后台已保存清理策略时，以后台值为准。
- `ENV`、`TZ`、`GOMEMLIMIT`、`GOGC` 已由 Dockerfile 预设，写在这里只是显式列出，一般不需要修改。
- 以后换域名才需要另加 `PUBLIC_BASE_URL`，见下方「其他变量」。

#### 按变量逐个复制

以下每个配置项均采用 `变量名=变量值` 格式，点击复制即可一次性复制完整内容。

**1. ENV · 预设**

```text
ENV=production
```

**2. TZ · 预设**

```text
TZ=Asia/Shanghai
```

**3. DATABASE_URL · 必填**

```text
DATABASE_URL=postgresql://USER:PASSWORD@HOST:PORT/DATABASE?sslmode=verify-full
```

**4. PGSSLROOTCERT · 必填**

```text
PGSSLROOTCERT=/etc/secrets/aiven-ca.pem
```

**5. ADMIN_USERNAME · 可选**

```text
ADMIN_USERNAME=admin
```

**6. ADMIN_PASSWORD · 必填**

```text
ADMIN_PASSWORD=REPLACE_WITH_A_UNIQUE_PASSWORD
```

**7. TELEGRAM_TOKEN_SECRET · 必填**

```text
TELEGRAM_TOKEN_SECRET=REPLACE_WITH_A_RANDOM_SECRET_AT_LEAST_32_CHARACTERS
```

生成命令：

```sh
openssl rand -hex 32
```

**8. DB_MAX_CONNECTIONS · 可选**

```text
DB_MAX_CONNECTIONS=5
```

**9. GOMEMLIMIT · 预设**

```text
GOMEMLIMIT=256MiB
```

**10. GOGC · 预设**

```text
GOGC=70
```

**11. DATABASE_QUOTA_BYTES · 可选**

```text
DATABASE_QUOTA_BYTES=0
```

Aiven 免费版 1 GiB 可使用：

```text
DATABASE_QUOTA_BYTES=1073741824
```

**12. RESOURCE_DISK_PATH · 可选**

```text
RESOURCE_DISK_PATH=/app
```

**13. TELEGRAM_RESET_EXPORT_DIR · 可选**

```text
TELEGRAM_RESET_EXPORT_DIR=/tmp/tg-attendance-exports
```

**14. TELEGRAM_INBOX_CONCURRENCY · 可选**

```text
TELEGRAM_INBOX_CONCURRENCY=2
```

**注意：**

- `DATABASE_URL` 必须替换成 Aiven 控制台提供的真实 Service URI。
- `ADMIN_PASSWORD` 必须替换成自己的管理员密码。
- `TELEGRAM_TOKEN_SECRET` 必须使用随机生成的密钥，至少 32 字符。
- `DATABASE_QUOTA_BYTES` 如果使用后台已保存的清理策略，以后台配置为准。
- `ENV`、`TZ`、`GOMEMLIMIT`、`GOGC` 已由 Dockerfile 预设，一般不需要重复修改。

### CA 证书

数据库连接要校验 Aiven 的 CA 证书，证书**不进 GitHub**（`.gitignore` 已排除 `*.pem`），在 Render 单独添加：

1. 在 Aiven 控制台下载 **CA certificate**。
2. 打开 Render → Environment → **Secret Files**，新建一个文件。
3. Filename 填：

```text
aiven-ca.pem
```

4. Contents 粘贴证书的**完整 PEM 内容**（从 `-----BEGIN CERTIFICATE-----` 到 `-----END CERTIFICATE-----`）。
5. Render 会把它挂载到下面这个路径，正好对应 `PGSSLROOTCERT` 的值：

```text
/etc/secrets/aiven-ca.pem
```

CA 不是数据库密码，但用 Secret File 管理更不容易出错。缺少或放错证书时服务无法连接数据库，不要用关闭校验的方式绕过。自有服务器 + Aiven 的做法见 [外部数据库部署](docs/SERVER-AIVEN.md)。

### Render 自动注入的变量（不用设置）

- `PORT`：Render 注入，程序用它监听端口。
- `RENDER_EXTERNAL_URL`：Render 注入的服务地址，用来生成 Webhook 地址。以后用自定义域名时，再另加 `PUBLIC_BASE_URL`（值形如 `https://你的域名`，不要带路径）。

另外，`FRONTEND_DIST` 已在 Dockerfile 中预设，无需填写。修改任何环境变量后需重启或重新部署。

### 其他变量（按需）

#### PUBLIC_BASE_URL · 可选

说明：公网 HTTPS 地址，用来生成 Webhook 地址。**不要带路径**，也不要加 `/telegram/webhook`。Render 默认回退到自动注入的 `RENDER_EXTERNAL_URL`，使用自定义域名时再设置；本地运行需自己填写。

```text
PUBLIC_BASE_URL=https://YOUR-DOMAIN.example.com
```

#### TELEGRAM_BOT_TOKEN · 可选

说明：通常在后台「打卡规则 → 基础配置」保存 Token。设置此变量后，每次启动会用它更新后台里的 Token。属于秘密信息，不要提交到仓库。

```text
TELEGRAM_BOT_TOKEN=REPLACE_WITH_BOTFATHER_TOKEN
```

#### POSTGRES_PASSWORD · 仅服务器

说明：只在自有服务器使用本地数据库的 `compose.yaml` 时需要。建议使用随机十六进制，避免特殊字符破坏 URI。

```text
POSTGRES_PASSWORD=REPLACE_WITH_64_RANDOM_HEX_CHARACTERS
```

#### APP_DOMAIN · 仅服务器

说明：只在两种服务器 Compose 里需要。纯域名，不带 `https://` 或路径。

```text
APP_DOMAIN=attendance.example.com
```

### 容量监控与清理

- **后台已保存的清理策略优先于 `DATABASE_QUOTA_BYTES`**：修改环境变量无法覆盖已保存的额度，请到「设置 → 清理内存」修改。
- 默认自动清理，每分钟检查，严格超过 80% 才触发，两次执行至少间隔 30 分钟；已处理消息/通知保留 30 天，维护日志 180 天，已备份导出的临时副本 72 小时。开关和保留期都在后台设置，无需额外环境变量。
- 只清理允许清理的数据，不删除核心考勤、账号、有效备份或数据库底层文件。容量告警显示在清理页面和服务日志中，详见 [升级说明](docs/UPGRADE.md)。

## 排查打卡延迟

日志里每次打卡会输出各环节耗时（毫秒）：

- `[worker]`：每个机器人请求的 `auth_ms`（鉴权）和 `handler_ms`（处理）。
- `[punch]`：活动开始 / 结束的分段耗时，如 `identity`、`lock`、`state`、`bounds`、`count`、`insert`、`close_*`、`tail`；最大的一项就是瓶颈。
- `[tgbot] stage=`、`[inbox]`：机器人侧各环节、队列与处理总耗时。

设置行在进程内缓存 3 秒：后台保存立即生效，直接改数据库最长 3 秒后生效。

空闲时每 45 秒预热一次到 Telegram 的连接，避免两次打卡间隔较久后首条回复重新握手（约多 500ms）。同一时机还会让数据库连接池保持最多 3 条连接活跃（`warmDatabasePool`，每条只做一次 Ping，繁忙时 2 秒内放弃），避免空闲超过 5 分钟或被数据库断开后，下一次打卡并行读取时重新建连。

**延迟的主要来源是网络距离。** 日志里 `[punch]` 各分段每步约等于一次应用到数据库的往返；Render 与 Aiven 不在同一区域（例如新加坡与孟买）时每步约 80ms，打卡链路约 1 秒，其中数据库往返占一半左右。把数据库放到与 Render 相同的区域，是最有效的提速方式，不需要改代码。迁移时保持 `TELEGRAM_TOKEN_SECRET` 不变。

Render 免费版空闲 15 分钟会休眠，唤醒约需一分钟，进程睡着时内部保活不会运行。`render.yaml` 使用 `plan: free`、`region: singapore`，不需要升级付费实例。通知并发和立即唤醒优化在实例运行时生效，不能消除平台休眠带来的冷启动等待；需要及时响应时，用外部监控每 5–10 分钟访问一次 `/health`，进程内部保活在休眠后不会运行。

### 通知发送队列与并发

- 附加群/频道推送、审批提示、全员下班通知进入数据库队列（`standalone_notifications`），打卡处理不再等待这些通知的 Telegram 请求；用户本人的打卡回复仍直接发送。
- 通知最多同时发送 2 条，单目标请求最长 5 秒，最多重试 8 次；失败内容保留，可在后台概览的“通知发送队列”手动重试，失败原因（如 `chat_not_found`、`insufficient_permissions`）显示在“说明”列和日志 `[notifications] target_failed`。5 秒超时可能发生在 Telegram 已接收之后，因此仍有重复通知的边界。
- 频道/群全部失败后才发给管理员；每个成功目标会保存进度，重试时不会重复发送。
- 默认同时处理 2 个互不冲突的打卡请求（`TELEGRAM_INBOX_CONCURRENCY`，上限 4，实际还受 `DB_MAX_CONNECTIONS` 限制）；同一 Telegram 用户、员工、共享设备或设备关联员工的消息按顺序处理，绑定、审批、导出等非打卡更新作为全局顺序屏障。有限人数的活动按活动代码加事务锁后计数，防止超员。
- 有限人数活动的开始由数据库函数 `public.wf_start_activity_guarded` 一次完成（锁、计数、插入）；应用启动迁移会自动创建或更新它，数据库账号需要在 `public` 模式下创建函数的权限，启动日志出现迁移错误时先检查这一点。普通（未超时）回座把读取、关闭和当日统计合并为一条 SQL，超时回座仍走原来的罚款路径。
- 日志 `[inbox] queue_ms/processing_ms`、`[punch] op=checkin|checkout|activity_start|activity_end`、`[tgbot] stage=…` 用于区分排队、业务处理和数据库/网络耗时。
- 回退：没有破坏性的数据库迁移，新增的是 `standalone_notifications` 的几列和上述数据库函数（留着无害）。回退到旧镜像前，先在后台处理完通知发送队列。

## 源码组织

- `backend/services/telegram.go`：TG 业务、文案、活动、轮班、罚款与报表，以及打卡操作锁、分段耗时日志和活动提醒。
- `backend/services/attendance.go`：员工与考勤核心逻辑。
- `backend/services/deployment.go`：独立数据库、维护任务、事件时间和绑定。
- `backend/internal/standalone/module.go`：会话、子账号权限和持久消息队列。
- `backend/internal/tgbot/module.go`：TG 命令、菜单、消息和入群事件。
- `backend/handlers/attendance.go`：考勤 / TG 后台接口。
- `backend/services/compatibility.go`：旧业务仍引用的辅助类型和实现。没有路由、也不会被独立入口调用的遗留功能（站点统计、薪资、员工账号管理、LiveKit、Redis、MinIO、远程查看、邮件、告警监控、WebSocket 中心、客户端通知、客户端防护、诊断中心等）已清理，路由与现有功能不变。
- `frontend/`：可单独编辑的 Vue 页面、组件和样式。

Go 源文件按包合并，保留 `Source:` 标记和测试。合并减少的是文件数，**不等于代码量、内存或网络延迟同比减少**。真实的数据库初始化和迁移在 Go 源码内，没有独立的 SQL 迁移文件。后续拆分建议见 [ARCHITECTURE.md](docs/ARCHITECTURE.md)。

## 本地构建与测试

需要 Go 1.26、Node 22+。在 `frontend` 运行 `npm ci`、`npm run build`；在 `backend` 按 `.env.example` 创建 `.env` 后运行 `go run .`。

```sh
cd backend
go test . ./services ./internal/tgbot ./internal/standalone
```

数据库集成测试只能连接专用的临时测试库（会改动 TG 配置并插入虚拟员工），不能指向业务库：

```sh
STANDALONE_TEST_DATABASE_URL='postgresql://.../disposable_db?sslmode=disable' \
  go test . ./services ./internal/standalone -run 'TestStandalone|TestInbox|TestMemberNotice' -count=1
```

Render 免费版休眠会延迟响应和定时任务，常驻服务器可避免，但不保证网络故障时消息即时送达。镜像和服务器 HTTPS 需部署后验收。

## 遗留表清理（数据库版本 12）

本版本不再创建以下旧模块表，已有独立部署数据库会在下次启动时自动删除这些表及其中的数据：
`auth_sessions`、`cs_tool_settings`、`cs_tool_shift_roster`、`employee_payment_accounts`、
`wf_tg_employee_shift_state`、`wf_tg_reset_export_log`、`employee_accounts`、`sites`。
这里的 `employee_accounts` 是旧站点账号表，不是现用后台子账号表 `standalone_subaccounts`。

迁移使用事务，将删表和版本标记一并提交；不使用 `CASCADE`。如果存在未预期的外部依赖，迁移失败并回滚，服务启动会报错，需检查依赖后重试。升级前保留数据库备份，旧模块历史数据恢复需要使用备份。现用会话、账号、考勤、活动、罚款、绩效、报表及其归档表不在删除范围内。

迁移逻辑已合并到 `backend/services/deployment.go`，无需额外迁移文件。普通编译检查通过不代表已完成数据库迁移实测。
