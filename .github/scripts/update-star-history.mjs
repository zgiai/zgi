import { mkdir, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";

const token = process.env.GITHUB_TOKEN;
const repository = process.env.GITHUB_REPOSITORY;
const outputDir = process.env.STAR_HISTORY_OUTPUT_DIR ?? "assets";

if (!token) throw new Error("GITHUB_TOKEN is required");
if (!repository || !/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repository)) {
  throw new Error("GITHUB_REPOSITORY must use the owner/repository format");
}

const [owner, name] = repository.split("/");
const query = `
  query StarHistory($owner: String!, $name: String!, $cursor: String) {
    repository(owner: $owner, name: $name) {
      createdAt
      nameWithOwner
      stargazerCount
      stargazers(
        first: 100
        after: $cursor
        orderBy: { field: STARRED_AT, direction: ASC }
      ) {
        edges { starredAt }
        pageInfo { endCursor hasNextPage }
      }
    }
  }
`;

async function requestPage(cursor) {
  const response = await fetch("https://api.github.com/graphql", {
    method: "POST",
    headers: {
      Accept: "application/vnd.github+json",
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
      "User-Agent": "zgi-star-history",
      "X-GitHub-Api-Version": "2022-11-28",
    },
    signal: AbortSignal.timeout(20_000),
    body: JSON.stringify({ query, variables: { owner, name, cursor } }),
  });

  if (!response.ok) {
    throw new Error(`GitHub GraphQL request failed with HTTP ${response.status}`);
  }

  const payload = await response.json();
  if (payload.errors?.length) {
    throw new Error(payload.errors.map(({ message }) => message).join("; "));
  }
  if (!payload.data?.repository) {
    throw new Error(`Repository ${repository} was not found or is not accessible`);
  }

  return payload.data.repository;
}

async function fetchHistoryOnce() {
  const starredAt = [];
  let cursor = null;
  let metadata;

  do {
    const page = await requestPage(cursor);
    metadata ??= {
      createdAt: page.createdAt,
      nameWithOwner: page.nameWithOwner,
      stargazerCount: page.stargazerCount,
    };
    starredAt.push(...page.stargazers.edges.map((edge) => edge.starredAt));
    cursor = page.stargazers.pageInfo.hasNextPage
      ? page.stargazers.pageInfo.endCursor
      : null;
  } while (cursor);

  starredAt.sort((left, right) => Date.parse(left) - Date.parse(right));
  return { ...metadata, starredAt };
}

async function fetchHistory() {
  for (let attempt = 1; attempt <= 2; attempt += 1) {
    const history = await fetchHistoryOnce();
    if (history.stargazerCount === history.starredAt.length) return history;
    if (attempt === 1) {
      console.warn("Star count changed while reading history; retrying once");
      continue;
    }
    throw new Error(
      `Star history is inconsistent: expected ${history.stargazerCount}, received ${history.starredAt.length}`,
    );
  }
  throw new Error("Unable to read Star History");
}

const DAY = 86_400_000;

function utcDayEnd(timestamp) {
  const date = new Date(timestamp);
  return Date.UTC(
    date.getUTCFullYear(),
    date.getUTCMonth(),
    date.getUTCDate(),
    23,
    59,
    59,
    999,
  );
}

function buildPoints(createdAt, starredAt, now) {
  let start = Date.parse(createdAt);
  if (!Number.isFinite(start) || start >= now) start = now - DAY;

  const daily = new Map();
  starredAt.forEach((value, index) => {
    daily.set(utcDayEnd(Date.parse(value)), index + 1);
  });

  const points = [{ time: start, count: 0 }];
  for (const [time, count] of daily) {
    if (time > start && time < now) points.push({ time, count });
  }
  points.push({ time: now, count: starredAt.length });
  return points;
}

function niceMaximum(value) {
  if (value <= 1) return 1;
  const power = 10 ** Math.floor(Math.log10(value));
  const fraction = value / power;
  const rounded = fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 5 ? 5 : 10;
  return rounded * power;
}

function formatDate(timestamp) {
  return new Date(timestamp).toISOString().slice(0, 10);
}

function escapeXml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&apos;");
}

