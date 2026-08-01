const messages = {
  title: 'External Apps',
  description: 'Connect external providers, control their actions, and review audited activity.',
  disabledDescription:
    'External apps are disabled for this deployment. Enable the integration service and restart the API.',
  connectionCenter: {
    title: 'Connection Center',
    description:
      'Connect external apps and data sources, then clearly control who can use them and which actions are available.',
    searchPlaceholder: 'Search apps or connections',
    searchAria: 'Search external apps and connections',
    tabs: {
      available: 'Available',
      connected: 'Connected',
      availableCount: 'Available ({count})',
      connectedCount: 'Connected ({count})',
    },
    categories: {
      all: 'All',
    },
    quickConnect: {
      title: 'Quick connect',
      description: 'Choose an app and connect it with your own or organization credentials.',
      connect: 'Connect',
      addAnother: 'Add another connection',
      addShared: 'Add shared connection',
      manage: 'Manage',
      empty: 'No available apps match your search.',
    },
    connected: {
      title: 'Connected apps',
      description:
        'Review connection health, usage rules, and AIChat enablement for the current workspace.',
      connectionCount: '{count, plural, one {# connection} other {# connections}}',
      addAccount: 'Add another connection',
      addPersonalAccount: 'Add personal connection',
      addSharedAccount: 'Add shared connection',
      expandProvider: 'Expand {provider} connections',
      collapseProvider: 'Collapse {provider} connections',
      healthySummary: '{healthy}/{total} healthy',
      views: {
        label: 'Switch between connected accounts, app capabilities, and OAuth application setup',
        connections: 'Connected accounts',
        capabilities: 'App capabilities',
        oauth_config: 'OAuth app & secrets',
      },
      emptyTitle: 'No connected apps yet',
      emptyDescription:
        'After connecting an app, personal connections can be selected in AIChat; shared connections also require usage rules.',
      emptyAction: 'Browse available apps',
      journey: {
        state: {
          completed: 'Completed',
          current: 'Current step',
          pending: 'Pending',
        },
        connected: {
          title: 'Connected',
          description: 'Credentials verified',
          summary: 'Healthy connections: {healthy}/{total}',
        },
        authorized: {
          title: 'Usage rules',
          description: 'Who can use it and the available actions are configured',
          summary: 'Rules configured: {count}/{total}',
          loading: 'Loading usage rules',
        },
        ready: {
          title: 'Enable in AIChat',
          description: 'The current user selected it in this workspace',
          summary:
            '{count, plural, one {# connection is} other {# connections are}} enabled in this workspace',
          loading: 'Loading AIChat selection status',
          loadFailed: 'AIChat selection status could not be loaded',
        },
      },
      columns: {
        account: 'Connection',
        health: 'Connection status',
        usageRules: 'Usage rules',
        aiChat: 'AIChat',
        actions: 'Actions',
      },
      usageRules: {
        personal: 'Personal · Only you',
        personalDescription: 'Available only to your AIChat and not to agents.',
        configured: 'Usage rules configured',
        configuredSummary:
          '{rules, plural, one {# rule} other {# rules}} · {actions, plural, one {# available action} other {# available actions}}',
        configuredWithIssues: 'Some rules need attention',
        configuredWithIssuesSummary:
          '{rules, plural, one {# valid rule} other {# valid rules}} · {actions, plural, one {# available action} other {# available actions}} · {issues, plural, one {# invalid rule} other {# invalid rules}}',
        notConfigured: 'Usage rules not configured',
        notConfiguredDescription: 'Members and agents cannot use this shared connection yet.',
        availableToMe: 'Available in this workspace',
        managedByOrganization:
          'Usage targets and available actions are managed by an administrator.',
        currentUserAvailable: 'Available to you',
        unknown: 'Usage status unknown',
        loadFailed: 'Usage rules could not be loaded.',
        configure: 'Configure usage rules',
        manage: 'Manage usage rules',
        manageFor: 'Manage usage rules for “{name}”',
      },
      aiChat: {
        selected: 'Enabled in my AIChat',
        selectedDescription: 'Selected for your account in the current workspace.',
        selectedButUnavailable: 'Selected but currently unavailable',
        selectedButUnavailableDescription:
          'The connection is unhealthy or its usage rules changed. Repair it, then select it again.',
        unknown: 'AIChat status unknown',
        loadFailed: 'AIChat selection status could not be loaded for this workspace.',
        available: 'Available to enable',
        availableDescription: 'Select this account from External Apps in AIChat.',
        unavailable: 'Not available in AIChat',
        unavailableDescription: 'Repair the connection or configure its usage rules first.',
      },
      lastChecked: 'Last checked {date}',
      neverChecked: 'Not tested yet',
      openAIChat: 'Open AIChat',
      enableAIChat: 'Choose in AIChat',
      aiChatUnavailable: 'AIChat unavailable',
      aiChatUnavailableHint:
        'Make at least one healthy connection available to the current user first.',
      permissions: 'Usage rules',
      reconnect: 'Reconnect',
      securityNote:
        'Credentials stay encrypted. Connecting an organization account does not share it until an administrator configures usage targets and available actions.',
    },
    oauthRecovery: {
      title: 'Provider access cleanup needs attention',
      description:
        'ZGI could not confirm that one or more revoked OAuth credentials were removed at the provider. An administrator must verify each item.',
      unresolvedCount: '{count, plural, one {# unresolved item} other {# unresolved items}}',
      pendingCount: 'Pending: {count}',
      manualCount: 'Manual review: {count}',
      failedCount: 'Automatic failures: {count}',
      guidance:
        'Open the provider connected-app or security settings, remove this application access if it is still present, then record the outcome below. Acknowledging only closes the recovery item in ZGI; it does not revoke provider access.',
      manualReview: 'Manual review',
      operationDescription:
        'Automatic cleanup could not be confirmed for {provider}. No token or client secret is shown here.',
      failedAt: 'Last failed {date}',
      attempts: '{count, plural, one {# attempt} other {# attempts}}',
      accessRemoved: 'Provider access removed',
      tokenExpired: 'Token confirmed expired',
      confirmAccessRemovedTitle: 'Confirm access was removed from {provider}?',
      confirmAccessRemovedDescription:
        'Choose this only after verifying in {provider} that this application no longer has access. ZGI will record the resolution and close this recovery item.',
      confirmAccessRemoved: 'Confirm access removed',
      confirmTokenExpiredTitle: 'Confirm the {provider} token is expired?',
      confirmTokenExpiredDescription:
        'Choose this only after verifying that the provider token can no longer be used. ZGI will record the resolution and close this recovery item.',
      confirmTokenExpired: 'Confirm token expired',
      cancel: 'Cancel',
    },
    advanced: {
      label: 'Management center',
      policies: 'Action policies',
      executions: 'Execution logs',
      providerDetails: 'App details',
    },
  },
  capabilities: {
    view: 'View capabilities',
    viewFor: 'View the capabilities adapted for {provider}',
    title: '{provider} capabilities',
    description:
      'Review adapted features, connection methods, provider permissions, and supported surfaces.',
    connectedViewLabel: 'Connected app capabilities',
    connectedSummary:
      '{total} actions adapted · {available} available to current connections · {attention} need attention',
    catalogSummary: '{total} features adapted · {read} read · {write} write',
    catalogNotice:
      'This view only describes adapted features. Manage account health and provider access under Connections.',
    statusFilterLabel: 'Filter app capabilities by availability',
    statusFilters: {
      all: 'All',
      available: 'Available',
      needs_attention: 'Needs attention',
    },
    columns: {
      action: 'App capability',
      description: 'Description',
      effect: 'Effect',
      risk: 'Risk',
      availability: 'Connection availability',
      execution: 'Confirmation',
    },
    summary: '{count} actions · {access}',
    filterLabel: 'Filter app capabilities',
    filters: {
      all: 'All {count}',
      read: 'Read {count}',
      write: 'Write {count}',
    },
    access: {
      read: 'Read',
      write: 'Write',
      readOnly: 'Read only',
      readWrite: 'Read and write',
    },
    availability: {
      available: 'Available',
      needs_connection: 'Connection required',
      needs_scope: 'More access required',
      needs_permission: 'Usage permission required',
      disabled_by_policy: 'Disabled by usage rules',
      data_egress_blocked: 'Data sharing disabled',
      checking: 'Checking availability',
      status_unavailable: 'Live status unavailable',
    },
    availableConnections: '{count} compatible connections can run this action',
    remediation: {
      needs_connection: 'Connect an account that supports this action.',
      needs_scope: 'Reconnect the account and approve the additional provider permissions.',
      needs_permission:
        'This audience is not allowed to use the action. Ask an administrator to update its usage rules.',
      disabled_by_policy:
        'This action is disabled by the current usage rules. Ask an administrator to enable it.',
      data_egress_blocked:
        'This action sends data to the provider. Ask an administrator to allow data sharing.',
      checking: 'This action is not marked available until the check completes.',
      status_unavailable:
        'Current executability cannot be confirmed, so this action is not marked available.',
    },
    loadFailed: 'App capabilities could not be loaded. Try again shortly.',
    liveStatusUnavailable:
      'Live availability is temporarily unavailable. The adapted capabilities below do not imply that the current account can run them.',
    empty: 'No adapted actions match this filter.',
    requiredScopes: 'Required permissions',
    noAdditionalScopes: 'No additional permissions',
    risk: 'Risk: {risk}',
    approval: 'Confirmation: {approval}',
    approvalAlways: 'Confirm every time',
    approvalInherit: 'Use default setting',
    currentPolicy: 'Current usage rules',
    currentPolicySummary: '{approval} · {egress}',
    currentPolicyUnavailable: 'Current usage rules cannot be confirmed',
    authentication: 'Authentication and permissions',
    noAuthenticationMethods: 'No specific connection method declared',
    surfaces: 'Available surfaces',
    surface: {
      aichat: 'AIChat',
      agent: 'Agent',
      workflow: 'Workflow',
      api: 'API',
      other: 'Other caller',
    },
    noSupportedSurfaces: 'No supported surface declared',
    workflowUnavailable: 'Workflow is not yet supported',
    notice:
      'This view describes adapted features. Review individual connection health and provider permissions under Connections.',
    executionSettings: 'Execution settings',
    saveExecutionSettings: 'Save execution settings',
    actionStatus: 'Action status',
    actionStatusDescription: 'When disabled, no connection can run this action.',
    actionEnabled: 'Enabled',
    actionDisabled: 'Disabled',
    dataEgress: 'Data sharing',
    externalDestination: 'Execution sends data to {destination}',
    unknownDestination: 'the provider API',
    noExternalDestination: 'This feature does not send data to an external service',
    dataEgressAllowed: 'Allowed',
    dataEgressBlocked: 'Blocked',
    dataEgressNotRequired: 'No external data sharing',
    documentation: 'View developer docs',
    connect: 'Connect {provider}',
  },
  authMethodPicker: {
    title: 'Connect {provider}',
    description:
      'Choose how this provider should identify you. Each method keeps its own credential scope and available actions.',
    listLabel: 'Available authentication methods',
    recommended: 'Recommended',
    otherMethods: 'Other connection methods',
    openFor: 'Choose another way to connect {provider}',
    continue: 'Continue',
    cancel: 'Cancel',
    configureOAuth: 'Continue to configure the organization OAuth application.',
    adminSetupRequired: 'An administrator must configure this OAuth application first.',
    unavailable: 'This method is unavailable in the current deployment.',
    actionCount: '{count} actions supported',
    selected: 'Selected: {method}.',
    sharedOAuth: {
      title: 'These OAuth connections share one application setup',
      description:
        'The client ID and secret belong to the organization OAuth application and are configured once. Each connection method still has a different owner, usage scope, and action set.',
    },
    result: {
      personal: 'Result: a personal connection available only to you in AIChat.',
      organizationUser:
        'Result: an organization-shared account. Usage rules can grant it to members, workspaces, and agents.',
      organizationApplication:
        'Result: an organization-shared application identity limited to actions supported by this method.',
    },
    identity: {
      user: 'User identity',
      application: 'Application identity',
      channel: 'Channel identity',
      service: 'Service identity',
    },
    credentialSource: {
      platform: 'Platform managed',
      organization: 'Organization shared',
      account: 'Personal · Only you',
    },
  },
  oauth: {
    flow: {
      connectTitle: 'Connect {provider}',
      reconnectTitle: 'Reconnect {provider}',
      upgradeTitle: 'Update {provider} access',
      description:
        'Continue with {provider} to authorize “{connection}”. Credentials are exchanged and encrypted by the server.',
      continueToProvider: 'Continue to {provider}',
      status: {
        idle: 'Ready to connect',
        starting: 'Preparing a secure connection',
        waiting: 'Waiting for provider authorization',
        exchanging: 'Finishing the connection',
        succeeded: 'Connection verified',
        failed: 'Connection could not be completed',
        expired: 'Authorization expired',
        cancelled: 'Authorization cancelled',
        popup_blocked: 'The authorization window was blocked',
        popup_closed: 'The authorization window was closed',
        timed_out: 'Authorization timed out',
      },
      statusDescription: {
        idle: 'Start again when you are ready.',
        starting: 'Creating a short-lived authorization flow.',
        waiting: 'Complete the provider consent screen. This page updates automatically.',
        exchanging: 'The server is validating scopes and encrypting the new credential.',
        succeeded: 'The provider account is connected. Review the readiness steps below.',
        failed: 'No credential was exposed. You can safely try the authorization flow again.',
        expired: 'The short-lived flow expired without changing the saved connection.',
        cancelled: 'The flow ended without changing the saved connection.',
        popup_blocked: 'Open the provider in a new window or continue in this tab.',
        popup_closed:
          'Reopen the provider window, or check the status if consent already finished.',
        timed_out: 'The provider did not finish before the secure flow expired.',
      },
      popupBlockedHint:
        'Your browser blocked the provider window. Nothing was shared with the provider yet.',
      popupClosedHint:
        'The provider window closed before a final result was received. The server continues checking this short-lived flow.',
      scopeUpgradeUnavailable:
        'The missing provider permission is not mapped to a currently available action. Review the intended action and try again.',
      openPopup: 'Open authorization window',
      continueFullPage: 'Continue in this tab',
      cancel: 'Cancel',
      checkStatus: 'Check status',
      close: 'Close',
      tryAgain: 'Try again',
      done: 'Done',
      success: {
        verified: 'Verified',
        verifiedDescription: 'The provider identity and credential were validated.',
        rulesRequired: 'Usage rules required',
        rulesRequiredDescription:
          'An administrator must choose who can use this shared connection and which actions are allowed.',
        rulesReady: 'Usage scope ready',
        rulesReadyDescription: 'This personal connection remains available only to its owner.',
        aiChatAvailable: 'Available to AIChat',
        aiChatAvailableDescription:
          'The connection can now be selected from External Apps in AIChat.',
        aiChatPending: 'AIChat selection pending',
        aiChatPendingDescription:
          'Finish usage rules when required, then select the connection in AIChat.',
      },
    },
    result: {
      status: {
        pending: {
          title: 'Waiting for authorization',
          description: 'This page updates automatically when the provider finishes.',
        },
        authorizing: {
          title: 'Authorization in progress',
          description: 'Complete the provider consent screen to continue.',
        },
        exchanging: {
          title: 'Securing the connection',
          description: 'The server is validating and encrypting the provider credential.',
        },
        succeeded: {
          title: 'Connection complete',
          description: 'You can close this window and return to Connection Center.',
        },
        failed: {
          title: 'Connection failed',
          description: 'Return to Connection Center to review the safe error and try again.',
        },
        expired: {
          title: 'Authorization expired',
          description: 'Return to Connection Center and start a new authorization flow.',
        },
        cancelled: {
          title: 'Authorization cancelled',
          description: 'No saved connection was changed.',
        },
        invalid: {
          title: 'Invalid authorization result',
          description: 'This page does not contain a valid short-lived flow reference.',
        },
        unreachable: {
          title: 'Checking the connection',
          description: 'The server is temporarily unreachable. This page will keep trying.',
        },
      },
      retry: 'Check again',
      returnToConnections: 'Return to Connection Center',
      noScript:
        'JavaScript is unavailable. Return to Connection Center to check whether the connection completed.',
    },
    clientConfig: {
      connectedViewLabel: 'OAuth application and secret management',
      connectedViewTitle: 'OAuth application and secret management',
      connectedViewDescription:
        'Manage the client ID, client secret, and authorization callback this organization uses to connect the app.',
      writeOnlyBadge: 'Write-only secret storage',
      connectedViewSecurityNote:
        'Client secrets stay encrypted and are never returned to the browser. Before rotation or removal, ZGI checks connected accounts and pending authorizations.',
      sourceLabel: 'Configuration source',
      source: {
        organization: 'Organization-owned configuration',
        deployment: 'Deployment-managed configuration',
        none: 'Not configured',
      },
      updatedLabel: 'Last updated',
      neverUpdated: 'Not saved yet',
      title: 'Configure {provider} OAuth application',
      description:
        'Use an OAuth application owned by your organization. Provider secrets stay encrypted and are never returned to the browser.',
      sharedTitle: 'This configures an OAuth application, not an account connection',
      sharedDescription:
        'One {provider} OAuth application can support the connection methods below. After saving, you will continue to the provider account you selected.',
      continueAfterSave: 'Continue after saving: {method}',
      adminOnlyNotice:
        'Only organization owners and administrators can view this setup or rotate its secret. Members only see whether connecting is available.',
      callbackURL: 'Authorized callback URL',
      callbackURLDescription:
        'Add this exact URL to the provider OAuth application before saving its client credentials.',
      copyCallbackURL: 'Copy callback URL',
      callbackCopied: 'Callback URL copied',
      copyFailed: 'Copy failed. Copy the callback URL manually.',
      openProviderConsole: 'Open {provider} developer console',
      setupGuide: {
        title: 'Get {provider} connection credentials',
        description:
          'Follow these {count} steps to obtain credentials safely, then enter them below.',
        toggle: 'Credential guide',
        openConsole: 'Open {provider} developer console',
        openDocumentation: 'Open official setup documentation',
      },
      currentStatus: 'Current status:',
      configured: 'Configured',
      notConfigured: 'Not configured',
      clientID: 'Client ID',
      clientIDDescription: 'The public identifier of your organization-owned OAuth application.',
      clientIDPlaceholder: 'Paste OAuth client ID',
      clientSecret: 'Client secret',
      clientSecretDescription:
        'Encrypted on save and never returned. Leave blank to keep the existing secret.',
      clientSecretPlaceholder: 'Paste OAuth client secret',
      keepExistingSecret: 'Leave blank to keep the encrypted secret',
      secretStored: 'Securely stored',
      secretNotRequired: 'No secret is required for this connection method',
      rotateSecret: 'Rotate secret',
      changeClientID: 'Change Client ID',
      editAdditional: 'Edit app settings',
      editDescription:
        'Existing values are never returned to the browser. Enter only what you want to replace.',
      backToOverview: 'Back to overview',
      cancelEdit: 'Cancel editing',
      close: 'Close',
      saveChanges: 'Save changes',
      valueUnavailable: 'Securely stored',
      supportedMethods: 'Supported connection methods',
      sharedByMethods: 'Shared by {count} connection methods',
      connectedAccounts: '{count} connected accounts',
      updatedAt: 'Last updated: {time}',
      writeOnlyNotice: 'Credentials are write-only and are never returned to the browser.',
      dangerTitle: 'Danger zone',
      impactLoading: 'Checking connected accounts and pending authorizations.',
      impactSummary: '{accounts} connected accounts and {flows} pending authorizations.',
      impactUnavailable: 'Removal impact is temporarily unavailable. Try again later.',
      removeBlocked: 'Remove connected accounts and finish pending authorizations first.',
      required: 'Complete the required OAuth application fields.',
      loadFailed: 'OAuth application configuration could not be loaded.',
      continueRefreshFailed:
        'The OAuth application was saved, but connection readiness could not be refreshed. Reload and try connecting again.',
      retry: 'Reload',
      configureAction: 'Configure OAuth application',
      manageAction: 'OAuth application settings',
      manageFor: 'Manage the OAuth application for {provider}',
      adminSetupRequired: 'Administrator setup required',
      save: 'Save application',
      saveAndContinue: 'Save and continue',
      cancel: 'Cancel',
      remove: 'Remove configuration',
      removeTitle: 'Remove this OAuth application configuration?',
      removeDescription:
        'New {provider} connections will be blocked. Existing connections may continue until their tokens need refresh or authorization.',
    },
  },
  common: {
    unknownExternalApp: 'Unknown external app',
    unknownDriver: 'Unknown driver',
    unnamedConnection: 'Unnamed connection',
    unknownAction: 'Unknown action',
  },
  metadata: {
    providers: {
      github: {
        name: 'GitHub',
        description: 'Read repositories and issues through the GitHub REST API.',
        healthProbe: 'Reads the authenticated GitHub user without consuming a paid operation.',
      },
      gmail: {
        name: 'Gmail',
        description: 'Read account identity and send email through the Gmail API.',
        healthProbe: 'Reads the authenticated Google account identity without sending email.',
      },
      feishu: {
        name: 'Feishu',
        description: 'Access Feishu identity, cloud documents, files, and messaging.',
        healthProbe: 'Reads the authenticated Feishu identity without sending a message.',
      },
      x: {
        name: 'X',
        description: 'Read X account data and publish posts with explicit approval.',
        healthProbe: 'Reads the authenticated X account identity without publishing a post.',
      },
      webSearch: {
        name: 'Web Search',
        description: 'Search and read public webpages with Exa.',
        healthProbe: 'Runs a minimal Exa search to verify authentication and availability.',
      },
    },
    actions: {
      connectionTest: {
        name: 'Test connection',
        description: 'Verifies the saved connection credentials and service availability.',
      },
      githubUserGet: {
        name: 'Get authenticated GitHub user',
        description: 'Returns a bounded public profile for the GitHub account on this connection.',
      },
      githubRepositoryList: {
        name: 'List GitHub repositories',
        description: 'Lists repositories available to the authenticated GitHub user.',
      },
      githubRepositorySearch: {
        name: 'Search GitHub repositories',
        description: 'Searches GitHub repositories and returns bounded repository metadata.',
      },
      githubIssueList: {
        name: 'List GitHub repository issues',
        description:
          'Lists issue metadata for one repository. GitHub may also return pull requests from this endpoint.',
      },
      githubIssueGet: {
        name: 'Get GitHub issue',
        description: 'Reads one GitHub issue or pull request with a bounded body and metadata.',
      },
      githubIssueCommentList: {
        name: 'List GitHub issue comments',
        description: 'Lists a bounded page of comments for one GitHub issue or pull request.',
      },
      githubIssueCreate: {
        name: 'Create GitHub issue',
        description:
          'Creates an issue in one repository. This write is disabled by default and requires approval when enabled.',
      },
      githubIssueCommentCreate: {
        name: 'Create GitHub issue comment',
        description:
          'Sends a comment to an issue or pull request. This write is disabled by default and requires approval when enabled.',
      },
      gmailAccountGet: {
        name: 'Get Gmail account',
        description: 'Returns limited identity data for the connected Google account.',
      },
      gmailMailSend: {
        name: 'Send Gmail message',
        description: 'Sends an email from the connected Gmail account after explicit approval.',
      },
      gmailMailSearch: {
        name: 'Search Gmail messages',
        description: 'Searches the connected mailbox and returns bounded message summaries.',
      },
      gmailMailGet: {
        name: 'Read Gmail message',
        description: 'Safely decodes one Gmail message and returns a bounded plain-text body.',
      },
      gmailMailReply: {
        name: 'Reply to Gmail message',
        description: 'Replies in the original mail thread after explicit approval.',
      },
      gmailDraftCreate: {
        name: 'Create Gmail draft',
        description:
          'Creates a plain-text Gmail draft without sending it, after explicit approval.',
      },
      feishuAccountGet: {
        name: 'Get Feishu account',
        description: 'Returns limited identity data for the connected Feishu account.',
      },
      feishuDriveList: {
        name: 'List Feishu Drive files',
        description: 'Lists files visible to the connected Feishu identity.',
      },
      feishuDocumentRead: {
        name: 'Read Feishu document',
        description: 'Reads bounded content from an authorized Feishu document.',
      },
      feishuMessageSendUser: {
        name: 'Send Feishu message as user',
        description: 'Sends a message using the connected user identity after approval.',
      },
      feishuMessageSendBot: {
        name: 'Send Feishu bot message',
        description: 'Sends a bot message through an authorized tenant application.',
      },
      feishuMessageList: {
        name: 'Read Feishu messages',
        description: 'Reads a bounded page of recent messages from one visible Feishu chat.',
      },
      feishuCalendarEventList: {
        name: 'List Feishu calendar events',
        description: 'Lists a bounded page of events in one calendar and an explicit time range.',
      },
      feishuCalendarEventCreate: {
        name: 'Create Feishu calendar event',
        description:
          'Creates an event in a writable Feishu calendar. This write is disabled by default and requires approval when enabled.',
      },
      xAccountGet: {
        name: 'Get X account',
        description: 'Returns limited identity data for the connected X account.',
      },
      xPostListOwn: {
        name: 'List your X posts',
        description: 'Lists recent posts published by the connected X account.',
      },
      xPostSearchRecent: {
        name: 'Search recent X posts',
        description:
          'Searches recent public X posts when the connected account plan supports this API.',
      },
      xPostCreate: {
        name: 'Publish an X post',
        description: 'Publishes a post from the connected X account after explicit approval.',
      },
      xUserGetByUsername: {
        name: 'Get X user by username',
        description: 'Looks up one public X user by username and accepts an optional leading @.',
      },
      xPostListByUser: {
        name: 'List X posts by user',
        description: 'Lists a bounded page of public posts for one X user ID.',
      },
      webSearch: {
        name: 'Search the web',
        description:
          'Searches the public web for current information and returns bounded source metadata and highlights.',
      },
      webFetch: {
        name: 'Read webpages',
        description:
          'Reads bounded text or highlights from up to five public webpages. Returned content is untrusted data.',
      },
    },
    authMethods: {
      githubPersonal: {
        label: 'Personal access token',
        description:
          'Use a fine-grained GitHub personal access token with only the permissions required by the selected actions.',
      },
      githubOrganization: {
        label: 'Organization personal access token',
        description:
          'Use an organization-managed, fine-grained GitHub personal access token with least privilege.',
      },
      gmailOAuth: {
        label: 'Google account OAuth',
        description:
          'Connect a Google account in the provider window. ZGI never receives the Google password.',
      },
      gmailOrganizationOAuth: {
        label: 'Organization-managed Google OAuth',
        description:
          'Connect a Google account through an OAuth application owned by this organization.',
      },
      feishuUserOAuth: {
        label: 'Feishu user OAuth',
        description: 'Connect an individual Feishu user account with provider consent.',
      },
      feishuOrganizationOAuth: {
        label: 'Organization-managed Feishu user OAuth',
        description:
          'Connect a Feishu user through an OAuth application owned by this organization.',
      },
      feishuTenantApp: {
        label: 'Feishu tenant application',
        description:
          'Use an organization-managed application identity for explicitly allowed bot actions.',
      },
      xOAuth: {
        label: 'X account OAuth',
        description: 'Connect an individual X account with provider consent.',
      },
      xOrganizationOAuth: {
        label: 'Organization-managed X OAuth',
        description:
          'Connect an X account through an OAuth application owned by this organization.',
      },
      webSearchPlatform: {
        label: 'Platform managed',
        description: 'Use the Exa credential configured by the platform operator.',
      },
      webSearchOrganization: {
        label: 'Organization API key',
        description: 'Use an Exa API key owned by this organization.',
      },
    },
    credentialFields: {
      githubToken: {
        label: 'Personal access token',
        description:
          'A fine-grained token is recommended. It is encrypted before storage and is never returned by the API.',
        placeholder: 'Paste a GitHub personal access token',
      },
      oauthClientID: {
        label: 'Client ID',
        description: 'The public identifier of the OAuth application owned by your organization.',
        placeholder: 'Paste OAuth client ID',
      },
      oauthClientSecret: {
        label: 'Client secret',
        description:
          'Encrypted on save and never returned. Leave blank to keep the existing secret.',
        placeholder: 'Paste OAuth client secret',
      },
      exaApiKey: {
        label: 'Exa API key',
        description: 'The key is encrypted before storage and is never returned by the API.',
        placeholder: 'Paste an Exa API key',
      },
    },
    categories: {
      developer_tools: 'Developer tools',
      knowledge_retrieval: 'Knowledge retrieval',
      external: 'External apps',
      unknown: 'Other category',
    },
    tags: {
      code: 'Code',
      repositories: 'Repositories',
      issues: 'Issues',
      external: 'External service',
      web: 'Web',
      search: 'Search',
      unknown: 'Other capability',
    },
    scopes: {
      metadataRead: 'Repository metadata: read',
      issuesRead: 'Issues: read',
      webSearch: 'Web search',
      webRead: 'Webpage reading',
      unknown: 'Other permission',
    },
  },
  enums: {
    authType: {
      platform: 'Platform credential',
      api_key: 'API key',
      oauth: 'OAuth',
      oauth2: 'OAuth 2.0',
      custom_credential: 'Custom credential',
      service_account: 'Service account',
      no_auth: 'No authentication',
      unknown: 'Other authentication',
    },
    invokeFrom: {
      aichat: 'AIChat',
      agent: 'Agent',
      workflow: 'Workflow',
      api: 'API',
      unknown: 'Other caller',
    },
  },
  errors: {
    integration_disabled: 'External apps are disabled',
    integration_invalid_input: 'Invalid request parameters',
    integration_sensitive_input_blocked: 'Sensitive data was blocked before sending',
    integration_quota_exceeded: 'Organization quota exceeded',
    integration_auth_invalid: 'Credential is invalid',
    integration_budget_exceeded: 'Provider budget exhausted',
    integration_access_denied: 'Provider access denied',
    integration_rate_limited: 'Provider rate limit reached',
    integration_timeout: 'Provider request timed out',
    integration_upstream_unavailable: 'Provider is temporarily unavailable',
    integration_provider_rejected: 'Provider rejected the request',
    integration_response_invalid: 'Provider returned an invalid response',
    integration_audit_failed: 'Execution audit could not be recorded',
    integration_policy_conflict: 'Organization policy blocks this action',
    integration_reconnect_required: 'Reconnect the provider account',
    integration_connection_expired: 'Connection credential has expired',
    integration_insufficient_scope: 'Connection lacks a required provider permission',
    integration_action_auth_incompatible:
      'The current connection method does not support this action',
    integration_connection_not_found: 'Connection no longer exists',
    integration_connection_invalid: 'Connection is not available',
    integration_connection_conflict:
      'The connection changed or its name conflicts with another connection. Refresh and retry.',
    integration_connection_in_use: 'Connection is still in use',
    unknown: 'External app request failed',
  },
  units: {
    millisecondsShort: 'ms',
  },
  tabs: {
    catalog: 'App catalog',
    connections: 'Shared connections',
    policies: 'Action policies',
    executions: 'Execution logs',
  },
  catalog: {
    title: 'Provider catalog',
    description: 'Choose an app to configure accounts, credentials, actions, and access.',
    loadFailed: 'The provider catalog could not be loaded.',
    retry: 'Retry',
    searchPlaceholder: 'Search external apps',
    searchAria: 'Search external app catalog',
    empty: 'No external app providers are registered.',
    noResults: 'No external apps match this search.',
    noDescription: 'No description is available for this provider.',
    actions: 'Actions',
    connections: 'Ready / total',
    platformAvailable: 'Platform credential',
    setUp: 'Set up',
    manage: 'Manage',
  },
  detail: {
    back: 'Back to app catalog',
    loadFailed: 'This external app could not be loaded.',
    notFound: 'This external app is not registered.',
    documentation: 'Documentation',
    manageConnections: 'Manage shared connections',
    actions: 'Available actions',
    connections: 'Ready connections',
    authMethods: 'Auth methods',
    tabs: {
      overview: 'Overview',
      connections: 'Shared connections',
      authorization: 'Actions & usage rules',
      activity: 'Activity',
    },
    healthGuidance: {
      ready:
        'At least one connection has a healthy credential and provider API. This does not mean AIChat selected or called it.',
      configured:
        'Credentials are configured but have not passed a health check. This does not mean AIChat selected them.',
      setup_required: 'Create a connection or enable a platform credential before using this app.',
      degraded: 'At least one connection needs attention. Test or replace its credentials.',
      unavailable: 'This provider is currently unavailable.',
      unknown: 'Provider readiness could not be determined.',
    },
    availableActions: 'Available actions',
    dataEgress: 'Sends data to {destination}',
    externalProvider: 'the external provider',
    authentication: 'Authentication',
    available: 'Available',
    unavailable: 'Unavailable',
    healthProbeSupported: 'This provider supports connection health checks.',
    healthProbeMayCost: 'A check may make a billable provider request.',
    healthProbeNoCost: 'The provider reports that checks are not billable.',
  },
  health: {
    provider: {
      ready: 'Ready',
      configured: 'Configured',
      setup_required: 'Setup required',
      degraded: 'Needs attention',
      unavailable: 'Unavailable',
      unknown: 'Unknown',
    },
    connection: {
      ready: 'Healthy',
      testing: 'Pending test',
      degraded: 'Degraded',
      expired: 'Expired',
      revoked: 'Revoked',
      error: 'Unhealthy',
      disabled: 'Disabled',
      unknown: 'Unknown',
    },
  },
  connections: {
    add: 'Add shared connection',
    description:
      'Platform-provided and organization-owned connections are managed here. Personal connections are never shown.',
    providerFilter: 'Filter by external app',
    allProviders: 'All external apps',
    filteredBy: 'Showing {provider} shared connections',
    showAll: 'Show all',
    emptyTitle: 'No shared connections',
    emptyDescription:
      'Add a personal or organization connection, then make it available through usage rules.',
    loadFailed: 'Connections could not be loaded.',
    retry: 'Retry',
    testCostNotice:
      'A test only verifies the credential and provider API and may incur a small charge. Success does not mean AIChat selected or called the connection.',
    administrativeStatus: 'Admin status: {status}',
    table: {
      name: 'Name',
      integration: 'External app',
      credential: 'Connection scope',
      scope: 'Connection scope',
      status: 'Health',
      lastTested: 'Last tested',
      default: 'Default',
      actions: 'Actions',
      actionsFor: 'Connection actions for “{name}”',
    },
    status: {
      pending: 'Pending',
      active: 'Enabled',
      invalid: 'Invalid',
      disabled: 'Disabled',
    },
    credentialSource: {
      platform: 'Platform provided',
      organization: 'Organization shared',
      account: 'Personal · Only me',
    },
    neverTested: 'Never',
    defaultBadge: 'Default',
    personalOwnerOnly: 'Only the owner can manage this personal connection',
    actions: {
      view: 'View details',
      edit: 'Edit',
      test: 'Test connection',
      reconnect: 'Reconnect account',
      upgradeScopes: 'Update provider access',
      setDefault: 'Set as default',
      enable: 'Enable',
      disable: 'Disable',
      disconnectAccount: 'Disconnect account',
      delete: 'Delete',
    },
  },
  personal: {
    title: 'My personal connections',
    description:
      'Use your own provider account in AIChat. Only you can manage or use it; it is never shared with organization members or agents.',
    disabled:
      'External apps are disabled for this deployment, so personal connections are unavailable.',
    add: 'Add my connection',
    scopeBadge: 'Personal · Only me',
    secretNotice:
      'Secrets are encrypted on save and are never returned to the browser. Edit a connection to rotate its credential.',
    loadFailed: 'Your personal connections could not be loaded.',
    noProviders: 'No provider currently offers an available personal authentication method.',
    emptyTitle: 'No personal connections',
    emptyDescription:
      'Add your own provider credential, then select it from Connected Apps in AIChat.',
    lastTested: 'Last tested',
    credentialVersion: 'Credential v{version}',
    deleteDescription:
      'This permanently removes your encrypted credential. AIChat will no longer be able to use this personal connection.',
  },
  dialog: {
    createAndTestPersonal: 'Create and test',
    createAndTestShared: 'Create and test',
    saveAndTest: 'Save and test',
    testAfterSaveNotice:
      'The connection is tested immediately after its credential is saved and may incur a small provider charge.',
    createTitle: 'Add connection',
    editTitle: 'Edit connection',
    description: 'Secret values are encrypted and are never shown again after saving.',
    createSharedTitle: 'Add shared connection',
    editSharedTitle: 'Edit shared connection',
    sharedDescription:
      'Create an organization connection and configure which members, workspaces, and agents can use it. Secrets are encrypted.',
    createPersonalTitle: 'Add my connection',
    editPersonalTitle: 'Edit my connection',
    personalDescription:
      'This connection is available only to you in AIChat and is never shared with organization members or agents. Secrets are encrypted.',
    integration: 'External app',
    integrationPlaceholder: 'Select an external app',
    name: 'Connection name',
    namePlaceholder: 'For example: Team account',
    sharedNamePlaceholder: 'For example: Engineering GitHub',
    personalNamePlaceholder: 'For example: My GitHub',
    credentialSource: 'Credential source',
    platformCredential: 'Use platform credential',
    organizationCredential: 'Use organization credential',
    accountCredential: 'Use personal credential',
    authMethod: 'Authentication method',
    unknownAuthMethod: 'Authentication method',
    credentialField: 'Credential field',
    apiKeyAuth: 'API key',
    apiKey: 'API key',
    apiKeyPlaceholder: 'Paste API key',
    replaceApiKey: 'Replace API key',
    replaceApiKeyHint: 'Leave blank to keep the existing encrypted key.',
    keepExistingSecret: 'Leave blank to keep the saved value',
    replaceSecretHint:
      'Saved secret values cannot be viewed. Leave blank to keep the current value.',
    selectValue: 'Select a value',
    platformUnavailable: 'A platform credential is not available for this external app.',
    authUnavailable: 'This authentication flow is not available in the current deployment.',
    cancel: 'Cancel',
    create: 'Create connection',
    createShared: 'Create shared connection',
    createPersonal: 'Create my connection',
    save: 'Save changes',
    required: 'Complete all required fields.',
  },
  connectionDetail: {
    title: 'Connection details',
    description: 'Review identity, provider scopes, usage rules, and diagnostic status.',
    personalDescription:
      'Review the identity, provider access, and diagnostic status of your personal connection.',
    health: 'Connection health',
    credential: 'Credential',
    credentialStored: 'Encrypted and stored',
    credentialMissing: 'Not configured',
    authorization: 'Usage rules',
    authenticationStatus: 'Authentication',
    actionCount: '{count, plural, one {# allowed action} other {# allowed actions}}',
    organizationPolicy: 'Organization policy',
    secretNotice:
      'Secret values are never returned to the browser. Replace or reconnect credentials to rotate them.',
    identityTitle: 'Identity & configuration',
    provider: 'External app',
    connectionId: 'Connection ID',
    driver: 'Driver',
    externalIdentity: 'External identity',
    notReported: 'Not reported by provider',
    authType: 'Credential source / auth',
    authSummary: '{source} / {method}',
    ownerAccount: 'Personal connection owner',
    currentAccount: 'Current signed-in account',
    defaultConnection: 'Organization default',
    yes: 'Yes',
    no: 'No',
    accessTitle: 'Provider access',
    scopes: 'Provider scopes',
    noScopes: 'The provider did not report scopes.',
    allowedActions: 'Allowed actions',
    noActions: 'No actions are available.',
    policyInherited:
      'These are provider actions. Effective access is still constrained by organization policy and usage rules.',
    managePolicy: 'Manage action policy',
    diagnosticsTitle: 'Diagnostics',
    lastTested: 'Last tested',
    lastUsed: 'Last used',
    lastRuntimeSuccess: 'Last successful use',
    neverUsed: 'Never',
    expiresAt: 'Credential expires',
    accessTokenExpiresAt: 'Access token expires (automatically refreshed)',
    refreshTokenExpiresAt: 'Reauthorization expires',
    noExpiry: 'No expiry reported',
    lastError: 'Last error',
    noError: 'No error reported',
    updatedAt: 'Last updated',
    credentialVersion: 'Credential version',
    healthCheckedAt: 'Health checked',
    personalAccessTitle: 'Personal usage scope',
    personalAccessDescription:
      'Personal connections have a fixed usage scope and do not need organization usage rules.',
    personalAccessBadge: 'Personal · Only you',
    personalAccessAIChat: 'Only you can select and use this connection in AIChat.',
    personalAccessAgents: 'It cannot be shared with organization members, workspaces, or agents.',
    loadFailed: 'Connection details could not be loaded.',
  },
  permissionSummary: {
    title: 'Connection access',
    description:
      'Separates ZGI-adapted capabilities, provider grants, and connection-maintenance access.',
    availableCapabilities: 'Available capabilities',
    providerScopeCount: 'Provider grants',
    needsAttention: 'Needs attention',
    capabilitiesTitle: 'Adapted capabilities and authorization',
    capabilitiesDescription:
      'A healthy connection does not mean every action is authorized. Grant additional access only when needed.',
    availableOfTotal: '{available}/{total} available',
    noCapabilities: 'No adapted action is available for this authentication method.',
    missingAccess: 'Missing access',
    upgradeAction: 'Grant access',
    broad: 'Broad access',
    providerNative: 'Provider-native',
    broadWarning:
      'This credential includes broad provider access. ZGI will still execute only adapted actions allowed by the applicable usage rules.',
    missingWarning:
      'This connection is missing {count} required permissions. Reauthorize it or replace the credential.',
    providerDetailsTitle: 'Provider-reported permission details',
    providerDetailsCount: '{count} permissions. Collapsed by default to keep this view clear.',
    providerScopesNotReported: 'The provider did not report a complete permission list',
    providerScopesNotReportedDescription:
      'This authentication method did not return a displayable scope list. ZGI still verifies access when an action runs.',
    groups: {
      identity: 'Identity',
      lifecycle: 'Connection maintenance',
      provider: 'Provider-native access',
    },
    access: {
      unknown: 'Unclassified',
      read: 'Read',
      write: 'Write',
      manage: 'Manage',
      identity: 'Identity',
      session: 'Keep signed in',
    },
  },
  grants: {
    title: 'Usage rules',
    description: 'Choose who may use this connection and which provider actions are available.',
    add: 'Add usage rule',
    empty: 'No usage targets or available actions are configured yet.',
    loadFailed: 'Usage rules could not be loaded.',
    organizationPrincipal: 'Current organization',
    principal: {
      organization: 'Entire organization',
      workspace: 'Specific workspace',
      account: 'Specific member · AIChat only',
    },
    accessMode: {
      read: 'Read only',
      write: 'Read and write',
    },
    allActions: 'All provider actions',
    allActionsDescription:
      'Legacy rule. Saving it again expands it to the provider actions available now.',
    edit: 'Edit usage rule',
    delete: 'Delete usage rule',
    createTitle: 'Add usage rule',
    editTitle: 'Edit usage rule',
    editorDescription:
      'Choose who may use this shared connection. Organization Action policies are enforced as a separate gate.',
    principalLabel: 'Usage target',
    principalId: 'Usage target',
    principalIdHint: 'Choose a usage target that belongs to the current organization.',
    principalPlaceholder: {
      workspace: 'Select a workspace',
      account: 'Select a member',
    },
    accessModeLabel: 'Action access',
    actionsLabel: 'Allowed actions',
    scopeLabel: 'Who can use this connection?',
    scope: {
      organization: {
        title: 'Entire organization',
        description:
          'Organization members may use it in AIChat. Agents still need an explicit connection and Action binding.',
      },
      workspace: {
        title: 'Specific workspace',
        description:
          'Available in that workspace context. Agents in the workspace still need an explicit binding.',
      },
      account: {
        title: 'Specific member · AIChat only',
        description:
          'Only the selected member may use it in AIChat. This does not apply to agents.',
      },
    },
    principalPicker: {
      label: {
        workspace: 'Workspace',
        account: 'Organization member',
      },
      placeholder: {
        workspace: 'Select a workspace',
        account: 'Select a member',
      },
      search: {
        workspace: 'Search workspaces',
        account: 'Search members by name or email',
      },
      empty: {
        workspace: 'No workspaces match this search.',
        account: 'No members match this search.',
      },
      missing: {
        workspace: 'Workspace is no longer available',
        account: 'Member is no longer available',
      },
      missingDescription: {
        workspace: 'This workspace no longer belongs to the organization. Select a replacement.',
        account: 'This account is no longer an active organization member. Select a replacement.',
      },
      loading: 'Loading members…',
      loadFailed: 'Members could not be loaded.',
      resolving: 'Resolving selection…',
      unnamed: {
        workspace: 'Unnamed workspace',
        account: 'Unnamed member',
      },
    },
    principalState: {
      missing: {
        workspace: 'Unavailable workspace',
        account: 'Unavailable member',
      },
      missingBadge: 'Needs attention',
    },
    permissionLevel: 'Action access',
    permissionLevelDescription:
      'Calculated from the selected actions using the smallest required access.',
    permissionPending: 'Select Actions first',
    actionEffect: {
      none: 'No external effect',
      read: 'Read',
      create: 'Create',
      update: 'Update',
      delete: 'Delete',
      publish: 'Publish',
      invoke: 'Invoke',
      schedule: 'Schedule',
      external_send: 'Send externally',
      unknown: 'Unknown effect',
    },
    riskLevel: {
      low: 'Low risk',
      medium: 'Medium risk',
      high: 'High risk',
      critical: 'Critical risk',
      unknown: 'Unknown risk',
    },
    unavailableAction: 'Unavailable legacy Action',
    unknownActionsBlocking:
      'Remove every unavailable legacy action before saving this rule. The current provider no longer defines these actions.',
    explicitActionsNotice:
      'Only the actions selected here are available. Actions added by the provider later require an explicit rule update.',
    readOnlyGrant: 'Read-only rule',
    resourceConstrainedDescription:
      'This rule contains resource-level constraints that this editor cannot safely preserve. It can be reviewed or deleted, but not edited here.',
    readOnlyDescription: 'This rule is read-only and cannot be edited from this page.',
    editDisabled: 'This rule cannot be edited from this page',
    summary: {
      organization:
        'Allow “{principal}” to use {count, plural, one {# {permission} action} other {# {permission} actions}}. Agents still need an explicit binding.',
      workspace:
        'Allow the “{principal}” workspace to use {count, plural, one {# {permission} action} other {# {permission} actions}}. Agents still need an explicit binding.',
      account:
        'Allow only “{principal}” to use {count, plural, one {# {permission} action} other {# {permission} actions}} in AIChat. This does not apply to agents.',
      pendingPrincipal: 'Select a target',
    },
    unionNotice:
      'Usage rules are additive and never override one another. A narrower rule cannot revoke access already made available to the organization or workspace.',
    validationError: 'Choose a usage scope, its target when required, and at least one action.',
    save: 'Save usage rule',
    deleteTitle: 'Delete this usage rule?',
    deleteDescription:
      'This removes only the access provided by this rule. The subject may still have access through another organization or workspace rule.',
  },
  connectionHealth: {
    reason: {
      runtimeSuccess: 'The external app call succeeded.',
      connectionTestSucceeded: 'The connection test succeeded.',
      scheduledCheckSucceeded:
        'A legacy scheduled check succeeded. Periodic checks are now disabled.',
      unknown: 'No additional health detail is available.',
    },
    title: 'Connection health',
    description:
      'Status changes only after a manual test or a real external-app call. ZGI never runs periodic provider checks.',
    overall: 'Overall health',
    authentication: 'Authentication',
    scope: 'Provider scope check',
    failures: 'Consecutive failures',
    authStatus: {
      valid: 'Valid',
      reconnect_required: 'Reconnect required',
      expired: 'Expired',
      unknown: 'Unknown',
    },
    scopeStatus: {
      verified: 'Verified',
      drifted: 'Scope drift detected',
      unknown: 'Unknown',
    },
    healthStatus: {
      healthy: 'Healthy',
      degraded: 'Degraded',
      unhealthy: 'Unhealthy',
      unknown: 'Unknown',
    },
    attention: {
      reconnect_required: 'Reconnect this account to restore access.',
      scope_update_required: 'Provider scopes no longer satisfy the selected actions.',
      billing_required: 'Provider billing or quota needs attention.',
      provider_incident: 'The provider appears to be experiencing an incident.',
      admin_check_required: 'An administrator should review this connection.',
    },
    missingScopes: 'Missing provider scopes: {scopes}',
    lastChecked: 'Last health check',
    lastHealthy: 'Last healthy',
    lastRuntimeSuccess: 'Last runtime success',
    lastRuntimeFailure: 'Last runtime failure',
    scopeChecked: 'Provider scopes last checked',
    history: 'Health history',
    historyCount: '{count, plural, one {# recorded event} other {# recorded events}}',
    refresh: 'Refresh saved history · no provider request',
    loadFailed: 'Health history could not be loaded.',
    empty: 'No health events have been recorded yet.',
    source: {
      manual: 'Manual test',
      scheduled: 'Legacy scheduled check · disabled',
      runtime: 'Runtime signal',
      oauth_refresh: 'OAuth refresh',
    },
    checkKind: {
      full: 'Full check',
      auth: 'Authentication',
      scope: 'Provider scopes',
      passive: 'Passive',
    },
    classification: {
      success: 'Success',
      auth_invalid: 'Authentication invalid',
      oauth_expired: 'OAuth expired',
      scope_drift: 'Provider scope drift',
      access_denied: 'Access denied',
      budget_exhausted: 'Budget exhausted',
      rate_limited: 'Rate limited',
      transient: 'Transient failure',
      provider_incident: 'Provider incident',
      ignored: 'Ignored',
    },
    notApplied: 'Superseded observation',
    resultingHealth: 'Result',
    latency: '{value} ms',
    previous: 'Previous',
    next: 'Next',
    page: 'Page {page}',
  },
  test: {
    title: 'Test this connection?',
    description:
      'This provider request runs only after you confirm and may incur a small charge. ZGI does not test connections periodically. No credential value is returned or logged. Success does not enable or call the connection in AIChat: a shared connection still needs usage rules, then selection under External Apps in AIChat.',
    cancel: 'Cancel',
    confirm: 'Run test',
  },
  delete: {
    title: 'Delete connection?',
    description: 'This permanently removes the encrypted credential and cannot be undone.',
    loadingImpact: 'Checking dependent agents...',
    impact:
      '{count, plural, one {# agent currently references} other {# agents currently reference}} this connection.',
    impactUnavailable: 'The connection usage could not be verified. Try again before removing it.',
    defaultImpact: 'This is the organization default. Choose another default after deletion.',
    cancel: 'Cancel',
    confirm: 'Delete connection',
  },
  disconnect: {
    title: 'Disconnect this account?',
    description:
      'Disconnecting “{account}” removes the encrypted {provider} OAuth credential stored by ZGI and asks the provider to revoke remote access when supported. It does not delete your provider account or remove the organization OAuth application configuration.',
    personalEffect:
      'This account is also removed from your AIChat External Apps selection. If the provider cannot confirm revocation automatically, ZGI keeps a recovery item and asks you to check the provider’s connected-app settings.',
    sharedEffect:
      'Remove agent bindings before disconnecting a shared account. After disconnection, members and workspaces can no longer access the provider through this connection.',
    resolveDependencies:
      'Remove this connection from the affected agents, then disconnect the account again.',
    confirm: 'Disconnect account',
  },
  policies: {
    integration: 'External app',
    selectIntegration: 'Select an external app',
    description:
      'Organization policies may disable or tighten an action, but cannot reduce provider risk.',
    empty: 'This external app does not expose configurable actions.',
    loadFailed: 'Action policies could not be loaded.',
    action: 'Action',
    enabled: 'Enabled',
    dataEgress: 'Allow data egress',
    approval: 'Confirmation',
    inherit: 'Inherit provider policy',
    alwaysAsk: 'Confirm every time',
    immutable: 'Provider effect, risk level, and destination remain enforced.',
    save: 'Save policies',
  },
  providerDiagnostics: {
    errorCode: 'Provider error code',
    httpStatus: 'HTTP status',
    retryAfter: 'Retry after',
    requestId: 'Provider request ID',
    hiddenRequestId: 'Reference hidden',
  },
  executions: {
    description:
      'Audit metadata and safe provider diagnostics only. Queries, credentials, provider messages, response bodies, and headers are not stored.',
    loadFailed: 'Execution logs could not be loaded.',
    empty: 'No external app executions match the current filters.',
    retry: 'Retry',
    statusFilter: 'All statuses',
    status: {
      running: 'Running',
      succeeded: 'Succeeded',
      failed: 'Failed',
      timedOut: 'Timed out',
    },
    table: {
      time: 'Time',
      integration: 'External app / action',
      connection: 'Connection',
      caller: 'Caller',
      status: 'Status',
      duration: 'Duration',
      cost: 'Cost',
      requestId: 'Provider request ID',
      error: 'Error / provider diagnostics',
    },
    noConnection: 'Platform fallback',
    unknownConnection: 'Connection no longer available',
    hiddenReference: 'Reference hidden',
    noValue: '—',
  },
  messages: {
    created: 'Connection created',
    updated: 'Connection updated',
    tested:
      'Connection test passed: the credential and provider API are available. This does not mean AIChat selected or called it.',
    defaultSet: 'Default connection updated',
    deleted: 'Connection removed',
    policiesSaved: 'Action policies saved',
    grantSaved: 'Usage rule saved',
    grantDeleted: 'Usage rule deleted',
    oauthClientConfigured: 'OAuth application configuration saved',
    oauthClientRemoved: 'OAuth application configuration removed',
    oauthRecoveryAcknowledged: 'OAuth recovery item resolved',
    personalCredentialRequired: 'A personal connection must use your personal credential.',
    requestFailed: 'The external app request failed',
  },
};

export type IntegrationMessages = typeof messages;
export default messages;
