#!/usr/bin/env node
// Render the outlined README SVGs to PNG, assemble the walkthrough GIF from its
// SVG frames, and record in renders.json the bytes each output came from.
// No installed fonts are needed: the SVG text is already outlined.
// Dependency: npm install sharp
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const sharp = require('sharp');

const root = path.resolve(__dirname, '../../..');
const STILLS = ['usage-flow', 'goal-folder', 'report-card'];
const LANGUAGES = ['ko', 'en'];
const FRAMES = 'assets/readme/source/walkthrough/frames.json';
const MANIFEST = 'assets/readme/source/renders.json';

const absolute = file => path.join(root, file);
const sha256 = file => crypto.createHash('sha256').update(fs.readFileSync(absolute(file))).digest('hex');

function sorted(value) {
  if (Array.isArray(value)) return value.map(sorted);
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.keys(value).sort().map(key => [key, sorted(value[key])]));
  }
  return value;
}

async function renderStill(name, language) {
  const svg = `assets/readme/${name}.${language}.svg`;
  const png = `assets/readme/${name}.${language}.png`;
  const info = await sharp(absolute(svg)).png({ compressionLevel: 9 }).toFile(absolute(png));
  return [png, { width: info.width, height: info.height, inputs: [svg] }];
}

async function renderWalkthrough() {
  const plan = JSON.parse(fs.readFileSync(absolute(FRAMES), 'utf8'));
  const frames = [];
  for (const frame of plan.frames) {
    frames.push(await sharp(absolute(frame.svg)).png().toBuffer());
  }
  await sharp(frames, { join: { animated: true } })
    .gif({ delay: plan.frames.map(frame => frame.delay_ms), loop: 0, effort: 10, dither: 0 })
    .toFile(absolute(plan.output));
  return [plan.output, {
    width: plan.width, height: plan.height, frames: plan.frames.length,
    inputs: [FRAMES, ...plan.frames.map(frame => frame.svg)],
  }];
}

async function renderImages({ only } = {}) {
  const wanted = name => !only || only.includes(name);
  const manifest = fs.existsSync(absolute(MANIFEST))
    ? JSON.parse(fs.readFileSync(absolute(MANIFEST), 'utf8'))
    : { outputs: {} };
  const rendered = [];
  for (const name of STILLS.filter(wanted)) {
    for (const language of LANGUAGES) rendered.push(await renderStill(name, language));
  }
  if (wanted('walkthrough')) rendered.push(await renderWalkthrough());
  for (const [output, entry] of rendered) {
    manifest.outputs[output] = {
      sha256: sha256(output),
      width: entry.width,
      height: entry.height,
      ...(entry.frames ? { frames: entry.frames } : {}),
      inputs: Object.fromEntries(entry.inputs.map(input => [input, sha256(input)])),
    };
    console.log(`Rendered ${output}`);
  }
  manifest.renderer = { sharp: sharp.versions.sharp, libvips: sharp.versions.vips };
  fs.writeFileSync(absolute(MANIFEST), JSON.stringify(sorted(manifest), null, 2) + '\n');
}

module.exports = { renderImages };

if (require.main === module) {
  renderImages().catch(error => { console.error(error); process.exitCode = 1; });
}
