const messages = {
  backgroundGrants: {
    title: 'Scheduled connector access',
    description:
      'Let scheduled runs use selected operations on one connected account within a fixed time and call limit.',
    create: 'Add grant',
    creating: 'Adding…',
    created: 'Scheduled connector access added.',
    createFailed: 'Could not add scheduled connector access.',
    loadFailed: 'Could not load scheduled connector access.',
    empty: 'No scheduled connector access has been granted for this digital human.',
    regrantNotice:
      'Publishing a new version, reconnecting the account, or changing its permissions makes an existing grant stale. Review the new setup and grant access again.',
    unknownAccount: 'Unavailable account',
    status: {
      active: 'Active',
      expired: 'Expired',
      revoked: 'Revoked',
      stale: 'Needs a new grant',
    },
    expires: 'Expires {date}',
    usage: '{used} of {total} calls used',
    revoke: 'Revoke',
    revoked: 'Scheduled connector access revoked.',
    revokeTitle: 'Revoke scheduled connector access?',
    revokeDescription:
      'Future scheduled runs will need confirmation or another matching grant for these operations.',
    revokeFailed: 'Could not revoke scheduled connector access.',
    dialogTitle: 'Add scheduled connector access',
    dialogDescription:
      'Choose one connected account and only the operations the digital human needs. Nothing is selected by default.',
    account: 'Connected account',
    accountPlaceholder: 'Choose an account and binding',
    operations: 'Allowed operations',
    operationsDescription:
      'Each selected operation can run without per-call confirmation only while this grant still matches the published version and account.',
    noOperations: 'This account currently has no operations available for a background grant.',
    duration: 'Duration in days',
    durationHint: 'From 1 to 90 days. The default is 7 days.',
    maxCalls: 'Maximum calls',
    destinations: 'Exact destination constraints (optional)',
    destinationsDescription:
      'Add a field and its exact allowed value, such as account_id and the approved page. A scheduled call must match every row.',
    addDestination: 'Add constraint',
    destinationField: 'Destination field',
    destinationFieldPlaceholder: 'Field, for example account_id',
    destinationValue: 'Exact destination value',
    destinationValuePlaceholder: 'Exact allowed value',
    removeDestination: 'Remove destination constraint',
    cancel: 'Cancel',
    errors: {
      binding: 'Choose a connected account.',
      operations: 'Select at least one operation. Operations cannot be duplicated.',
      duration: 'Enter a whole number from 1 to 90 days.',
      maxInvocations: 'Enter a whole number from 1 to 100,000 calls.',
      destinations:
        'Use unique field names and exact values. Wildcards, line breaks, and empty values are not allowed.',
      request:
        'The grant was not created. Review the live publication and account, then try again.',
    },
  },
  oauth: {
    title: 'Connect your platform account',
    description:
      'Authorize access on the platform. ZGI saves credentials securely and renews them automatically when the platform supports it. You may need to reconnect when authorization expires or is revoked.',
    connect: 'Connect account',
    reconnect: 'Reconnect account',
    reconnectDescription:
      'Authorize the same account to keep this connection and its usage locations. A different account or Facebook Page creates a separate connection.',
    loading: 'Loading…',
    waiting: 'Complete authorization in the new window. This screen updates automatically.',
    installApp:
      'Install ZGI Connect on GitHub and choose the repositories to allow. Then return here to finish connecting.',
    openInstallation: 'Choose GitHub repositories',
    completeInstallation: 'I have installed the app — continue',
    installationFailed:
      'Could not verify the installation. Check the selected account and repository access, then try again.',
    selectPage: 'Choose the Facebook Page to connect',
    noPages: 'No authorized Pages are available. Reconnect and grant access to a Page you manage.',
    pageFailed: 'Could not save this Page. Retry your selection or refresh the status.',
    ready: 'Your connection is ready. You can close the authorization window.',
    changedIdentity:
      'A different account or Page was authorized. A new connection was created; existing usage locations still use the original connection.',
    failed: 'Authorization could not be completed. Retry authorization or use manual credentials.',
    statusFailed: 'Could not load authorization status. Retry to check the result.',
    blocked: 'Allow popups for this site, then try Connect again.',
    closed:
      'The authorization window was closed. Reconnect to start again, or wait for the saved result.',
    unavailable:
      'An administrator must configure this platform’s OAuth application before automatic authorization is available. You can use manual credentials below.',
    manual: 'Use manual credentials',
    back: 'Back to platform authorization',
    returnToWindow:
      'After authorization and any Page selection, return to your original ZGI window. It updates automatically.',
  },
  operationPolicy: {
    added: 'Will allow: {operations}',
    removed: 'Will stop allowing: {operations}',
    title: 'Connection usage',
    notice:
      'These settings control what this connection can do in ZGI, across all its usage locations.',
    notAccepted:
      'This connection has not accepted this capability. Update and verify its credentials first.',
    paused: 'No operations selected: saving pauses all tool operations for this connection.',
    sendConfirmation:
      'Sending still requires confirmation. Allowed usage does not prove access to upstream resources.',
    save: 'Save usage',
    saved: 'Usage saved.',
    failed:
      'The save result could not be confirmed. Refresh and check the status; retrying the same selection will not apply it twice.',
    conflict: 'This connection changed. Refresh it before selecting usage again.',
  },
  capabilityGroups: {
    label: 'View by usage',
    all: 'All operations',
    messaging: 'Chats, messages and cards',
    documents: 'Documents, storage and comments',
    people: 'People and tenant',
    resources: 'Message resources',
    subject: 'Credential subject',
    unknown: 'Unknown',
    confirmation:
      'This operation changes or sends content and remains subject to confirmation policy.',
    scopes: 'Required scope combinations',
    scopeLogic:
      'Every row is required; any scope within a row satisfies that row. Approved scopes still require access to the target resource.',
    resourceOnly:
      'Availability is checked against the credential subject and resource access at execution.',
  },
  mail: {
    fixedUsage: 'This connection can read and send email.',
    authorizationCodeHint: 'Enter the client authorization code generated by your email provider.',
    readFailed: 'Receiving verification failed',
    sendFailed: 'Sending verification failed',
    title: 'Email',
    provider: 'Email provider',
    catalogDescription: 'Connect QQ Mail, 163 Mail or Gmail to read and send email',
    upgradeReconnect: 'Reconnect and enable read and send',
    upgradeDescription: 'After verification, this connection will allow reading and sending email.',
    authorizationCode: 'Client authorization code',
    getAuthorizationCode: 'Official authorization code instructions',
    providerSettings: 'Official mail settings and help',
    qqNotice:
      'Enable the required IMAP/SMTP services in QQ Mail account security settings and generate a client authorization code. Use a standard @qq.com address, not your QQ login password.',
    neteaseNotice:
      'Enable the required services under POP3/SMTP/IMAP in NetEase Mail settings and generate an authorization password, or use third-party login management in Mail Master. Use a standard @163.com address, not the mailbox login password.',
    qqEmailHint: 'Enter your complete @qq.com email address.',
    neteaseEmailHint: 'Enter your complete @163.com email address.',
    gmailEmailHint: 'Gmail accepts @gmail.com or an eligible Google Workspace domain.',
    qqEmailPlaceholder: 'name@qq.com',
    neteaseEmailPlaceholder: 'name@163.com',
    gmailEmailPlaceholder: 'name@gmail.com or Workspace address',
    qqPasswordHint:
      'Enter the client authorization code generated by QQ. You can paste it directly; do not use the login password.',
    neteasePasswordHint:
      'Enter the client authorization code generated by NetEase. You can paste it directly; do not use the mailbox login password.',
    invalidAuthorizationCode: 'Enter a valid client authorization code.',
    clientIdentityRequired:
      'The mail server did not accept the client identity or access policy. Contact your deployment administrator to check protocol compatibility; keep mailbox security enabled.',
    authMechanismUnsupported:
      'The server advertises no supported authentication mechanism. Contact your deployment administrator.',
    partialFailure:
      'Reading and sending were not both verified. Check your email settings and reconnect.',
    protocol: {
      verified: 'Authenticated',
      failed: 'Verification failed',
      not_requested: 'Not requested',
    },
    policyRestricted:
      'Email settings or organization policy restrict reading or sending. Check settings or contact your administrator.',
    authenticationRejected:
      'The mail service rejected login. Check the full address, client password or authorization code, and enabled services. Generate a replacement for revoked credentials.',
    tlsFailed:
      'A secure mail-server connection could not be established. Check connectivity and retry without changing security settings.',
    notice:
      'Connect Gmail or an eligible Google Workspace mailbox using its full email address and a Google app password. Enable 2-Step Verification first; your normal Google password is not accepted.',
    email: 'Email address',
    emailHint: 'Enter the full mailbox login, including your Workspace domain if applicable.',
    lockedEmail: 'To use another mailbox, add a new connection.',
    invalidEmail: 'Enter a full email address for the selected provider.',
    appPassword: 'Google app password',
    passwordHint: 'Use the app password generated by Google.',
    invalidPassword: 'Enter a valid Google app password.',
    usage: 'Allow ZGI to',
    read: 'Read mail',
    send: 'Send mail',
    read_send: 'Read and send mail',
    localUsage:
      'This selection restricts operations inside ZGI. Client passwords and authorization codes do not provide separate read and send scopes.',
    getPassword: 'Get a Google app password',
    officialHelp: 'Google instructions',
    eligibilityTitle: 'Cannot find the app password option?',
    eligibility:
      'App passwords require 2-Step Verification. The option may be unavailable for accounts using only security keys, Advanced Protection, or organization policies. Your email domain alone cannot confirm eligibility; use Google settings or ask your administrator.',
    imapPolicy:
      'Personal Gmail already has IMAP enabled. Workspace administrators may restrict IMAP and app password access; read access depends on those policies.',
    workspaceHelp: 'Workspace administrator guidance',
    connect: 'Connect email',
    reconnect: 'Reconnect',
    failed:
      'Connection failed. Check the email address and app password or authorization code, then retry.',
    retry: 'Enter credentials again',
    verifying:
      'Verifying only the protocols required by your choice: IMAP for reading and SMTP authentication for sending. No test email is sent.',
    verifiedHint:
      'The chosen mailbox login protocols were verified. SMTP authentication does not verify delivery; sending still depends on recipients, quotas and account policies.',
    currentUsage: 'Current use: {usage}',
    secret:
      'The app password is write-only and cleared from the form after submission or closing. ZGI stores it encrypted on the server; it is never included in shared chat, agent or workflow settings.',
    revokeHint: 'Disconnecting removes the ZGI connection. To revoke client credentials, see',
    googleSettings: 'Google app password settings',
  },
  platform: {
    resourceAtExecution:
      'Access to the selected recipient, conversation or document is checked when the operation runs.',
    capabilityTitle: 'Capabilities and platform conditions',
    feishu:
      'The template requests application permissions. Approval, publishing and resource access still control each operation. Continuing the original App ID may update the app configuration; events and user authorization are not activated by displaying the template.',
    dingtalk:
      'The official QR page can offer an existing robot. When reconnecting, choose the original robot; ZGI verifies the returned AppKey. Sending remains conditional on the recipient and conversation. Contact access is not offered until its application and permission requirements are verified.',
    wecom:
      'Supports bot messages and connected document and table operations. Resource access is checked when used.',
    weixin:
      'The current personal WeChat integration retrieves messages and replies through iLink. Reconnect with the original account, completing any verification code or existing-binding prompt on the official page. Contacts, arbitrary group access and full history are not included.',
    evidenceHint:
      'Implemented operations, observed permissions and resource conditions are separate. Unknown evidence is not authorization; changing a template does not grant new access automatically.',
  },
  pending: {
    title: 'Pending completion',
    awaitingShort: 'Waiting for app readiness',
    verifyShort: 'Verification required',
    count: '{count} pending',
    continue: 'Continue connection',
    awaitingHint:
      'Complete app approval, publication and availability settings in Feishu, then continue verification.',
    verifyHint: 'Continue verification for the app below.',
    lastChecked: 'Last verification: {date}',
    retention: 'Continue this connection by {date}.',
    expiringSoon:
      'This record expires in less than 3 days. Complete verification soon; the app in Feishu will not be deleted.',
    qrUnavailable: 'QR continuation is unavailable. Enter the original app’s App Secret.',
    resumeQRHint: 'Scan and confirm the original App ID and configuration on the Feishu page.',
    secretHint:
      'Find App Secret in the existing app’s developer console under Credentials & Basic Info. It is used only for this verification and stored encrypted only after success. No renewal token is needed.',
    openConsole: 'Open the app developer console',
    refresh: 'Refresh status',
    closeHint:
      'Closing this window keeps the pending record. No usable connection or conversation binding exists until verification succeeds.',
    resumeQR: 'Scan to resume this app',
    verify: 'Verify existing app and connect',
    loadFailed: 'Could not load pending records. Saved connections are unchanged.',
    remove: 'Remove pending record',
    removeTitle: 'Remove this pending record?',
    removeFailed:
      'Could not remove this record; its state may have changed. Refresh the list and try again.',
    removeHint:
      'Only this local record and its active scan will be cancelled. The app in Feishu and saved connections will not be deleted.',
    expired: 'Pending record expired',
    cancelled: 'Pending record removed',
    completed: 'This app is already connected',
    terminalHint: 'Use the saved connection or add the original app again.',
    newManual: 'Add existing app manually',
    back: 'Back to connectors',
    invalidLink: 'This connection link is invalid or unavailable in the current workspace.',
    updateSecret: 'Update app Secret',
    updateSecretHint:
      'The app identity stays fixed. The current Secret and bindings remain unchanged if verification fails.',
  },
  messaging: {
    alreadyBound:
      'WeChat reports an existing binding without new credentials. Review your original connection or start a new login. No connection was created.',
    regionUnsupported:
      'This deployment does not support this WeChat service region. Contact your administrator.',
    originalSubject: 'Original connection: {name} ({identity})',
    phase: {
      token: 'Checking the received credential.',
      identity: 'Checking that this is the expected account or bot.',
      operation: 'Checking available operations and resource conditions.',
      checkpoint: 'Preserving the verification result.',
      commit: 'Finishing the connection update.',
    },
    verificationWaiting: 'Verification is incomplete. Retry verification to continue.',
    retryVerification: 'Retry verification',
    verificationDeadline: 'Continue verification before {date}.',
    dingtalkReauthorize:
      'Choose the original bot on the official page. Add a new connection to use a different bot.',
    wecomReauthorize:
      'Enter the original bot ID and Secret. Add a new connection to use a different bot.',
    weixinReauthorize: 'Scan with the original WeChat account and confirm on your phone.',
    startAgain: 'Enter credentials again',
    reauthorizeTitle: 'Update connection credentials',
    restart: 'I checked the outcome; prepare a new connection',
    notice:
      'Add a personal connection, then select it in Chat, Agents or Workflows for explicit operations.',
    reauthorizeHint:
      'Update credentials for the original account or bot using the methods shown for this platform. A different identity requires adding a separate connection.',
    dingtalk:
      'Send text or Markdown to eligible users and groups. Application credentials are verified first; permissions and recipient eligibility are checked when sending.',
    wecom: 'Use bot messages, documents and tables with the required resource access.',
    weixin:
      'Retrieve direct messages explicitly and reply using their conversation context. Manual token setup is not available.',
    unavailable: 'This connection service is currently unavailable. Please try again later.',
    dingtalkPermission: 'DingTalk refused to create the app: the scanning account has no developer permission in the selected organization. Make sure you picked the organization you manage when scanning and that developer access is enabled on open-dev.dingtalk.com, or enter an existing app\'s AppKey and AppSecret instead.',
    qrUnavailable: 'QR connection is currently unavailable. You can enter credentials manually.',
    qr: 'Connect with QR',
    start: 'Start QR connection',
    verify: 'Verify and save',
    starting: 'Preparing the QR connection…',
    waiting: 'Waiting for confirmation on your phone…',
    verifying: 'Verifying connection…',
    ready: 'Connected',
    localExpired: 'This waiting period has ended. Check the flow before starting again.',
    cancelled:
      'The local connection flow was cancelled. A bot already created at the provider is not deleted.',
    failed:
      'The connection could not be completed. Check credentials, authorization and the latest flow status.',
    recovery:
      'The request outcome is uncertain. Check this flow and saved connections before starting again. Registration and messages are not automatically resent.',
    scanHint: 'Scan and confirm with {platform}.',
    code: 'Numbers shown on your phone',
    submitCode: 'Submit code',
    cancel: 'Cancel this flow',
    check: 'Check flow status',
    secret:
      'Secrets and QR challenges are handled privately. Credentials are saved after verification; replies require the current connection and conversation context.',
    conditional: 'Checked when sending',
    contextRequired: 'Conversation context required',
    unverified: 'Connect to verify',
    permissionUnknown: 'Permission unverified',
  },
  app: {
    permissionSummary:
      '{verified} verified; {missing} not granted; {unknown} awaiting verification.',
    configure: 'Configure app',
    permissionDetails: 'Permission details',
    permissionVerified: 'Verified',
    permissionNotGranted: 'Not granted',
    credentialsRequired: 'Scan to create an app, or enter your application credentials.',
    reconnectTitle: 'Application connection needs attention',
    reconnectDescription:
      'Check the application identity and permissions, or update its Secret. Existing bindings are preserved.',
    title: 'Feishu application connection',
    notice:
      'Use Feishu as a self-built application in Chat, Agents and Workflows. This is not a bot messaging channel.',
    qr: 'Create with QR',
    manual: 'Enter credentials',
    region: 'Application region',
    feishu: 'Feishu',
    lark: 'Lark',
    start: 'Start registration',
    scan: 'Scan with Feishu / Lark and confirm configuration',
    scanHint: 'Scan with Feishu or Lark.',
    open: 'Cannot scan? Open the official authorization page',
    cancel: 'Cancel registration',
    restore: 'Check this registration',
    pending: 'Waiting for confirmation on your phone…',
    starting: 'Preparing registration…',
    verifying: 'Verifying application identity and granted permissions…',
    ready: 'Connected',
    readyHint:
      'Only verified operations are available. Events and user authorization are not enabled in ZGI.',
    recovery:
      'The registration outcome is uncertain. Check the Feishu developer console and add the application manually if needed. No new application will be created automatically.',
    expired:
      'This QR code has expired. If the app was created, continue from its pending record. Without a record, use manual setup with the existing app credentials to avoid creating another app.',
    cancelled:
      'Registration cancelled. Any application already created in Feishu is not automatically deleted.',
    failed: 'Registration did not complete. Other saved connections are unchanged.',
    invalid:
      'Could not verify the App ID and App Secret. Check the region and make sure both belong to the same application.',
    permissions: 'The application is missing required permissions. Grant them and verify again.',
    notReady: 'The application is not ready. Check activation, publication and authorization.',
    rate: 'Registration is rate-limited or another flow is already active. Try again later.',
    secretNotice:
      'The App Secret is write-only. Candidate credentials are not saved before verification succeeds.',
    use: 'Use this connection',
    manage: 'Manage this connection',
    newConnection: 'Saved as a new independent connection. Existing bindings are unchanged.',
    updateTitle: 'Update application Secret',
    updateHint:
      'The application identity stays fixed. A verified Secret replaces the current one; failed verification preserves credentials and bindings. Use Add connection for another app.',
    repairTitle: 'Configure application permissions',
    repairHint: 'Scan and check the original app on the official page.',
    repair: 'Configure with QR',
    evidenceTitle: 'Application identity and permissions',
    template: 'Current registration template',
    templateIncrementalHint:
      'Only this template is requested, without merging the upstream default. Requested scopes are not verified grants. Removing a scope here does not revoke an existing platform grant; changes may require approval and publishing.',
    userEventDependencyHint:
      'These user events also require the user permissions and subscription APIs below. They are not requested or called by this flow. offline_access alone does not enable them; ZGI does not consume these events.',
    requiredUserScope: 'Additional user permission',
    requiredSubscription: 'Required subscription API',
    granted: 'Verified grants',
    unknown: 'Not verified',
    appScopes: 'Application permissions',
    userScopes: 'User permissions (user authorization not enabled)',
    events: 'Events and callbacks (configuration only, not consumed by ZGI)',
    configurationOnly: 'A template request is not proof of authorization or event support in ZGI.',
    appEvents: 'Application events',
    userEvents: 'User events',
    callbacks: 'Callbacks',
    scopeHint: 'Actual resource access remains subject to Feishu sharing and permissions.',
    notVerified: 'Verify after connecting',
    refreshConnections: 'Check saved connections',
    retryManual: 'Enter credentials again',
    requestFailed:
      'The request could not be completed. Check the registration or connection state before retrying.',
  },
  references: {
    title: 'References needing replacement',
    description:
      'This connection is unavailable. Choose a personal connection or remove this reference.',
    originalConnection: 'Original connection: {connection}',
    published: 'This published version is immutable. Repair its draft and publish a new version.',
    pendingSave: 'Change queued for the next save.',
    connection: 'Choose a connection',
    operation: 'Choose an operation',
    replace: 'Replace',
    detach: 'Remove reference',
    reasonUnsupported: 'The original connection method or provider is no longer supported.',
    reasonAccess: 'The original owner or access restrictions do not permit this operation.',
    reasonSelection: 'This snapshot needs a connection selected for the current account.',
    reasonUnconfigured:
      'This workflow node was saved without a connection. Choose one and save the workflow to finish setup.',
    reasonUnavailable: 'The connection or operation is unavailable.',
  },
  title: 'Connectors',
  disabledDescription: 'Connectors are not available for this deployment.',
  searchPlaceholder: 'Search Connectors and Data Sources',
  searchAria: 'Search Connectors and Data Sources',
  manageConnections: 'Manage my connections',
  metadata: {
    descriptionUnavailable: 'A description is not available for this Connector.',
    capabilityDescriptionUnavailable: 'A description is not available for this capability.',
  },
  filters: {
    label: 'Filter Connector status',
    all: 'All',
    connected: 'Connected',
    needsAttention: 'Needs attention',
    category: 'Filter by category',
    tag: 'Filter by tag',
    allCategories: 'All categories',
    allTags: 'All tags',
  },
  sections: {
    connectors: 'Connectors',
    connectorsDescription: 'Connect your apps and accounts',
    dataSources: 'Data Sources',
    dataSourcesDescription: 'Find external information',
    empty: 'No items are available in this category.',
  },
  catalog: {
    connectedCount: '{count} connected',
    manageConnections: 'Manage',
    connectionActions: 'Connection actions',
    loading: 'Loading Connectors',
    loadFailedTitle: 'Connectors could not be loaded',
    loadFailedDescription: 'Try again without changing any saved connections.',
    emptyTitle: 'No matching Connectors',
    emptyDescription: 'Adjust your search or status filter and try again.',
    resetFilters: 'Reset filters',
    keyRequired: 'Personal Key required',
    connectionCount: '{count} connections',
    needsAttentionCount: 'Needs attention: {count}',
    connectedAs: 'Connected as {name}',
    pluginNote:
      'Plugins provide capabilities. Installing a plugin never creates a personal connection or stores your Key.',
  },
  classification: {
    connector: 'Connector',
    dataSource: 'Data Source',
  },
  categories: {
    communication: 'Social & communication',
    productivity: 'Productivity tools',
    information: 'Information retrieval',
    developer: 'Developer tools',
    other: 'Other tools',
  },
  tags: {
    social: 'Social',
    messaging: 'Messaging',
    email: 'Email',
    enterprise: 'Enterprise',
    collaboration: 'Collaboration',
    documents: 'Documents',
    tables: 'Tables',
    workflow: 'Workflow',
    search: 'Search',
    web: 'Web',
    research: 'Research',
    code: 'Code',
    repository: 'Repository',
    issues: 'Issues',
    integration: 'Integration',
    more: '+{count}',
  },
  status: {
    ready: 'Ready',
    reconnectRequired: 'Reconnect required',
    unavailable: 'Unavailable',
    verifying: 'Verifying',
  },
  actions: {
    connect: 'Connect',
    manage: 'Manage',
    viewDetails: 'View details',
    retry: 'Retry',
    backToConnectors: 'Back to Connectors',
    backToCatalog: 'Return to Connector Center',
  },
  detail: {
    about: 'About this connection',
    checkAtUse: 'Checked when used',
    showAll: 'Show all {count} capabilities',
    showLess: 'Show fewer capabilities',
    breadcrumb: 'Connectors',
    pluginNoticeTitle: 'Capability and personal connection are separate',
    pluginNoticeDescription:
      'This plugin is available, but only a verified personal Key creates a connection for you.',
    capabilitiesTitle: 'What you can do',
    capabilitiesDescription: 'Capabilities are verified independently when you connect.',
    capabilityUnavailable: 'Currently unavailable',
    requirementsTitle: 'Key requirements',
    requirementsDescription: 'Use a Key issued for your own external account.',
    examplesTitle: 'Example uses',
    noExamples: 'Examples are not available yet.',
    loadFailedTitle: 'Connector details could not be loaded',
    loadFailedDescription: 'The Connector may be unavailable or you may no longer have access.',
    connectedTitle: 'Personal connection',
    connectedDescription:
      'This Connector is already connected. Manage the verified account without showing its Key.',
    manageConnection: 'Manage connection',
  },
  connections: {
    title: 'My connections',
    description: 'Manage every personal connection separately without displaying its Key.',
    loading: 'Loading your connections',
    loadFailedTitle: 'Connections could not be loaded',
    loadFailedDescription: 'Try again without changing any saved connections.',
    emptyTitle: 'No saved connections',
    emptyDescription: 'Open a Connector to add your first personal connection.',
    count: '{count} connections',
    connectorCount: '{count} services',
    addConnection: 'Add connection',
    manageConnection: 'Manage',
    unknownConnection: 'Unnamed connection',
    unknownIdentity: 'Account details are unavailable',
    updatedAt: 'Updated {date}',
    groupDescription: 'Each connection has its own Key and status.',
  },
  connection: {
    diagnosticDetails: 'Connection details',
    loading: 'Loading personal connection',
    title: '{connector} connection management',
    subtitle: 'Manage your personal connection and where it is used.',
    breadcrumbManage: 'Manage',
    loadFailedTitle: 'Connection could not be loaded',
    loadFailedDescription: 'Try again without changing the connection or any usage locations.',
    notFoundTitle: 'Connection not found',
    notFoundDescription: 'This connection may have been removed or is not owned by your account.',
    connectorFallback: 'Connector',
    banner: {
      applicationDescription:
        'The application is not ready. Check its publication or approval status at the provider.',
      resourceDescription: 'This account cannot access the resource. Check its sharing settings.',
      rateLimitDescription:
        'The provider has temporarily limited requests. Check this connection again later.',

      permissionDescription:
        'Required provider permissions are missing. Check application permissions and resource access.',
      readyTitle: 'Connection successful and ready to use.',
      readyDescription: 'Available in AI Chat, Agents, and Workflows.',
      reconnectTitle: 'This connection needs a new Key.',
      reconnectDescription:
        'This connection needs verification. Update its credentials to reconnect.',
      unavailableTitle: 'This connection is currently unavailable.',
      unavailableDescription:
        'This connection is unavailable. Run diagnostics for the cause and next steps.',
    },
  },
  account: {
    title: 'Connection information',
    externalAccount: '{connector} personal account',
    unknownIdentity: 'Verified personal account',
    connectionName: 'Connection name: {name}',
    createdAt: 'Created',
    lastVerifiedAt: 'Last verified',
    neverVerified: 'Not yet verified',
    viewConnectorDetails: 'View Connector details',
    rename: 'Rename',
    renameTitle: 'Rename connection',
    renameDescription:
      'Use clear names to distinguish connections, including different Keys for the same account.',
    nameLabel: 'Connection name',
    namePlaceholder: 'For example, Work GitHub',
    saveName: 'Save name',
    invalidName: 'Enter a name between 1 and 128 characters.',
  },
  usage: {
    title: 'Usage locations',
    count: '{count}',
    description: 'The same personal connection can be reused in multiple locations.',
    emptyTitle: 'Not used anywhere yet',
    emptyDescription: 'Add this connection from AI Chat, an Agent, or a Workflow.',
    unnamedTarget: 'Unnamed {surface}',
    surfaces: {
      chat: 'AI Chat',
      agent: 'Agent',
      workflow: 'Workflow',
      digitalHuman: 'DigitalHuman',
    },
    status: {
      active: 'In use',
      disabled: 'Disabled',
      needsReplacement: 'Needs replacement',
    },
    view: 'View',
  },
  maintenance: {
    title: 'Connection maintenance',
    updateKeyTitle: 'Update this connection’s Key',
    updateKeyDescription:
      'Replace only this connection’s Key. Its usage locations stay attached; other connections are unchanged.',
    updateAction: 'Update',
    diagnoseTitle: 'Diagnose connection',
    diagnoseDescription: 'Check whether the external account can be reached normally.',
    diagnoseAction: 'Run diagnostics',
    diagnosing: 'Running diagnostics',
    securityNotice: 'For security, the Key is never displayed on this page.',
  },
  reconnect: {
    title: 'Update this connection’s Key',
    description:
      'Enter a replacement Key. The current verified Key stays active unless replacement succeeds.',
    verifyingTitle: 'Verifying replacement Key',
    verifyingDescription: 'Checking the external account and available capabilities.',
    readyTitle: 'Key updated',
    readyDescription: 'The existing connection and its usage locations were preserved.',
    newConnectionTitle: 'A different account was connected',
    newConnectionDescription:
      'A new unbound personal connection was created instead of replacing this connection.',
    confirmIdentityTitle: 'The provider cannot identify this account reliably',
    confirmIdentityDescription:
      'The replacement cannot be matched to the current account. Continue only if you intend to replace this connection, then re-enter the Key.',
    confirmIdentityAction: 'Confirm replacement and re-enter Key',
    confirmedTitle: 'Replacement confirmed',
    confirmedDescription:
      'Submitting the Key now may replace this connection even though the provider cannot verify account identity.',
    viewNewConnection: 'View new connection',
    failedTitle: 'Replacement Key could not be verified',
    retry: 'Enter the Key again',
    submit: 'Update Key',
    cancel: 'Cancel',
    close: 'Close',
  },
  removal: {
    providerAuthorization: 'This does not revoke authorization at the provider.',
    title: 'Disconnect personal connection',
    description: 'Review this connection and the usage locations affected by disconnecting.',
    loading: 'Checking removal impact',
    loadFailedTitle: 'Removal impact could not be loaded',
    loadFailedDescription: 'No connection was removed. Close this dialog and try again.',
    affectedTitle: 'Affected usage locations ({count})',
    affectedDescription: 'These locations will no longer be able to use this connection.',
    safeTitle: 'No usage locations are affected',
    safeDescription: 'This connection can be disconnected normally.',
    forceTitle: 'This connection is still in use',
    forceDescription:
      'Normal disconnect is blocked. Force disconnect only after reviewing the complete list below.',
    tokenUnavailable: 'Force disconnect is unavailable because the impact confirmation expired.',
    disconnectDescription:
      'Disconnecting removes this connection from {count} usage locations and may make related features unavailable.',
    trigger: 'Disconnect',
    normalAction: 'Disconnect connection',
    forceAction: 'Force disconnect',
    cancel: 'Cancel',
  },
  feedback: {
    diagnosed: 'Connection diagnostics completed.',
    diagnoseFailed: 'Connection diagnostics could not be completed.',
    renamed: 'Connection name updated.',
    renameFailed: 'Connection name could not be updated.',
    removed: 'Personal connection disconnected.',
    removeFailed: 'The connection was not disconnected.',
  },
  setup: {
    title: 'Connect your account',
    description: 'Enter the required Key once. ZGI verifies it before creating a connection.',
    addTitle: 'Add {connector} connection',
    createConnectionTitle: 'Create a connection',
    createConnectionDescription: 'Choose the Connector to create in this context.',
    inlineDescription: 'After the connection is created, you will return to the current context.',
    noConnectorsAvailable: 'No Connectors can be created here.',
    addDescription: 'Save a separate connection without changing any existing Key.',
    securityNotice: 'Your Key is write-only and is never shown again after submission.',
    ownerOnly: 'This connection belongs only to you and cannot be used by another member.',
    fieldFallback: 'Key',
    helpLink: 'View official setup instructions',
    submit: 'Verify and save',
    verifyingTitle: 'Connecting…',
    verifyingDescription:
      'Checking the external account and available capabilities. Please do not submit again.',
    readyTitle: 'Connected',
    readyDescription: 'Your personal connection is verified and ready to use.',
    readyAs: 'Verified as {name}',
    nameConnection: 'Name this connection',
    nameAfterReady: 'After verification, you can rename this connection to distinguish its Key.',
    failedTitle: 'Connection could not be verified',
    unknownTitle: 'The result could not be confirmed',
    checkConnections: 'Refresh and check existing connections',
    statusReadFailedTitle: 'Verification status is temporarily unavailable',
    statusReadFailedDescription:
      'The connection may still be processing. Checking again only reads this attempt; it does not resubmit the Key.',
    retryStatus: 'Check status again',
    retry: 'Enter the Key again',
    errors: {
      invalidKey: 'The Key is invalid or has been revoked. Create a new Key and try again.',
      insufficientKey:
        'The Key cannot access the required capabilities. Check its provider access.',
      providerUnavailable: 'The provider is temporarily unavailable. Your Key was not saved.',
      requestFailed:
        'The connection request could not be completed. Check its status before trying again.',
      resultUnknown:
        'No response confirmed the result. A network interruption or cross-origin configuration may be responsible. Refresh the connection list to check its status before deciding whether to enter the Key again. The request will not be resubmitted automatically.',
    },
  },
  serveropsSsh: {
    createTitle: 'Connect an SSH server',
    createDescription:
      'Save one verified connection to a single Linux server. Name and usage stay in Connection Center.',
    reconnectTitle: 'Update this SSH server connection',
    reconnectDescription:
      'Re-enter the credentials for the same server. A matching server identity updates this connection; a different identity is reported instead of silently replacing it.',
    host: 'Server host',
    hostHint: 'Hostname or IP address of one server. One connection serves one server.',
    port: 'SSH port',
    portHint: 'Whole-number TCP port between 1 and 65535. The default is 22.',
    username: 'Username',
    authMethod: 'SSH authentication method',
    authMethodHint:
      'A platform-generated key is the simplest: install its public key once and no password is stored. Use exactly one method.',
    passwordMethod: 'Password login',
    privateKeyMethod: 'SSH private key',
    generatedMethod: 'Generated key (recommended)',
    generatedHint:
      'The platform generated a new key for this connection. Its private half is never shown and is stored encrypted after the test passes. Add the public key below to the server, then test the connection.',
    generatedPublicKey: 'Public key',
    generatedInstall: 'Or run this on the server as the login user:',
    generatedCopy: 'Copy',
    generatedCopied: 'Copied',
    generatedRegenerate: 'Generate another key',
    generating: 'Generating a key…',
    generateFailed: 'This browser cannot generate the key. Use a password or paste a private key instead.',
    password: 'Login password',
    passwordHint:
      'Used only to verify this connection. It is never displayed or returned after saving.',
    privateKey: 'SSH private key',
    privateKeyHint:
      'Paste the PEM or OpenSSH private key for this user. It stays in the form through the test and is cleared only after the final save or when this panel closes.',
    passphrase: 'Private key passphrase (optional)',
    passphraseHint:
      'Leave empty for an unencrypted key. A blank passphrase is omitted and never sent.',
    privilege: 'Command privilege',
    privilegeHint:
      'Service actions (restart, start, stop, reload) need permission to run systemctl on the server. Inspection always runs as the SSH user.',
    privilegeUser: 'Run as the SSH user',
    privilegeSudo: 'Use sudo (no password)',
    privilegeSudoHint:
      'Requires passwordless sudo for systemctl on the server. If sudo asks for a password, the action fails instead of waiting.',
    hostVerification: 'Host identity verification',
    hostVerificationHint:
      'The first test reads the server fingerprint automatically. After confirmation, later connections verify it strictly.',
    showVerificationSettings: 'Advanced settings',
    hideVerificationSettings: 'Hide settings',
    fingerprint: 'Known host key SHA-256 fingerprint (optional)',
    fingerprintHelp:
      'Optional, in the SHA256: form. Leave it empty to let the test read the host key the server presents. If you already have a trusted fingerprint, enter it here and the test strictly verifies it. Host-key verification is never skipped.',
    probe: 'Test server and host key',
    probingTitle: 'Testing the SSH server…',
    probingDescription:
      'Connecting with the submitted credentials to read the server host key. Nothing is saved yet, and the password, private key, and passphrase stay in the form until you confirm.',
    confirmationTitle: 'Confirm this server identity',
    confirmationDescription:
      'The test completed. Review the observed host key, then confirm to save this connection.',
    confirmationTarget: 'Server and user',
    observedHostname: 'Remote hostname',
    observedFingerprint: 'Observed host key SHA-256 fingerprint',
    observedFingerprintUnavailable: 'Not observed; enter a verified fingerprint above.',
    confirmationWarningTitle: 'Check the server identity before saving',
    confirmationWarning:
      'A first connection cannot prove the server identity from the network alone. Check the host, port, and username; compare the fingerprint with a trusted source when one is available. After you confirm, this fingerprint is pinned and a later host-key change will block the connection and require a new test.',
    confirm: 'Confirm and save connection',
    reTest: 'Re-test',
    verifyingTitle: 'Verifying the SSH connection…',
    verifyingDescription:
      'Connecting with the confirmed host key. Credential fields were cleared and are not resubmitted automatically.',
    checkStatus: 'Check status again',
    readyTitle: 'SSH server connected',
    readyDescription: 'The server identity and confirmed host key were verified.',
    newConnectionTitle: 'A different server was connected',
    newConnectionDescription:
      'The submitted identity does not match this connection, so a separate connection was created instead of replacing it. Review it before switching usage.',
    failedTitle: 'SSH connection could not be verified',
    retry: 'Enter credentials again',
    manage: 'Manage this connection',
    externalTitle: 'Create this connection in Connector Center',
    externalDescription:
      'SSH server connections are created and managed only in Connector Center, where the credentials and the pinned host key are verified. Open the connector page there to add or repair this server connection.',
    externalAction: 'Open in Connector Center',
    ownerNotice:
      'This connection belongs only to you. Rename it and review its usage in Connection Center.',
    secretNotice:
      'The password, private key, and passphrase are write-only. They are cleared from the form after the final save or closing and are never written to a URL or browser storage.',
    errors: {
      host: 'Enter one server host with no spaces or surrounding whitespace.',
      port: 'Enter a whole-number TCP port between 1 and 65535.',
      username: 'Enter the SSH username with no surrounding whitespace.',
      password: 'Enter the login password.',
      privateKey: 'Paste a non-empty PEM or OpenSSH private key.',
      passphrase: 'Leave the passphrase empty or enter a single-line value.',
      fingerprint: 'Enter the canonical SHA256: fingerprint without spaces or padding.',
      rejected:
        'The server rejected the connection. Check the username, login password, or private key. If the server was rebuilt or moved, ask its administrator whether the host fingerprint changed. A new host key is never accepted automatically.',
      unreachable:
        'The server could not be reached or authenticated. Check the host, port, network access, and credentials.',
      staleObservation:
        'The server details changed after this host key was observed. Test the server again before saving.',
      fingerprintChanged:
        'The fingerprint changed after this host key was observed. Test the server again and confirm the new fingerprint before saving.',
      resultUnknown:
        'No response confirmed the result. Check the connection list before entering the credentials again. The request will not be resubmitted automatically.',
      requestFailed:
        'The connection request could not be completed. Check its status before trying again.',
    },
  },
  defaultsDraft: {
    loading: 'Loading default connections…',
    loadFailed:
      'Default connections are unavailable. You can choose connectors manually; sending is still available.',
    changed: 'Your new-chat defaults changed. Review and refresh before sending.',
    unavailable:
      'Some selected default connections are unavailable. Repair, choose another connection, or remove them for this chat.',
    retry: 'Retry loading',
    refresh: 'Refresh defaults, keep my changes',
    bypass: 'Do not use connectors this time',
    selectionFailed: 'Review the connector selection before sending.',
  },
  defaults: {
    badge: 'Default',
    newChatDefault: 'New-chat default',
    set: 'Set as new-chat default',
    setShort: 'Set as default',
    clear: 'Clear new-chat default',
    accountActions: 'Default settings for {name}',
    scopeHint:
      'Defaults apply to your new chats in this organization. Switching or removing a connection here does not change them.',
    managementDescription:
      'Use this connection automatically in your future chats in this organization. Existing chats, agents, and workflows are unchanged.',
    saved: 'New-chat default saved.',
    cleared: 'New-chat default cleared. Existing selections are unchanged.',
    saveFailed: 'The default was not saved. Refresh and retry; current selections are unchanged.',
    selectionSavedDefaultFailed:
      'Connection saved, but the default setting could not be saved. Set it again in connection management.',
    retrySave: 'Retry default setting',
    loadFailed:
      'Default settings could not be loaded. You can still select a connection for this target.',
    loading: 'Loading default setting…',
    notDefault: 'Not the new-chat default',
    firstUseTitle: 'Use this connection',
    firstUseDescription:
      'Use {name} in this chat. You can also use it automatically in future chats.',
    firstUseCheckbox: 'Use this connection by default in new chats',
    firstUseConfirm: 'Use connection',
    deletionNotice:
      'This is a new-chat default. Disconnecting clears that default; no replacement account will be selected.',
  },
  picker: {
    authorization: {
      directTitle: 'Authorize direct Connector execution for this target',
      directDescription:
        'Selected operations will not ask again in this target. You can revoke this authorization later.',
      directShort: 'Direct execution',
      askShort: 'Confirm each time',
      modeLabel: 'Authorization mode:',
      addToAgentTitle: 'Add connection to this Agent',
      selectedConnection: 'Selected connection: {connection}',
      required: 'Required',
      agentScopeHint:
        'Direct execution applies only to this Agent and this connection. It is saved with the Agent.',
      confirmAdd: 'Confirm add',
      selectedTitle: 'Selected connections ({count})',
      cancel: 'Cancel',
      confirm: 'Save authorization',
      scopeHint: 'High-risk actions will not ask again for this target and connection.',
      switchToAsk: 'Require confirmation each time',
      switchToDirect: 'Allow direct execution',
      hostDelegationLabel: 'Allow hosts to use:',
      hostDelegationHint:
        'Applies only to the current published version; high-risk operations may still ask for confirmation.',
      hostAudienceWorkspace: 'Workspace',
      hostAudienceApi: 'API',
      hostAudienceChannel: 'Channel',
      hostDelegationUnavailable:
        'This connection is not bound to a published version yet. Save the Agent before delegating it.',
      hostDelegationSaved: 'Host authorization updated.',
      hostDelegationSaveFailed: 'Host authorization was not updated. Refresh and try again.',
    },
    group: {
      selectedCount: '{count} connections selected',
      chooseMultipleForTarget: 'Choose connections for {target}.',
      multipleSubtitle: 'Choose services and connections for {target}.',
      removeConnectionFromTarget: 'Remove {name} from {target}',
      currentLabel: '{connector}: currently using {name}, {status}. Choose a connection.',
      savedLabel: '{connector}: saved reference {name}, {status}. Choose a connection.',
      chooseConnection: 'Choose connection',
      usedInTarget: 'Used in {target}',
      savedCount: '{count} saved connections',
      noSavedConnection: 'No saved connection',
      currentMissing: 'Current connection unavailable',
      savedReference: 'Saved reference',
      serviceUnavailable: 'This connector is unavailable. Replace or remove its saved selection.',
      draftFooter:
        'Your choices will be attached when you send the first message, before the model starts. Saved connections are unchanged.',
      subtitle: 'For {target}; use one connection per service.',
      connectionsLabel: '{connector}: {count} saved connections. Choose a connection.',
      accountsTitle: '{connector} connections',
      chooseForTarget: 'Choose one connection for {target}.',
      currentUnavailable:
        'The current connection is unavailable. Update it, choose another, or remove it from this target.',
      removeFromTarget: 'Remove from {target}',
      footer: 'Switching only affects {target}; saved connections are not deleted.',
    },
    menu: {
      label: 'Connectors for {target}',
      accountCount: '{count} accounts',
      accounts: '{connector} accounts',
      back: 'Back to connectors',
      useInTarget: 'Use in {target}',
      chooseAccountFirst: 'Choose an account below to enable.',
      noAccounts: 'No accounts connected',
      connectAnotherAccount: 'Connect another account',
      manageConnector: 'Manage connector',
      pendingSwitch: 'Switch {current} to {next}?',
      unavailableAccount: 'Unavailable',
      pending: 'Pending',
    },
    title: 'Choose Connectors',
    subtitle: 'Add to {target}',
    setupTitle: 'Connect {connector}',
    backToChoices: 'Back to choices',
    targetTypes: {
      chat: 'this AI Chat',
      agent: 'this Agent',
      workflow: 'this Workflow',
      digital_human: 'this DigitalHuman',
    },
    searchPlaceholder: 'Search Connectors',
    searchAria: 'Search eligible Connectors and Data Sources',
    filtersLabel: 'Filter Connector choices',
    loading: 'Loading Connector choices',
    loadFailedTitle: 'Connector choices could not be loaded',
    loadFailedDescription: 'Try again without changing the current target.',
    emptyTitle: 'No matching Connectors',
    emptyDescription: 'Adjust your search or status filter and try again.',
    connectedAs: 'Connected as {name}',
    states: {
      connected: 'Connected',
      notConnected: 'Not connected',
      inUse: 'In use',
    },
    actions: {
      added: 'Added',
      remove: 'Remove',
      add: 'Add',
      addConnection: 'Add connection',
      switch: 'Switch',
      updateKey: 'Update Key',
      connectAndAdd: 'Connect and add',
      viewReason: 'View reason',
    },
    switchTitle: 'Switch {connector} connection?',
    switchDescription:
      'Replace {current} with {next} for {target}. Both saved connections will remain available.',
    switchConfirm: 'Switch connection',
    switchCancel: 'Cancel',
    connectionReadyNotAdded: 'Connection saved. Choose it to switch this target.',
    workflowConnectionLabel: 'Connection for this node',
    workflowConnectionPlaceholder: 'Choose a personal connection',
    workflowConnectionHint:
      'Each node selects one connection independently. Other nodes and saved connections are unchanged.',
    toolConfigAction: 'Tool settings',
    footer: 'After a successful connection, it will be added to {target}.',
    updating: 'Updating the Connector selection.',
    actionFailed: 'The Connector selection could not be updated. Try again.',
    conversationRequired: 'Start or open a conversation before choosing Connectors.',
    toolbarAction: 'Connectors ({count} added to this conversation)',
    connectionReadyBindingFailed:
      'The connection is ready, but it could not be added to this conversation. Reopen Connectors to try again.',
  },
  runtime: {
    presentation: {
      title: 'Connector operation',
      unavailable: 'The saved operation details cannot be fully displayed.',
      history: 'Historical record · read only',
      truncated: 'The saved summary was shortened.',
      redacted: 'Private operation details are hidden.',
      status: {
        successful: 'Succeeded',
        partial: 'Partially succeeded',
        failed: 'Failed',
        unknown: 'Result unknown',
        cancelled: 'Cancelled',
        unavailable: 'Unavailable',
        pending: 'Awaiting confirmation',
        not_submitted: 'Not submitted',
        in_progress: 'Processing',
      },
      errors: {
        mailSubmissionRejected:
          'The mail server rejected this submission. Check the mail service and try again.',
        mailRecipientRejected:
          'The mail server rejected a recipient. Check the recipients and try again.',
        mailAuthenticationRejected:
          'Email authentication failed. Check the connection credentials and enabled mail services.',
        unknown:
          'The operation result cannot be confirmed yet. Check the provider before retrying.',
        generic: 'The Connector operation did not complete. Check the connection and try again.',
      },
    },
    confirmation: {
      previewEyebrow: 'Security preview',
      previewAccount: 'Account',
      previewTruncated: 'This content preview is shortened.',
      previewUnavailable:
        'This record lacks a verifiable account or operation preview. Cancel it and start a new operation after the connection service is updated.',
      sendMail: 'Send email',

      transportUnconfirmed: 'Result unconfirmed',
      transportUnconfirmedHint:
        'The result has not been confirmed. Check its status before repeating the action.',
      checkStatus: 'Check status',
      statusReadFailed: 'Unable to read the operation status',
      statusReadFailedHint:
        'This does not mean the operation ran or that its result is unknown. Retry the read; the operation will not be resubmitted.',
      retryStatus: 'Retry',
      title: 'Confirm Connector action',
      description:
        'Review this action before it runs. Your decision applies only to this operation.',
      eyebrow: '{risk} Connector operation',
      consequenceFallback: 'This action has an external effect: {effect}.',
      fields: {
        connector: 'Connector',
        action: 'Action',
        target: 'Used in',
        externalEffect: 'External effect',
        dataEgress: 'Sends data externally',
        destination: 'External destination',
      },
      values: {
        yes: 'Yes',
        no: 'No',
      },
      status: {
        required: 'Confirmation required',
        submitting: 'Sending decision',
        confirmed: 'Confirmed',
        cancelled: 'Cancelled',
        completed: 'Completed',
        failed: 'Failed',
        unknown: 'Outcome unknown',
      },
      unclassifiedTitle: 'The platform cannot classify this operation',
      unclassifiedDescription:
        'Treat it as consequential and confirm only after reviewing the action and target.',
      dataEgressTitle: 'Data leaves ZGI',
      dataEgressDescription: 'This action sends data to an external service.',
      dataEgressDestination: 'This action sends data to {destination}.',
      confirm: 'Confirm and continue',
      cancel: 'Cancel action',
      completedTitle: 'Connector action completed',
      completedDescription:
        'The external operation completed. This one-shot confirmation is now closed.',
      cancelledTitle: 'Connector action cancelled',
      cancelledDescription:
        'The operation was cancelled before submission. No external action was sent.',
      operationFailed:
        'The external operation failed and will not be submitted again automatically.',
      unknownTitle: 'The provider outcome could not be verified',
      unknownDescription:
        'The execution result is unknown. Check the provider status before repeating the action.',
    },
    blocked: {
      title: 'Connector action blocked',
      description: 'This run cannot continue without an interactive confirmation.',
      status: 'Blocked',
      reasonTitle: 'Confirmation is required',
      reasonDescription:
        'Run this operation in an interactive Chat, Agent, or Workflow surface to review it. No external action was submitted.',
    },
    risk: {
      high: 'High risk',
      unclassified: 'Unclassified',
    },
    targetTypes: {
      chat: 'AI Chat',
      agent: 'Agent',
      workflow: 'Workflow',
      digital_human: 'DigitalHuman',
    },
    effects: {
      read: 'Read external data',
      create: 'Create external data',
      update: 'Update external data',
      delete: 'Delete external data',
      publish: 'Publish externally',
      invoke: 'Invoke an external operation',
      schedule: 'Schedule an external operation',
      externalSend: 'Send to an external destination',
      unspecified: 'Unspecified external effect',
    },
  },
  legacyBinding: {
    readOnly: 'Legacy · Read-only',
    description: 'This existing binding is preserved and remains managed in External Connections.',
    manage: 'Manage legacy',
    detach: 'Detach',
  },
};

export default messages;
export type ConnectorMessages = typeof messages;
