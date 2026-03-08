// render.mjs -- Render JSON screenshot data to PNGs via xterm.js + Playwright.
//
// Usage: node cmd/screenshots/render.mjs [dir]
//
// Reads .json files produced by the Go screenshot generator from [dir]
// (default: web/public/screenshots). Each JSON contains raw ANSI terminal
// output plus theme and highlight configuration. This script feeds the ANSI
// data into xterm.js (a real terminal emulator running in headless Chromium)
// and captures the result as a high-DPI PNG. JSON files are deleted after
// successful conversion.

import { chromium } from 'playwright';
import { readdir, readFile, unlink } from 'node:fs/promises';
import { join, resolve, relative, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const dir = resolve(process.argv[2] || 'web/public/screenshots');

// Paths to xterm.js assets (loaded into each Playwright page).
const xtermCssPath = join(__dirname, 'node_modules/@xterm/xterm/css/xterm.css');
const xtermJsPath = join(__dirname, 'node_modules/@xterm/xterm/lib/xterm.js');
const unicode11JsPath = join(__dirname, 'node_modules/@xterm/addon-unicode11/lib/addon-unicode11.js');

// Recursively find all .json files in dir and subdirectories.
async function findJsonFiles(baseDir) {
  const entries = await readdir(baseDir, { withFileTypes: true });
  let result = [];
  for (const entry of entries) {
    const fullPath = join(baseDir, entry.name);
    if (entry.isDirectory()) {
      result = result.concat(await findJsonFiles(fullPath));
    } else if (entry.name.endsWith('.json')) {
      result.push(fullPath);
    }
  }
  return result;
}

const jsonFiles = await findJsonFiles(dir);
if (jsonFiles.length === 0) {
  console.error(`No .json files found in ${dir}`);
  process.exit(1);
}

console.log(`Rendering ${jsonFiles.length} screenshots from ${dir}`);

const browser = await chromium.launch();
const context = await browser.newContext({
  deviceScaleFactor: 2,
  viewport: { width: 1400, height: 900 },
});

let failed = 0;
for (let i = 0; i < jsonFiles.length; i++) {
  const jsonPath = jsonFiles[i];
  const pngPath = jsonPath.replace(/\.json$/, '.png');
  const label = relative(dir, pngPath);

  try {
    const data = JSON.parse(await readFile(jsonPath, 'utf-8'));
    const page = await context.newPage();

    // Start with a minimal HTML shell.
    await page.setContent(`<!DOCTYPE html>
<html><head><meta charset="utf-8"></head>
<body><div id="terminal-container"><div id="terminal"></div></div></body>
</html>`, { waitUntil: 'load' });

    // Inject xterm.js CSS and JS.
    await page.addStyleTag({ path: xtermCssPath });
    await page.addScriptTag({ path: xtermJsPath });
    await page.addScriptTag({ path: unicode11JsPath });

    // Initialize the terminal, write ANSI data, and add highlights.
    const hasHL = data.highlights && data.highlights.length > 0;
    await page.evaluate(({ data, hasHL }) => {
      const pad = hasHL ? 6 : 0;

      // Style the page.
      document.body.style.cssText = `margin:0;padding:0;background:${data.bg};display:inline-block`;
      const container = document.getElementById('terminal-container');
      container.style.cssText = `position:relative;display:inline-block;padding:${pad}px`;

      // Build the xterm.js theme from the 16-color palette.
      const p = data.palette;
      const theme = {
        background:   data.bg,
        foreground:   data.fg,
        cursor:       data.fg,
        cursorAccent: data.bg,
        black:        p[0],
        red:          p[1],
        green:        p[2],
        yellow:       p[3],
        blue:         p[4],
        magenta:      p[5],
        cyan:         p[6],
        white:        p[7],
        brightBlack:  p[8],
        brightRed:    p[9],
        brightGreen:  p[10],
        brightYellow: p[11],
        brightBlue:   p[12],
        brightMagenta:p[13],
        brightCyan:   p[14],
        brightWhite:  p[15],
      };

      // Create the terminal emulator.
      const term = new window.Terminal({
        cols:            data.cols,
        rows:            data.rows,
        fontFamily:      "'CaskaydiaCove NF','CaskaydiaCove Nerd Font','Cascadia Code','JetBrains Mono','Fira Code','Consolas','Courier New',monospace",
        fontSize:        14,
        lineHeight:      1.2,
        letterSpacing:   0,
        allowTransparency: false,
        cursorBlink:     false,
        cursorStyle:     'block',
        disableStdin:    true,
        scrollback:      0,
        convertEol:      true,
        allowProposedApi: true,
        theme,
      });

      // Load Unicode 11 for proper wide-char rendering.
      // The UMD wrapper nests the constructor: window.Unicode11Addon.Unicode11Addon
      const Unicode11 = window.Unicode11Addon.Unicode11Addon || window.Unicode11Addon;
      const unicode11 = new Unicode11();
      term.loadAddon(unicode11);
      term.unicode.activeVersion = '11';

      term.open(document.getElementById('terminal'));
      term.write(data.ansi);

      // Hide the cursor layer.
      const cursorLayer = document.querySelector('.xterm-cursor-layer');
      if (cursorLayer) cursorLayer.style.display = 'none';

      // Add highlight overlays (spotlight + callout).
      if (hasHL) {
        const screen = document.querySelector('.xterm-screen');
        const cellW = screen.offsetWidth / data.cols;
        const cellH = screen.offsetHeight / data.rows;

        for (const h of data.highlights) {
          const y1 = h.row * cellH + pad;
          const y2 = y1 + h.rows * cellH;
          const x1 = h.col * cellW + pad;
          const x2 = x1 + h.cols * cellW;

          const spotStyle = 'position:absolute;background:rgba(0,0,0,0.50);pointer-events:none;z-index:10;';

          // Top bar.
          if (y1 > 0) {
            const d = document.createElement('div');
            d.style.cssText = spotStyle + `top:0;left:0;right:0;height:${y1}px`;
            container.appendChild(d);
          }
          // Bottom bar.
          const d2 = document.createElement('div');
          d2.style.cssText = spotStyle + `top:${y2}px;left:0;right:0;bottom:0`;
          container.appendChild(d2);
          // Left bar.
          if (x1 > 0) {
            const d3 = document.createElement('div');
            d3.style.cssText = spotStyle + `top:${y1}px;left:0;width:${x1}px;height:${y2 - y1}px`;
            container.appendChild(d3);
          }
          // Right bar.
          const d4 = document.createElement('div');
          d4.style.cssText = spotStyle + `top:${y1}px;left:${x2}px;right:0;height:${y2 - y1}px`;
          container.appendChild(d4);

          // Callout border.
          const callPad = 3;
          const d5 = document.createElement('div');
          d5.style.cssText = `position:absolute;pointer-events:none;z-index:11;border:3px solid #E8375A;border-radius:4px;box-shadow:inset 0 0 8px rgba(232,55,90,0.35);top:${y1 - callPad}px;left:${x1 - callPad}px;width:${x2 - x1 + callPad * 2}px;height:${y2 - y1 + callPad * 2}px`;
          container.appendChild(d5);
        }
      }
    }, { data, hasHL });

    // Give xterm.js a moment to finish painting (font loading, layout).
    await page.waitForTimeout(200);

    // Screenshot the terminal container.
    const container = page.locator('#terminal-container');
    await container.screenshot({ path: pngPath, type: 'png' });
    await page.close();

    await unlink(jsonPath);
    console.log(`  [${i + 1}/${jsonFiles.length}] ${label}`);
  } catch (err) {
    console.error(`  [${i + 1}/${jsonFiles.length}] FAIL ${label}: ${err.message}`);
    failed++;
  }
}

await browser.close();

if (failed > 0) {
  console.error(`\n${failed} of ${jsonFiles.length} screenshots failed`);
  process.exit(1);
}

console.log(`\nAll ${jsonFiles.length} screenshots saved to ${dir}`);
