// Renders the app icons from one drawing: the favicon's lock on the ink square.
// "any" icons keep the rounded square on a transparent canvas; the maskable one
// and the Apple touch icon fill the canvas, since those platforms cut the shape.
import { chromium } from "@playwright/test";
import { writeFileSync } from "node:fs";

const OUT = process.argv[2];
const INK = "#111113";
const ACCENT = "#f2a93b";

function lock(scale) {
  // The favicon's lock, drawn in a 32-unit box, scaled around the centre.
  const s = scale;
  return `<g transform="translate(16 16) scale(${s}) translate(-16 -16)">
    <rect x="8" y="14" width="16" height="12" rx="2.5" fill="${ACCENT}"/>
    <path d="M11 14v-3.5a5 5 0 0 1 10 0V14" fill="none" stroke="${ACCENT}" stroke-width="2.5" stroke-linecap="round"/>
    <circle cx="16" cy="20" r="1.8" fill="${INK}"/>
  </g>`;
}

const variants = [
  { file: "icon-192.png", size: 192, rounded: true, scale: 1 },
  { file: "icon-512.png", size: 512, rounded: true, scale: 1 },
  { file: "icon-maskable-512.png", size: 512, rounded: false, scale: 0.72 },
  { file: "apple-touch-icon.png", size: 180, rounded: false, scale: 0.84 },
];

const browser = await chromium.launch();
const page = await browser.newPage({ deviceScaleFactor: 1 });
for (const v of variants) {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="${v.size}" height="${v.size}">
    <rect width="32" height="32" rx="${v.rounded ? 7 : 0}" fill="${INK}"/>${lock(v.scale)}</svg>`;
  await page.setViewportSize({ width: v.size, height: v.size });
  await page.setContent(`<body style="margin:0;background:transparent">${svg}</body>`);
  const png = await page.screenshot({ omitBackground: true, clip: { x: 0, y: 0, width: v.size, height: v.size } });
  writeFileSync(`${OUT}/${v.file}`, png);
  console.log(v.file, png.length, "bytes");
}
await browser.close();
