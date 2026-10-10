#!/usr/bin/env node
// Render outlined SVGs; no local font installation is needed for this step.
// Dependency: npm install sharp
const path = require('node:path');
const sharp = require('sharp');
const root = process.argv[2] || path.resolve(__dirname, '../../..');

(async () => {
  for (const language of ['ko', 'en']) {
    const base = path.join(root, 'assets', 'readme', `usage-flow.${language}`);
    await sharp(`${base}.svg`).png().toFile(`${base}.png`);
    console.log(`Rendered ${base}.png`);
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
