const messages = {
  plugins: {
    title: 'System Plugins',
    description: 'View and use various functional plugins provided by the system',
    searchPlaceholder: 'Search plugins',
    noResults: 'No plugins found',
    noResultsDescription: 'No plugins found related to "{keyword}"',
    noPluginsDescription: 'No plugins available',
    clearSearch: 'Clear search',
    loading: 'Loading...',
    noMoreData: 'No more plugins',
    installError: 'Plugin identifier not found',
    installSuccess: 'Plugin installed successfully',
    installFailed: 'Failed to install plugin',
    categories: {
      all: 'All',
      tool: 'Tool',
      extension: 'Extension',
      integration: 'Integration',
    },
    modal: {
      by: 'by',
      description: 'Description',
      close: 'Close',
      add: 'Add',
      installing: 'Installing...',
      uninstalling: 'Uninstalling...',
      uninstall: 'Uninstall',
      publishedAt: 'Published At',
      versions: 'Versions',
      published: 'Published',
      packageSize: 'Package Size',
      noVersions: 'No versions available',
      noPluginData: 'No plugin data available',
    },
    uninstallSuccess: 'Plugin uninstalled successfully',
    uninstallFailed: 'Failed to uninstall plugin',
  },
};

export default messages;
export type MarketMessages = typeof messages;
