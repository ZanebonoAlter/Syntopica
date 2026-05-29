#!/usr/bin/env node
// E2E test runner that handles pnpm's -- argument passing
const { spawn } = require('child_process');

// Filter out the '--' that pnpm passes through
const args = process.argv.slice(2).filter(a => a !== '--');

const result = spawn('playwright', ['test', ...args], {
  stdio: 'inherit',
  shell: true
});

result.on('close', (code) => {
  process.exit(code);
});