function renderSvg({ nameWithOwner, createdAt, starredAt }, theme, now) {
  const colors = theme === "dark"
    ? {
        background: "#0d1117",
        border: "#30363d",
        grid: "#30363d",
        text: "#f0f6fc",
        muted: "#8b949e",
        accent: "#58a6ff",
        area: "#1f6feb",
      }
    : {
        background: "#ffffff",
        border: "#d0d7de",
        grid: "#d8dee4",
        text: "#24292f",
        muted: "#57606a",
        accent: "#0969da",
        area: "#54aeff",
      };

  const width = 900;
  const height = 460;
  const plot = { left: 72, top: 104, right: 868, bottom: 398 };
  const points = buildPoints(createdAt, starredAt, now);
  const start = points[0].time;
  const end = points.at(-1).time;
  const yMaximum = niceMaximum(starredAt.length);
  const x = (time) => plot.left + ((time - start) / (end - start)) * (plot.right - plot.left);
  const y = (count) => plot.bottom - (count / yMaximum) * (plot.bottom - plot.top);

  let linePath = `M ${x(points[0].time).toFixed(2)} ${y(points[0].count).toFixed(2)}`;
  for (const point of points.slice(1)) {
    linePath += ` H ${x(point.time).toFixed(2)} V ${y(point.count).toFixed(2)}`;
  }
  const areaPath = `${linePath} L ${plot.right} ${plot.bottom} L ${plot.left} ${plot.bottom} Z`;

  const yTicks = yMaximum <= 5
    ? Array.from({ length: yMaximum + 1 }, (_, index) => index)
    : Array.from({ length: 6 }, (_, index) => (yMaximum / 5) * index);
  const xTicks = Array.from({ length: 5 }, (_, index) => start + ((end - start) * index) / 4);

  const yGrid = yTicks.map((tick) => {
    const position = y(tick).toFixed(2);
    return `
      <line x1="${plot.left}" y1="${position}" x2="${plot.right}" y2="${position}" stroke="${colors.grid}" stroke-width="1" />
      <text x="${plot.left - 14}" y="${Number(position) + 4}" text-anchor="end" fill="${colors.muted}" font-size="12">${tick}</text>`;
  }).join("");

  const xGrid = xTicks.map((tick, index) => {
    const position = x(tick).toFixed(2);
    const anchor = index === 0 ? "start" : index === xTicks.length - 1 ? "end" : "middle";
    return `
      <line x1="${position}" y1="${plot.top}" x2="${position}" y2="${plot.bottom}" stroke="${colors.grid}" stroke-width="1" stroke-dasharray="3 5" />
      <text x="${position}" y="${plot.bottom + 28}" text-anchor="${anchor}" fill="${colors.muted}" font-size="12">${formatDate(tick)}</text>`;
  }).join("");

  const emptyState = starredAt.length === 0
    ? `<text x="450" y="255" text-anchor="middle" fill="${colors.muted}" font-size="16">No stars yet</text>`
    : "";
  const gradientId = `star-area-${theme}`;
  const starLabel = `${starredAt.length} ${starredAt.length === 1 ? "star" : "stars"}`;

  const svg = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" role="img" aria-labelledby="title description">
  <title id="title">${escapeXml(nameWithOwner)} Star History</title>
  <desc id="description">GitHub star history for ${escapeXml(nameWithOwner)}. Current total: ${starredAt.length}.</desc>
  <defs>
    <linearGradient id="${gradientId}" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="${colors.area}" stop-opacity="0.34" />
      <stop offset="100%" stop-color="${colors.area}" stop-opacity="0.04" />
    </linearGradient>
  </defs>
  <rect x="0.5" y="0.5" width="899" height="459" rx="16" fill="${colors.background}" stroke="${colors.border}" />
  <g font-family="-apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif">
    <text x="32" y="42" fill="${colors.text}" font-size="24" font-weight="700">Star History</text>
    <text x="32" y="68" fill="${colors.muted}" font-size="14">${escapeXml(nameWithOwner)}</text>
    <text x="868" y="42" text-anchor="end" fill="${colors.accent}" font-size="20" font-weight="700">${starLabel}</text>
    <text x="868" y="67" text-anchor="end" fill="${colors.muted}" font-size="12">Updated ${formatDate(now)} UTC</text>
    ${yGrid}
    ${xGrid}
    <path d="${areaPath}" fill="url(#${gradientId})" />
    <path d="${linePath}" fill="none" stroke="${colors.accent}" stroke-width="3" stroke-linejoin="round" />
    ${emptyState}
  </g>
</svg>
`;
  return svg.replace(/[ \t]+$/gm, "");
}

const history = await fetchHistory();
const now = utcDayEnd(Date.now());
await mkdir(outputDir, { recursive: true });
const lightPath = path.join(outputDir, "star-history.svg");
const darkPath = path.join(outputDir, "star-history-dark.svg");
const suffix = `.tmp-${process.pid}`;
const lightTemp = `${lightPath}${suffix}`;
const darkTemp = `${darkPath}${suffix}`;

try {
  await Promise.all([
    writeFile(lightTemp, renderSvg(history, "light", now)),
    writeFile(darkTemp, renderSvg(history, "dark", now)),
  ]);
  await rename(lightTemp, lightPath);
  await rename(darkTemp, darkPath);
} catch (error) {
  await Promise.all([
    rm(lightTemp, { force: true }),
    rm(darkTemp, { force: true }),
  ]);
  throw error;
}

const unit = history.starredAt.length === 1 ? "star" : "stars";
console.log(`Generated Star History for ${history.nameWithOwner}: ${history.starredAt.length} ${unit}`);
