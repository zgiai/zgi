const messages = {
  title: 'Prompts',
  description:
    'Manage reusable prompts, test them in the playground, then copy them into agents or workflow nodes for editing.',
  search: {
    placeholder: 'Search prompts',
  },
  tabs: {
    library: 'Library',
    libraryDescription:
      'Browse official, workspace, and personal prompt versions. Selecting one copies it into the current use case.',
    playground: 'Playground',
  },
  library: {
    currentVersion: 'Current v{version}',
    latestVersionLabel: 'Latest version',
    onlineVersionLabel: 'Online version',
    onlineVersionUnset: 'Not set',
    singleVersionLabel: 'Current version',
    singleVersionDescription:
      'There is only one version. Future edits are saved as new versions, so teams can review history and choose a stable version.',
    onlineSameAsLatest: 'Latest is online',
    latestNotOnline: 'Latest is not online',
    notPublished: 'Not published',
    officialHint: 'Official template, copy to edit',
    reusableHint: 'Copy into a node and edit',
  },
  localeOptions: {
    zhHans: 'Simplified Chinese',
    enUS: 'English',
    jaJP: 'Japanese',
    unknown: 'Other language',
  },
  promptTypes: {
    text: 'Text prompt',
    chat: 'Chat message prompt',
  },
  sources: {
    official: 'Official',
    workspace: 'Workspace',
    personal: 'My prompts',
  },
  fields: {
    name: 'Name',
    slug: 'Slug',
    description: 'Description',
    locale: 'Language',
    source: 'Source',
    category: 'Category',
    tags: 'Tags',
    promptType: 'Prompt type',
    content: 'Content',
    commitMessage: 'Commit message',
    workspace: 'Workspace',
    model: 'Model',
  },
  actions: {
    newPrompt: 'New prompt',
    optimizePrompt: 'Optimize prompt',
    testInPlayground: 'Test in playground',
    openPrompt: 'Open',
    retry: 'Retry',
    clearSearch: 'Clear search',
    newVersion: 'New version',
    create: 'Create',
    createVersion: 'Create version',
    editContent: 'Edit content',
    cancel: 'Cancel',
    moreOptions: 'More options',
    lessOptions: 'Fewer options',
    back: 'Back to prompts',
    saveMeta: 'Save details',
    editDetails: 'Edit details',
    copyLink: 'Copy link',
    copyAsPersonal: 'Copy and edit myself',
    shareToWorkspace: 'Share to workspace',
    optimizeAsPersonal: 'Optimize with AI, then save',
  },
  compare: {
    trigger: 'Compare versions',
    title: 'Compare prompt versions',
    description: 'Review two prompt versions side by side before deciding what to keep or publish.',
    leftVersion: 'Left version',
    rightVersion: 'Right version',
  },
  detail: {
    currentVersion: 'Current version',
    atAGlance: 'At a glance',
    atAGlanceDescription:
      'Check the language, source, and version status. Test it in the playground, or edit the content when changes are needed.',
    versionCount: 'Versions',
    latestVersionBehavior:
      'Saving content creates a new version and makes it the Latest version. Content already copied into agents or workflow nodes does not change automatically; choose the template again or update the node manually.',
    releaseStatus: 'Release status',
    releaseStatusDescription:
      'The Online version marks the team-validated recommended version. After copying it into a node, the node remains editable.',
    singleVersionReleaseDescription:
      'There is only one version. Test it first; when the team should reuse it long term, mark it as the Online version.',
    latestTarget: 'Latest version',
    onlineTarget: 'Online version',
    unsetTarget: 'Not set',
    makeCurrentOnline: 'Set current as online',
    currentOnline: 'Current version is online',
    makeVersionOnline: 'Set as online version',
    versionOnline: 'Already online',
    releaseImpactCurrent:
      'The latest version is also the Online version. New template selection will show this stable version marker.',
    releaseImpactPending:
      'The latest version {latestVersion} is not online yet. The team-recommended Online version is still {onlineVersion}.',
    releaseImpactUnset:
      'No Online version has been set yet. Mark a validated version as Online so teammates can choose the stable template.',
    publishOnlineConfirmTitle: 'Set as Online version?',
    publishOnlineConfirmDescription:
      'This switches the team-recommended Online version to {version}. Content already copied into nodes will not change automatically; only legacy managed references are affected on their next run.',
    publishOnlineConfirmImpactWithReferences:
      '{count} legacy managed node(s) currently follow the Online version. After publishing, those nodes will use the new version on their next run.',
    publishOnlineConfirmImpactNoReferences:
      'No legacy managed nodes currently follow the Online version, so publishing will not immediately affect existing workflows.',
    publishOnlineConfirmImpactUnknown:
      'Legacy managed reference impact cannot be confirmed yet. Wait for impact information to finish loading before publishing.',
    publishOnlineConfirmAction: 'Publish {version}',
    assetImpact: 'Legacy references',
    assetImpactDescription:
      'Shows only older managed references and runtime evidence. New workflow selection copies template content into the node.',
    assetHealthRiskTitle: 'Version risk detected',
    assetHealthRiskDescription:
      '{count} legacy managed node(s) still follow Latest. If they serve production traffic, migrate them to node copies or a stable version.',
    assetHealthRiskBadge: 'Needs action',
    assetHealthActiveTitle: 'Legacy managed references found',
    assetHealthActiveDescription:
      'This prompt has older workflow references and recent runtime evidence. Keep monitoring or migrate them to node copies.',
    assetHealthActiveBadge: 'Running',
    assetHealthLinkedTitle: 'Legacy references, no recent runs',
    assetHealthLinkedDescription:
      'Older workflow nodes reference this prompt, but no recent runtime evidence has been recorded yet.',
    assetHealthLinkedBadge: 'Needs verification',
    referenceTarget: 'Reference target: {target}',
    referenceLatestRisk: 'Latest risk',
    latestReferenceWarning:
      '{count} legacy managed node(s) still follow Latest. If they serve production traffic, migrate them to node copies or a stable version so draft iterations do not enter production automatically.',
    moreReferences: '{count} more reference(s) not expanded',
    lastRunAt: 'Last run: {time}',
    previousVersions: 'Previous versions',
    previousVersionsDescription:
      'Preview one selected version by default so long prompts do not take over the page.',
    previousVersionsCount: '{count} previous versions',
    versionPreview: '{version} preview',
    selectVersionPreview: 'Preview {version}',
    noVersionLabels: 'No version labels',
    moreInformation: 'Work record',
    moreInformationDescription:
      'Review related templates and optimization runs. Prompt settings live in Edit details.',
    metadata: 'Metadata',
    metadataDescription:
      'These details help with search, organization, and sharing. They do not change the current prompt version content.',
    officialReadOnlyNote:
      'Official templates are read-only. Copy this to your prompts before editing, versioning, or saving optimization results.',
    personalCopyName: '{name} Copy',
    optimizedCopyName: '{name} Optimized',
    personalCopyCommitMessage: 'Created personal copy from official template',
    optimizedCopyCommitMessage: 'Optimized official template into a personal copy',
    optimizedVersionCommitMessage: 'Created version from optimization result',
    editVersionCommitMessage: 'Edited content and saved as a new version',
  },
  form: {
    createTitle: 'Create prompt',
    simpleHint:
      'Start with just a name and prompt content. You can edit it later by saving a new version.',
    invalidChatJson:
      'Chat prompt content must be a valid JSON array. Check brackets, quotes, and commas.',
  },
  versions: {
    createTitle: 'Create prompt version',
    editTitle: 'Edit and save as new version',
    editDescription:
      'Saving creates a new version. It does not overwrite the current version. The prompt library shows the new version by default; content already copied into agents or workflows will not change automatically.',
    saveAsVersion: 'Save as new version',
    governanceTitle: 'Version impact',
    currentLatest: 'Current latest',
    currentOnline: 'Current online',
    afterSave: 'After save',
    governanceNote:
      'After saving, the new version becomes Latest but does not automatically become the Online version. Validate it first, then set it Online manually.',
  },
  placeholders: {
    name: 'Customer Support Reply',
    slug: 'team/customer-support-reply',
    description: 'What this prompt is for',
    category: 'customer-support',
    tags: 'support, reply, tone',
    textContent: 'Enter text prompt content',
    chatContent: 'Enter JSON array of chat messages',
    commitMessage: 'Summarize what changed in this version',
  },
  picker: {
    title: 'Select from prompt library',
    searchPlaceholder: 'Search prompt library',
    previewPlaceholder: 'Select a prompt to preview its latest version.',
    applyEditableCopy: 'Apply and keep editing',
    latestVersionShort: 'Latest {version}',
    onlineVersionShort: 'Online {version}',
    onlineVersionUnsetShort: 'Not online',
    version: 'Version',
    copyModeDescription:
      'The selected version will be copied into the current node so you can keep editing it without changing the original prompt in the library.',
    releaseLabels: {
      production: 'Online version',
      latest: 'Latest version',
      staging: 'Test version',
      grayA: 'Gray A',
      grayB: 'Gray B',
    },
    replaceWarningTitle: 'Copy to this node?',
    replaceWarningDescriptionCopy:
      'The selected version will become editable content in this node and replace the current system prompt.',
    replaceWarningSaveHintCopy:
      'This only changes the current node draft. Later prompt library updates will not sync automatically. Save the workflow for it to take effect.',
    replaceWarningConfirm: 'Copy and apply',
  },
  optimizer: {
    title: 'Optimize prompt',
    description:
      'Use the built-in optimizer to rewrite a raw prompt into stronger, production-ready prompt variants.',
    inputPanelLabel: 'Input & settings',
    inputPanelDescription: 'Review the prompt, edit request, and optimization settings in order.',
    settingsPanelLabel: 'Optimization settings',
    settingsPanelDescription:
      'Choose the model, target, length guardrail, and variable protection.',
    waitingTitle: 'Waiting for optimization',
    errorStateTitle: 'Optimization did not finish',
    errorStateDescription: 'Adjust the input or settings, then start optimization again.',
    goalLabel: 'Optimization goal',
    sourceLabel: 'Raw prompt',
    sourceHelpDescription:
      'Paste the prompt you want to improve here, choose a goal, then start optimization.',
    prefilledSourceDescription:
      'If content is already loaded here, you can edit it directly before running the optimizer.',
    resetSource: 'Restore original',
    sourcePlaceholder: 'Enter the raw prompt you want to optimize',
    preserveVariablesLabel: 'Protect variables',
    preserveVariablesDescription:
      'Keep detected placeholders exactly unchanged in the optimized result.',
    modelLabel: 'Optimization model',
    modelDescription:
      'Choose a model from your system-available text chat models, or leave it on the default choice.',
    fixedModelDescription:
      'This uses the same model selected on the current node so optimization stays consistent with the workflow configuration.',
    modelUnavailable: 'No model is available on the current node yet',
    detectedVariablesLabel: 'Detected variables',
    noVariables: 'No variables detected yet',
    outputLabel: 'Optimization results',
    variantHint:
      'You will get one stronger result. If it is not ideal, adjust the goal or run it again.',
    copy: 'Copy result',
    copyPartial: 'Copy partial result',
    apply: 'Apply result',
    saveAsPersonalPrompt: 'Save to my prompts',
    saveAsNewVersion: 'Save as new version',
    officialTemplateHelp:
      'Official templates are read-only. Optimize the content here, then save the result as your own prompt copy.',
    run: 'Optimize now',
    rerun: 'Retry optimization',
    running: 'Optimizing...',
    runningDescription: 'Generating a better prompt with the hidden optimizer...',
    emptyState: 'Enter a raw prompt, choose an optimization goal, then start optimization.',
    editInstructionLabel: 'Edit request',
    editInstructionDescription:
      'Optional. Tell the AI what to strengthen, shorten, preserve, or change this time.',
    editInstructionPlaceholder:
      'For example: only strengthen tool-use rules, keep the existing structure, make the tone more formal, shorten to 800 characters',
    targetMaxChars: 'Target result: no more than {count} characters',
    truncatedWarning:
      'The optimization result was cut off by the model output limit. Shorten the edit request or retry optimization; truncated results cannot be applied.',
    progress: {
      title: 'Optimization progress',
      steps: {
        analyze: 'Analyzing the raw prompt',
        variables: 'Checking variables and constraints',
        rewrite: 'Rewriting the prompt',
        polish: 'Polishing the final result',
      },
    },
    goals: {
      general: {
        label: 'Quick optimize',
        description: 'Improve clarity, completeness, and overall usability quickly.',
      },
      reliable: {
        label: 'More reliable',
        description: 'Reduce ambiguity and make the prompt more stable and consistent.',
      },
      structured: {
        label: 'More structured',
        description: 'Push the prompt toward clearer sections, steps, and formatted output.',
      },
      deep: {
        label: 'Deep optimize',
        description:
          'Perform a more complete, higher-quality rewrite for prompts that matter more or need stronger results.',
      },
    },
  },
  states: {
    loading: 'Loading prompts...',
    empty: 'No prompts yet',
    emptyTitle: 'Start from a tested prompt',
    emptyDescription:
      'Create a managed prompt, optimize a rough draft, or open the playground to validate behavior before publishing it.',
    emptySearchTitle: 'No prompts match this search',
    emptySearchDescription:
      'Try a different keyword, clear the search, or create a new prompt from the draft you have in mind.',
    loadFailedTitle: 'Prompt library could not load',
    loadFailedDescription:
      'Check the workspace connection or API service, then retry without leaving this page.',
    noDescription: 'No description',
    accessDeniedTitle: 'Access denied',
    accessDeniedDescription: 'You do not have permission to view the prompt library.',
  },
  relatedTemplates: {
    title: 'Related templates',
    openInGallery: 'Open template gallery',
  },
  shareToWorkspaceConfirm: {
    title: 'Share this prompt with the workspace?',
    description:
      'Workspace members with access can discover and reuse this prompt. Keep it personal if it is still a private draft.',
  },
  usage: {
    metrics: {
      linkedNodes: 'Linked nodes',
      totalRuns: 'Recent runs',
    },
    emptyReferences: 'This prompt is not currently referenced by any visible workflow nodes.',
    emptyRuns: 'No runtime evidence has been recorded for this prompt yet.',
    referenceModes: {
      managed: 'Managed prompt',
    },
  },
  history: {
    title: 'My optimization history',
    description:
      'Recent optimization runs are kept as your private working history. You can review the result and adopt it into a formal prompt version.',
    empty: 'No optimization history yet',
    variablesProtected: 'Variables protected',
    adopted: 'Adopted',
    adoptedVersion: 'Adopted as v{version}',
    copyCurrent: 'Copy current',
    adoptCurrent: 'Save current as version',
    retryCurrent: 'Retry with same settings',
  },
  playground: {
    title: 'Prompt playground',
    description:
      'Test a prompt directly with a model before putting it into a workflow or saving it as a formal asset.',
    runStatusReady: 'Ready',
    runStatusBlocked: 'Needs input',
    inputsTitle: 'Playground inputs',
    prefilledFrom: 'Loaded from prompt:',
    choosePrompt: 'Choose prompt',
    changePrompt: 'Change prompt',
    useSelectedPrompt: 'Use in playground',
    modelLabel: 'Model',
    promptLabel: 'Prompt',
    advancedTitle: 'Advanced settings',
    advancedDescription:
      'Only expand this section when you want to fine-tune the raw prompt or fill extra variables manually.',
    detailsTitle: 'View actual prompt payload',
    messagesLabel: 'Prompt messages',
    renderedMessagesLabel: 'Rendered messages',
    promptPlaceholder: 'Enter the prompt you want to test',
    inputLabel: 'Test input',
    inputPlaceholder: 'Paste a realistic user request or test payload',
    readinessReadyDescription:
      'The prompt, model, and required input are ready. Run once, then inspect the feedback.',
    variablesTitle: 'Variables',
    missingPromptHint:
      'Open Advanced settings first, enter the prompt you want to test, then run it.',
    missingInputHint:
      'This prompt uses the test input field. Enter any sample input above before running.',
    missingVariablesHint: 'You still need to fill these extra variables:',
    progressTitle: 'Run progress',
    progress: {
      steps: {
        analyze: 'Preparing the prompt payload',
        rewrite: 'Calling the selected model',
        polish: 'Showing the model result',
      },
    },
    outputTitle: 'Feedback',
    feedbackEmptyDescription:
      'Start a run to turn the model response into actionable prompt feedback.',
    feedbackReadyDescription:
      'Use this feedback to revise the prompt, then run again with a sharper sample.',
    renderedPromptLabel: 'Rendered prompt',
    resultLabel: 'Model feedback',
    resultPlaceholder: 'Run the playground to see feedback here',
    editPromptAction: 'Edit content',
    optimizePromptAction: 'Optimize prompt',
    copyOutput: 'Copy',
    runErrorTitle: 'Run failed',
    showErrorDetails: 'View error details',
    run: 'Run prompt',
    running: 'Running...',
    runFailed: 'Failed to run prompt playground',
    messageRoles: {
      system: 'System',
      user: 'User',
      assistant: 'Assistant',
    },
  },
  messages: {
    createSuccess: 'Prompt created',
    createFailed: 'Failed to create prompt',
    updateSuccess: 'Prompt updated',
    updateFailed: 'Failed to update prompt',
    versionCreateSuccess: 'Prompt version created',
    versionCreateFailed: 'Failed to create prompt version',
    labelUpdateSuccess: 'Online version updated',
    labelUpdateFailed: 'Failed to update Online version',
    shareCopied: 'Prompt link copied',
    shareCopyFailed: 'Failed to copy prompt link',
    shareToWorkspaceSuccess: 'Prompt shared to workspace',
    shareToWorkspaceFailed: 'Failed to share prompt to workspace',
    copyAsPersonalSuccess: 'Copied to my prompts',
    optimizedCopyAsPersonalSuccess: 'Optimized result saved to my prompts',
    copyAsPersonalFailed:
      'Failed to copy this prompt. Check that the current workspace is available.',
    currentModelFallback: 'Current model',
    providerBillingIssue:
      '{model} is currently unavailable. Check the provider balance/quota status or switch to another model.',
    providerBillingHintAdmin:
      'Go to channel management to check and top up the model quota, or switch to another available model for now.',
    providerBillingHintMember:
      'The current model channel is out of quota. Contact your administrator to top up the model quota, or switch to another available model.',
    providerAccessDenied:
      '{model} is currently unavailable. Check the provider access status or switch to another model.',
    providerAccessHintAdmin:
      'Go to channel management to review the model channel access configuration, or switch to another available model for now.',
    providerAccessHintMember:
      'The current model channel does not have usable access. Contact your administrator to review the channel configuration, or switch to another available model.',
    providerActionSwitchModel: 'Switch model',
    providerActionManageChannels: 'Open channel management',
    playgroundOutputCopied: 'Playground feedback copied',
    playgroundOutputCopyFailed: 'Failed to copy playground feedback',
    optimizerCopied: 'Optimized prompt copied',
    optimizerCopyFailed: 'Failed to copy optimized prompt',
    optimizerRunFailed: 'Failed to optimize prompt',
    optimizerAdoptSuccess: 'Optimization result saved as a new version',
    optimizerAdoptFailed: 'Failed to save optimization result as a new version',
  },
};

export default messages;
