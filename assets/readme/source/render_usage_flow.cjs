#!/usr/bin/env node
// Render only the usage flow PNGs; render_readme_images.cjs renders every
// README image and records what it rendered. No installed fonts are needed.
// Dependency: npm install sharp
const { renderImages } = require('./render_readme_images.cjs');

renderImages({ only: ['usage-flow'] }).catch(error => { console.error(error); process.exitCode = 1; });
