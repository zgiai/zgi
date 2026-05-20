# GitHub Actions Workflows

## CI/CD 完整流程（推荐）

**文件**：`ci-cd.yml`

### 功能概述

完整的 CI/CD 流程，包含：
- ✅ 代码质量检查（Linting）
- ✅ 单元测试
- ✅ 集成测试
- ✅ Docker 镜像构建和发布
- ✅ 自动创建 GitHub Release

### 触发方式

1. **Pull Request** - 代码审查阶段
   - 运行 Lint 和单元测试
   - 不构建镜像

2. **开发分支推送**（develop/main）
   ```bash
   git push origin develop
   # 构建测试镜像：dev-develop-<sha>
   
   git push origin main
   # 构建测试镜像：dev-<sha>, dev-latest
   ```

3. **版本标签推送**（正式发布）
   ```bash
   git tag -a v1.0.0 -m "Release version 1.0.0"
   git push origin v1.0.0
   # 构建正式镜像：v1.0.0, 1.0, 1, latest
   # 自动创建 GitHub Release
   ```

4. **手动触发**
   - 在 GitHub Actions 页面手动运行
   - 可选择部署环境

### 配置要求

在 GitHub Secrets 中配置：
- `DOCKER_HUB_USERNAME` - Docker Hub 用户名
- `DOCKER_HUB_TOKEN` - Docker Hub 访问令牌

### 相关文档

- [快速开始指南](../../docs/quick-start-release.md)
- [版本发布策略](../../docs/release-strategy.md)
- [测试策略](../../docs/testing-strategy.md)
- [Docker 构建配置](../../docs/docker-build-push-setup.md)

---

## 简化版 Docker 构建（已废弃）

**文件**：`docker-build-push.yml`

> ⚠️ **注意**：此 workflow 已废弃，请使用 `ci-cd.yml` 获取完整的 CI/CD 流程。

