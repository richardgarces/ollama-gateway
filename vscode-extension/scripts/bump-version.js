#!/usr/bin/env node
const fs = require('fs');
const path = require('path');

const packagePath = path.join(__dirname, '..', 'package.json');
const raw = fs.readFileSync(packagePath, 'utf8');
const pkg = JSON.parse(raw);

const version = String(pkg.version || '').trim();
const match = version.match(/^(\d+)\.(\d+)\.(\d+)$/);
if (!match) {
  console.error('Version invalida en package.json. Formato esperado: x.y.z');
  process.exit(1);
}

const major = Number(match[1]);
const minor = Number(match[2]);
const patch = Number(match[3]) + 1;
pkg.version = `${major}.${minor}.${patch}`;

fs.writeFileSync(packagePath, JSON.stringify(pkg, null, 2) + '\n', 'utf8');
console.log(`Version actualizada: ${version} -> ${pkg.version}`);
