#!/usr/bin/env node

const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const maxMb = Number(process.env.MAX_PACKAGE_SIZE_MB || 5);
const maxBytes = Math.floor(maxMb * 1024 * 1024);

const include = [
  'dist',
  'extension.js',
  'metrics.js',
  'package.json',
  '.vscodeignore',
];

function walk(filePath) {
  const stat = fs.statSync(filePath);
  if (stat.isFile()) return stat.size;
  if (!stat.isDirectory()) return 0;
  return fs.readdirSync(filePath).reduce((sum, child) => {
    return sum + walk(path.join(filePath, child));
  }, 0);
}

let total = 0;
for (const rel of include) {
  const abs = path.join(root, rel);
  if (!fs.existsSync(abs)) continue;
  total += walk(abs);
}

const mb = (total / (1024 * 1024)).toFixed(2);
if (total > maxBytes) {
  console.error(`Package footprint ${mb}MB exceeds ${maxMb}MB`);
  process.exit(1);
}

console.log(`Package footprint ${mb}MB within ${maxMb}MB`);
