# ZGI

ZGI 是开源 ZGI 产品栈的顶层产品仓库。

当前阶段，这个仓库主要承担产品壳层职责：

- `api/` 是后端代码仓库，以 git submodule 方式接入
- `web/` 是前端代码仓库，以 git submodule 方式接入
- `sandbox/` 是代码执行沙箱，以 git submodule 方式接入
- `plugin-runner/` 是插件执行服务，以 git submodule 方式接入
- `docker/` 提供共享本地中间件和后续自托管资产
- `dev/` 提供本地开发辅助脚本
- `docs/` 现在用于存放 README 的多语言版本

## 仓库策略

这个仓库不会重写现有后端和前端仓库的历史，而是通过 submodule 进行产品级聚合。

当前接入的仓库：

- `api/` -> `git@github.com:zgiai/zgi-api.git`
- `web/` -> `git@github.com:zgiai/zgi-web.git`
- `sandbox/` -> `git@github.com:zgiai/zgi-sandbox.git`
- `plugin-runner/` -> `git@github.com:zgiai/zgi-plugin-runner.git`

这样可以在保留原仓库独立性的同时，为开源产品提供统一的顶层入口。

## 目录结构

```text
.
├── api/                  后端 submodule
├── web/                  前端 submodule
├── sandbox/              沙箱 submodule
├── plugin-runner/        插件执行器 submodule
├── docker/               共享中间件和部署资产
├── dev/                  本地开发入口
├── docs/                 README 多语言版本
├── scripts/              发布和维护脚本
├── .github/              模板和 CI 规划
├── CONTRIBUTING.md
├── Makefile
└── README.md
```

## 快速开始

### macOS / Linux

#### 完整 Docker 栈

这是推荐的本地启动方式：

```bash
make dev-docker
```

`make dev-docker` 首次运行时会自动完成：

- 初始化 submodule
- 复制缺失的 env 模板
- 重新生成 root compose
- 启动完整 Docker 栈

中国大陆网络环境下，如果镜像构建较慢或不稳定，可以使用：

```bash
./dev/start-docker --china
```

这个模式会为当前构建注入推荐镜像源配置，而不会改写模板文件。

如果你想检查本地 env 与模板的差异：

```bash
make env-check
```

如果模板新增了字段，且你只想追加缺失项而不覆盖现有值：

```bash
make env-sync
```

#### 源码开发（仅 macOS / Linux）

如果你希望 `api/` 和 `web/` 直接用源码运行，而共享中间件继续通过 Docker 提供，可以使用：

```bash
make setup
make dev-docker
make dev-api
make dev-web
```

### Windows

Windows 最低支持 Docker Desktop + PowerShell。源码开发辅助工具（如 `dev/check-env`、`dev/start-api`、`dev/start-web`）依赖 Unix-like shell，在 Windows 上不可用。

```powershell
# PowerShell
.\dev\start-docker.ps1

# CMD
.\dev\start-docker.cmd

# PowerShell（国内镜像）
.\dev\start-docker.ps1 -china

# CMD（国内镜像）
.\dev\start-docker.cmd -china
```

默认访问地址：

- Web: `http://localhost:13000`
- API: `http://localhost:2678`
- PostgreSQL: `localhost:${HOST_POSTGRES_PORT:-15432}`
- Redis: `localhost:${HOST_REDIS_PORT:-16379}`
- Weaviate: `http://localhost:${HOST_WEAVIATE_PORT:-18080}`
- Neo4j HTTP: `http://localhost:${HOST_NEO4J_HTTP_PORT:-17474}`
- Neo4j Bolt: `bolt://localhost:${HOST_NEO4J_BOLT_PORT:-17687}`
- Sandbox: `http://localhost:${HOST_SANDBOX_PORT:-18194}`
- Plugin Runner: `http://localhost:${HOST_PLUGIN_RUNNER_PORT:-15000}`

## 当前范围

当前仓库聚焦在产品级组织与交付：

- `api/`、`web/`、`sandbox/`、`plugin-runner/` 保持各自独立演进
- `zgi-console-api` 暂未纳入当前仓库
- root 仓库主要负责 Docker、脚本、文档、多仓聚合和后续发布协同

## README 多语言说明

- 根 `README.md` 是英文主版
- 当前中文版本位于 `docs/zh-CN/README.md`
- 后续新增语言时，统一使用 `docs/<locale>/README.md`
- 修改英文 README 时，需要同步更新所有已存在的语言版本
