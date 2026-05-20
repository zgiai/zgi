#!/bin/bash

# GitHub Actions 同步到 Gitee 快速配置脚本
# 使用方法: ./setup-gitee-sync.sh

set -e

echo "🚀 GitHub Actions 同步到 Gitee 配置向导"
echo "======================================"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查必要工具
check_requirements() {
    echo "📋 检查系统要求..."

    if ! command -v ssh &> /dev/null; then
        echo -e "${RED}❌ SSH 未安装，请先安装 SSH${NC}"
        exit 1
    fi

    if ! command -v git &> /dev/null; then
        echo -e "${RED}❌ Git 未安装，请先安装 Git${NC}"
        exit 1
    fi

    echo -e "${GREEN}✅ 系统要求检查通过${NC}"
}

# 生成 SSH 密钥
generate_ssh_key() {
    echo ""
    echo "🔑 生成 SSH 密钥..."

    KEY_NAME="$HOME/.ssh/gitee_sync_key"

    if [ -f "$KEY_NAME" ]; then
        echo -e "${YELLOW}⚠️  SSH 密钥已存在: $KEY_NAME${NC}"
        read -p "是否重新生成? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo "使用现有 SSH 密钥"
            return
        fi
    fi

    ssh-keygen -t rsa -b 4096 -C "github-actions@zgiai.com" -f "$KEY_NAME" -N ""

    echo -e "${GREEN}✅ SSH 密钥生成完成${NC}"
    echo "私钥: $KEY_NAME"
    echo "公钥: $KEY_NAME.pub"
}

# 显示配置步骤
show_config_steps() {
    echo ""
    echo "📝 配置步骤说明"
    echo "================"
    echo ""
    echo "1️⃣  添加公钥到 Gitee:"
    echo "   复制以下公钥内容，添加到 Gitee 仓库的 SSH 公钥设置中:"
    echo ""
    cat "$KEY_NAME.pub"
    echo ""
    echo "   📖 详细步骤:"
    echo "   - 登录 https://gitee.com"
    echo "   - 进入 zgiai/zgi-api 仓库"
    echo "   - 点击 '设置' → 'SSH公钥'"
    echo "   - 粘贴上面的公钥内容"
    echo ""

    echo "2️⃣  添加私钥到 GitHub Secrets:"
    echo "   复制以下私钥内容，添加到 GitHub 仓库的 Secrets 中:"
    echo ""
    echo "   Secret 名称: GITEE_SSH_PRIVATE_KEY"
    echo "   Secret 值:"
    echo "   ------------------------"
    cat "$KEY_NAME"
    echo "   ------------------------"
    echo ""
    echo "   📖 详细步骤:"
    echo "   - 进入 GitHub 仓库页面"
    echo "   - 点击 'Settings' → 'Secrets and variables' → 'Actions'"
    echo "   - 点击 'New repository secret'"
    echo "   - 名称填写: GITEE_SSH_PRIVATE_KEY"
    echo "   - 值粘贴上面的私钥内容"
    echo ""

    echo "3️⃣  推送工作流文件:"
    echo "   GitHub Actions 工作流文件已创建，需要推送到仓库:"
    echo ""
    echo "   git add .github/workflows/"
    echo "   git commit -m 'Add GitHub Actions sync to Gitee'"
    echo "   git push"
    echo ""
}

# 测试 SSH 连接
test_ssh_connection() {
    echo "🔍 测试 SSH 连接到 Gitee..."

    # 临时添加密钥到 ssh-agent
    eval "$(ssh-agent -s)"
    ssh-add "$KEY_NAME" 2>/dev/null || true

    # 测试连接
    if ssh -T git@gitee.com 2>&1 | grep -q "successfully"; then
        echo -e "${GREEN}✅ SSH 连接测试成功${NC}"
    else
        echo -e "${YELLOW}⚠️  SSH 连接测试失败，请检查公钥是否正确添加到 Gitee${NC}"
    fi

    # 清理
    ssh-agent -k 2>/dev/null || true
}

# 创建配置文件
create_config_files() {
    echo "📁 创建配置文件..."

    # 创建 docs 目录
    mkdir -p docs

    # 检查工作流文件是否存在
    if [ ! -f ".github/workflows/sync-to-gitee-enhanced.yml" ]; then
        echo -e "${RED}❌ 工作流文件不存在，请确保在正确的项目目录中${NC}"
        exit 1
    fi

    echo -e "${GREEN}✅ 配置文件检查完成${NC}"
}

# 主函数
main() {
    check_requirements
    generate_ssh_key
    create_config_files
    show_config_steps

    read -p "是否测试 SSH 连接? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        test_ssh_connection
    fi

    echo ""
    echo -e "${GREEN}🎉 配置完成！${NC}"
    echo ""
    echo "📋 下一步操作:"
    echo "1. 按照上面的步骤添加公钥到 Gitee"
    echo "2. 添加私钥到 GitHub Secrets"
    echo "3. 推送工作流文件到 GitHub"
    echo "4. 在 GitHub Actions 中监控同步状态"
    echo ""
    echo "📖 详细文档: docs/github-actions-sync.md"
    echo "💬 如有问题，请查看文档或提交 Issue"
}

# 执行主函数
main "$@"
