import type { MarketMessages } from './en-US';

const messages: MarketMessages = {
  plugins: {
    title: '插件市场',
    description: '发现、安装和管理系统提供的功能插件',
    searchPlaceholder: '搜索插件',
    noResults: '未找到插件',
    noResultsDescription: '没有找到与 "{keyword}" 相关的插件',
    noPluginsDescription: '暂无可用插件',
    clearSearch: '清除搜索',
    loading: '加载中...',
    scrollHint: '继续向下滚动加载更多',
    noMoreData: '没有更多插件了',
    official: '官方',
    installError: '插件标识未找到',
    installSuccess: '插件安装成功',
    installFailed: '插件安装失败',
    categories: {
      all: '全部',
      tool: '工具',
      extension: '扩展',
      integration: '集成',
    },
    modal: {
      by: 'by',
      description: '功能描述',
      close: '关闭',
      add: '添加',
      installing: '安装中...',
      uninstalling: '卸载中...',
      uninstall: '卸载',
      publishedAt: '发布时间',
      versions: '版本列表',
      published: '已发布',
      packageSize: '包大小',
      noVersions: '暂无版本信息',
      noPluginData: '暂无插件数据',
    },
    uninstallSuccess: '插件卸载成功',
    uninstallFailed: '插件卸载失败',
  },
};

export default messages;
