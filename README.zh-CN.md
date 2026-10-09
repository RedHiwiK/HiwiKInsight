# HiwiKInsight

[English](README.md) | 简体中文

面向独立 iOS / macOS 开发者的自托管产品分析与 App Store 收入服务，既给人看，也能让 AI Agent 直接查询。

HiwiKInsight 是一个 Go 单二进制，内嵌 SQLite（纯 Go 实现，无 cgo）。它接收 [HiwiKInsightKit](https://github.com/RedHiwiK/HiwiKInsightKit) Swift SDK 上报的匿名事件，实时验证 App Store Server Notifications V2（并把每笔购买关联到促成它的付费墙），同步 App Store Connect 销售与分析报表，并提供 Web 看板、邮件报告、告警，以及一套只读查询层，Claude、Cursor 或任何 MCP 客户端都能直接使用。用户数据不经过任何第三方分析服务。

## 60 秒体验

```bash
git clone https://github.com/RedHiwiK/HiwiKInsight.git
cd HiwiKInsight
docker compose -f docker-compose.demo.yml up --build
```

打开 http://localhost:8080/dashboard/ ，用 `demo` / `demo` 登录。Demo 内置三个示例 App 和 90 天生成数据。查询 API 接受 token `demo-token`：

```bash
curl -s -H "Authorization: Bearer demo-token" http://localhost:8080/v1/query/describe | head -40
curl -s -H "Authorization: Bearer demo-token" "http://localhost:8080/v1/query/metric/retention?app=pawprint"
```

不用 Docker（需要 Go 1.26+、Node 22、pnpm 10）：`make demo`。

## 功能

- **匿名事件分析**：SDK 上报安装、会话、页面、启动来源、购买和错误事件；每次安装一个随机 ID，不采集个人信息，不存 IP。
- **实时收入**：完整验证 JWS 签名的 App Store Server Notifications V2，每笔交易一封邮件，退款冲减净额，金额折算为基准币种。
- **购买归因**：SDK 把一个 UUID 作为 StoreKit 的 `appAccountToken` 传入；Apple 通知到达时，HiwiKInsight 就知道是哪个付费墙、哪个商品、哪个版本、安装第几天促成了这笔购买。
- **App Store Connect 同步**：销售报表（扣除佣金后的到手收入、下载、退款；最多 365 天日报和 36 个月月报的历史）与分析报表（按来源和国家的曝光、产品页浏览、首次下载）。
- **15 个口径固定的内置指标**：DAU/WAU/MAU 与粘性、活跃天数分布、真新用户留存、新增/回访/回流/流失、模块与页面使用、付费墙漏斗、收入归因、设备/系统/地区分布、会话、用户分层、事件趋势、错误、跨 App 经营总览、商店转化漏斗。见 [docs/metrics.md](docs/metrics.md)。
- **Web 看板**：13 个页面，中英文，浅色/深色，可切换展示币种，bcrypt 密码登录。
- **邮件报告与告警**：日报、周报；购买失败、付费相关错误、新错误或突增错误、App Store Connect 报表停更、App 停止上报等告警。
- **面向 AI 的查询层**：`describe` 语义层（表、视图、口径、事件字典、指标）；指标、SQL、单用户接口；`insight` CLI；内置 MCP 服务（`insight mcp`）；Claude Code Skill。全部只读。
- **运维简单**：一个静态二进制或一个容器，一个 YAML 配置，一个需要备份的 SQLite 文件。

## 架构

```
  iOS / macOS app                       Apple                       Apple
  (HiwiKInsightKit)           App Store Server Notif. V2     App Store Connect API
        |                               |                    (sales + analytics reports)
        | POST /v1/events               | POST /v1/appstore/notifications     ^
        v                               v                                     | pulled every 6 h
  +-----------------------------------------------------------------------------------+
  |  hiwikinsight (single Go binary)                                                  |
  |                                                                                   |
  |   ingest ---> write queue ---> SQLite (WAL, one writer) <--- App Store Connect    |
  |   notify (verify JWS, attribute, email) ---^        |              sync           |
  |                                                     | read-only connection pool   |
  |   daily/weekly reports + alerts (email) <-----------+                             |
  |   query API /v1/query/* <---------------------------+                             |
  |   dashboard /dashboard/ (embedded React app, reads the query API)                 |
  +-----------------------------------------------------------------------------------+
        ^                          ^                           ^
        | browser                  | insight CLI, curl         | insight mcp (stdio)
       you                      scripts, agents          Claude Code, Claude Desktop, Cursor
```

详见 [docs/architecture.md](docs/architecture.md)（英文）。

## 正式部署快速上手

需要一台有公网 HTTPS 域名的服务器（例如 `insight.example.com`），因为 SDK 和 Apple 都要能访问它。

### Docker Compose + Caddy（自动 HTTPS）

```bash
git clone https://github.com/RedHiwiK/HiwiKInsight.git && cd HiwiKInsight
mkdir -p config/catalogs
cp examples/config.example.yaml config/config.yaml    # 修改 apps、timezone、currency、users
cp deploy/.env.example .env                           # 填写 DOMAIN 和各项密钥
docker run --rm -it ghcr.io/redhiwik/hiwikinsight:latest hash-password
docker compose run --rm hiwikinsight check-config
docker compose up -d
```

容器内数据库放在 `/data` 卷中，所以 `config/config.yaml` 里不要设置 `data_dir`。

### 二进制 + systemd

从 [GitHub Releases](https://github.com/RedHiwiK/HiwiKInsight/releases) 下载对应平台的压缩包（内含 `hiwikinsight`、`insight` 和 `config.example.yaml`），按 [deploy/systemd/hiwikinsight.service](deploy/systemd/hiwikinsight.service) 里的注释安装，前面放 nginx（[deploy/nginx/hiwikinsight.conf](deploy/nginx/hiwikinsight.conf)）或 Caddy。

完整说明、备份与升级：[docs/deployment.md](docs/deployment.md)。十分钟上手：[docs/getting-started.md](docs/getting-started.md)。

## 接入 App

用 Swift Package Manager 添加 SDK（`https://github.com/RedHiwiK/HiwiKInsightKit`），然后：

```swift
import HiwiKInsightKit

// 启动时调用一次。appKey 必须出现在服务端配置的 apps[].key 中。
HiwiKInsight.start(.init(appKey: "pawprint", endpoint: URL(string: "https://insight.example.com")!)) {
    ["entry_count": "10-50", "is_pro": "true"]   // 可选的每日用户快照，只用分桶值
}

// 自定义事件
HiwiKInsight.signal("entry.created", ["entry_type": "note"])

// 页面（SwiftUI）：出现时 screen.viewed，消失时带停留时长的 screen.left
SettingsView().trackScreen("settings", module: "settings")

// 错误
HiwiKInsight.error(id: "sync.failed", category: "thrown-exception", message: "timeout")

// 购买归因：token 把 Apple 的通知关联到这个付费墙
let token = HiwiKInsight.beginPurchase(product: product.id, context: "paywall_onboarding")
let result = try await product.purchase(options: [.appAccountToken(token)])
```

然后在 App Store Connect 把生产和沙盒环境的 App Store 服务器通知（Version 2）地址都设为 `https://insight.example.com/v1/appstore/notifications`。详见 [docs/sdk-integration.md](docs/sdk-integration.md) 和 [docs/app-store-setup.md](docs/app-store-setup.md)。

## 用 AI 问数据

安装 CLI 并指向你的服务：

```bash
go install github.com/RedHiwiK/HiwiKInsight/cmd/insight@latest
export INSIGHT_ENDPOINT=https://insight.example.com
export INSIGHT_TOKEN=<query.tokens 中的一个>

insight describe                                  # 语义层，先读它
insight metric retention --app pawprint --from 2026-09-01 --to 2026-09-30
insight sql "SELECT name, COUNT(*) FROM events WHERE day >= date(local_today(), '-6 days') GROUP BY name"
```

在 Claude Code 中添加 MCP：

```bash
claude mcp add hiwikinsight -e INSIGHT_ENDPOINT=https://insight.example.com -e INSIGHT_TOKEN=<token> -- insight mcp
```

Claude Desktop（`claude_desktop_config.json`）、Cursor（`.cursor/mcp.json`）等客户端：

```json
{
  "mcpServers": {
    "hiwikinsight": {
      "command": "insight",
      "args": ["mcp"],
      "env": { "INSIGHT_ENDPOINT": "https://insight.example.com", "INSIGHT_TOKEN": "<token>" }
    }
  }
}
```

Claude Code Skill：`cp -r skills/hiwikinsight-analytics ~/.claude/skills/`，然后直接问“这个月的 7 日留存是多少？”。

更多：[docs/ai-integration.md](docs/ai-integration.md)、[docs/cli.md](docs/cli.md)、[docs/query-api.md](docs/query-api.md)。

## 配置

所有配置都在一个 YAML 文件里。从 [examples/config.example.yaml](examples/config.example.yaml) 开始；任何值都可以用 `${NAME}` 或 `${NAME:-default}` 引用环境变量。用 `hiwikinsight check-config -config config.yaml` 校验。全部字段见 [docs/configuration.md](docs/configuration.md)。

## 文档

`docs/` 下的文档只有英文版。

| 文档 | 内容 |
|---|---|
| [Getting started](docs/getting-started.md) | Demo、第一次正式部署、第一条事件 |
| [Configuration](docs/configuration.md) | 全部配置字段、默认值、环境变量 |
| [Deployment](docs/deployment.md) | Docker Compose、二进制 + systemd、反向代理、备份、升级、安全 |
| [SDK integration](docs/sdk-integration.md) | 事件、页面、错误、购买归因、环境识别 |
| [Event catalog](docs/event-catalog.md) | 为人和 AI 编写事件字典 |
| [App Store setup](docs/app-store-setup.md) | 服务器通知与 App Store Connect API |
| [Metrics](docs/metrics.md) | 每个内置指标的口径、参数和输出表 |
| [Query API](docs/query-api.md) | `/v1/query/*` 接口、鉴权与限制 |
| [CLI](docs/cli.md) | `insight` 命令 |
| [AI integration](docs/ai-integration.md) | CLI + Skill、MCP、HTTP、提问技巧 |
| [Dashboard](docs/dashboard.md) | 页面、登录、币种、语言 |
| [Reports and alerts](docs/reports-and-alerts.md) | 邮件时间、内容、告警规则 |
| [Architecture](docs/architecture.md) | 组件、数据流、表结构、设计取舍 |
| [Extending](docs/extending.md) | 新增指标、页面、告警、配置项 |
| [Privacy](docs/privacy.md) | 采集与不采集的内容、保留期 |

## 给 AI Agent

- 要**修改本仓库**：先读 [AGENTS.md](AGENTS.md)（仓库地图、构建与测试命令、约定、扩展步骤）。
- 要**使用某个 HiwiKInsight 服务的数据**：读 [docs/ai-integration.md](docs/ai-integration.md)。机器可读入口是 `insight describe`（CLI）、`GET /v1/query/describe`（HTTP，返回 Markdown）和 `insight mcp` 的 `describe` 工具。先调用其中之一，不要猜表名、列名或事件名。
- 查询层天然只读：只接受经过校验的单条 `SELECT`/`WITH`，并运行在只读 SQLite 连接上。

## 隐私

SDK 发送随机安装 ID、App/系统/设备上下文和你选择上报的事件。上报接口从不读取或存储客户端 IP。原始事件在 `retention_days`（默认 365 天）后删除。详见 [docs/privacy.md](docs/privacy.md)。

## 贡献与安全

见 [CONTRIBUTING.md](CONTRIBUTING.md)。安全问题请按 [SECURITY.md](SECURITY.md) 私下报告。版本记录见 [CHANGELOG.md](CHANGELOG.md)。

## 许可证

MIT，见 [LICENSE](LICENSE)。
