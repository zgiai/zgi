const messages = {
  page: {
    eyebrow: 'Developer / Content parse',
    title: 'File recognition playground',
    description:
      'Upload a file to inspect parsing output, bounding-box alignment, and canonical intermediate results. Business modules consume one normalized output while provider details stay behind policy and admin configuration.',
  },
  status: {
    parsed: 'Parsed',
    parsing: 'Parsing',
    ready: 'Ready',
  },
  providerLabel: 'Provider',
  advanced: {
    open: 'Advanced settings',
    close: 'Hide settings',
    profileLabel: 'Parse strategy',
    ocrLabel: 'Local OCR',
    providerCompareHint:
      'Choose a provider to compare vendor output. Unconfigured providers stay visible but cannot be selected.',
  },
  providerStatus: {
    label: 'Status',
    available: 'Available',
    fallback: 'Fallback',
    notConfigured: 'Not configured',
    unavailable: 'Unavailable',
    unknown: 'Unknown',
    sourceDefaultCatalog: 'Default catalog',
    loading: 'Checking providers',
  },
  providers: {
    auto: {
      label: 'Automatic route',
      hint: 'Choose by policy',
      explanation:
        'The system chooses the best configured provider first, then falls back to the local chain when needed.',
    },
    local: {
      label: 'Local rules',
      hint: 'Self-hosted fallback chain',
      explanation:
        'Uses the built-in local engine: document structure parsing, OCR, layout rules, and optional local visual-model fallback. It does not require a third-party parsing service.',
    },
    vlm: {
      label: 'Visual model',
      hint: 'Vision-language enhancement',
      explanation:
        'Uses a configured visual model to read image-like or scanned content when rules and OCR are not enough.',
    },
    reducto: {
      label: 'Reducto',
      hint: 'Third-party parser',
      explanation:
        'Uses the administrator-configured Reducto service and maps its result back to the unified canonical output.',
    },
    mineru: {
      label: 'MinerU',
      hint: 'Layout and OCR parser',
      explanation:
        'Uses the configured MinerU provider for layout-aware document parsing and OCR-heavy files.',
    },
    hyperparseApi: {
      label: 'Hyperparse API',
      hint: 'Remote compatibility endpoint',
      explanation:
        'Uses a remote Hyperparse-compatible API and maps its response into the same canonical output.',
    },
  },
  profiles: {
    auto: 'Smart strategy',
    highQuality: 'Deep parse',
    layoutFirst: 'Layout first',
    fast: 'Speed first',
    localFirst: 'Local first',
    descriptions: {
      auto: 'The system chooses by file type, configured providers, and health.',
      highQuality:
        'Enables more aggressive layout, table, OCR, and visual enhancement for scans and complex layouts.',
      layoutFirst:
        'Prioritizes layout structure and bounding boxes for tables, contracts, and reports.',
      fast: 'Reduces enhancement and retries for quick previews or coarse batch tests.',
      localFirst: 'Prefers the local fallback chain for privacy and offline capability tests.',
    },
  },
  ocr: {
    auto: 'Auto select',
    tesseract: 'Tesseract',
    paddleocr: 'PaddleOCR',
    notApplicable: 'Only applies to local rules or auto route when local is selected',
    descriptions: {
      auto: 'Uses the current server-side default OCR configuration.',
      tesseract: 'Calls the local tesseract command.',
      paddleocr: 'Calls the local paddleocr command; disabled when it is not installed.',
    },
  },
  actions: {
    run: 'Run',
    rerun: 'Rerun',
    copy: 'Copy',
    save: 'Save record',
    saved: 'Saved',
    share: 'Share',
    history: 'History',
    compare: 'Same-file compare',
    health: 'Health/cost',
  },
  history: {
    title: 'Saved records',
    description: 'Only explicitly saved parse runs appear here. Running a parse does not persist by default.',
    loading: 'Loading records',
    empty: 'No saved parse records yet.',
  },
  compare: {
    title: 'Same-file hash comparison',
    description: 'Compare the same sha256 file across providers, strategies, or OCR settings.',
    empty: 'No saved comparison records for this file yet.',
  },
  providerSummary: {
    title: 'Provider health/cost summary',
    description:
      'Aggregated from recently saved playground records. Cost is a placeholder until admin pricing configuration is connected.',
    loading: 'Loading provider summary',
    empty: 'No saved records to summarize yet.',
    runs: '{count} runs',
    success: 'Success',
    degraded: 'Degraded',
    failed: 'Failed',
    fallback: 'Fallback',
    avgTime: 'Avg time',
    avgText: 'Avg text',
    cost: 'Cost',
  },
  upload: {
    title: 'Upload or drop a file',
    subtitle: 'PDF / image / Office / Markdown / text, up to 64 MB',
  },
  output: {
    title: 'Parsing output',
    description: 'Route plan, bounding boxes, text blocks, and chunk preview',
    tabs: {
      blocks: 'Blocks',
      markdown: 'Markdown',
      json: 'JSON',
      route: 'Route',
    },
  },
  metrics: {
    quality: 'Quality',
    bbox: 'Boxes',
    engine: 'Engine',
    time: 'Time',
    rendering: 'rendering',
    noRun: 'No parse run yet',
    pages: '{count} pages',
    markdownChars: '{count} Markdown chars',
    reliableRatio: '{count}% reliable boxes',
  },
  qualityLevels: {
    high: 'High',
    standard: 'Standard',
    degraded: 'Degraded',
    failed: 'Failed',
  },
  help: {
    detailsToggle: 'Details',
    providerTitle: 'Current provider',
    qualityTitle: 'Quality means',
    providerStatusDescription:
      'Provider status comes from the current capability catalog and adapter health. Unconfigured providers stay visible for planning, but cannot be selected until an administrator configures them.',
    qualityDescription:
      'Quality is a coarse health label for this parse result. It combines parse status, fallback/degraded signals, text output, and reliable bounding-box coverage. Use it for debugging and comparison, not as a final business score.',
  },
  empty: {
    previewTitle: 'Select a file to start',
    previewDescription:
      'PDFs and images show original pages. Office and text files use a layout-only page after parsing when boxes are available.',
    blocks: 'Run parsing to list canonical elements here.',
    markdown: 'Parsed Markdown will appear here.',
    json: 'Normalized artifact JSON will appear here.',
    route: 'Provider-policy route, chunk plan, and quality summary will appear here.',
  },
  preview: {
    title: 'Document preview',
    renderFailed:
      'Preview rendering failed, but parsing can still run. After parsing, layout-only pages can still show available boxes.',
    sourceUnavailable:
      'This saved run does not include the original file, so only layout placeholders and available boxes can be shown.',
    rendering: 'Rendering preview',
    page: 'Page {page}',
    layoutPage: 'Layout page {page}',
    imagePreview: 'Image preview',
    boxes: '{count} boxes',
  },
  element: {
    pageShort: 'p{page}',
    noText: 'No text content',
    types: {
      title: 'Title',
      heading: 'Heading',
      text: 'Text',
      paragraph: 'Paragraph',
      table: 'Table',
      figure: 'Figure',
      image: 'Image',
      formula: 'Formula',
      list: 'List',
      listItem: 'List item',
      code: 'Code',
      element: 'Element',
    },
    precision: {
      reliable: 'Reliable',
      unreliable: 'Unreliable',
      estimated: 'Estimated',
    },
  },
  toast: {
    selectFile: 'Select a test file first',
    parseDone: 'Parsing finished',
    parseFailed: 'Parsing failed',
    copied: 'Copied',
    saveRequiresResult: 'Run parsing before saving a record',
    saveDone: 'Record saved',
    saveFailed: 'Failed to save record',
    saveBeforeShare: 'Save a record before copying a share link',
    shareCopied: 'Share link copied',
    shareLoaded: 'Shared record loaded',
    shareLoadFailed: 'Failed to load shared record',
    historyLoadFailed: 'Failed to load history',
    compareRequiresHash: 'Current result has no file hash',
    compareLoadFailed: 'Failed to load same-file comparison',
    providerSummaryLoadFailed: 'Failed to load provider summary',
  },
};

export default messages;
export type ContentParseMessages = typeof messages;
