#!/usr/bin/env node
// Bake this trusted entrypoint into the pinned suite image as /wazi/qualify.
// It exercises only a synthetic /work data directory; no monitor credentials.
import { spawn } from 'node:child_process';
import { mkdir, readFile, stat, writeFile } from 'node:fs/promises';
import http from 'node:http';

const ids = ['candidate-profile', 'startup-http', 'sqlite-created', 'restart-http', 'synthetic-file-persistence'];
const results = new Map(ids.map(id => [id, 'failed']));
let child;
async function stop() {
  if (!child || !child.pid || child.exitCode !== null) return;
  const current = child;
  const exited = new Promise(resolve => current.once('exit', resolve));
  current.kill('SIGTERM');
  const timer = setTimeout(() => current.kill('SIGKILL'), 5000);
  await exited;
  clearTimeout(timer);
}
function start() {
  child = spawn('/usr/local/bin/node', ['/app/server/server.js'], {
    cwd: '/app', env: { ...process.env, DATA_DIR: '/work/data' }, stdio: 'ignore',
  });
  // Spawn errors become a failed readiness result rather than raw output.
  child.on('error', () => {});
}
function probe() {
  return new Promise(resolve => {
    const request = http.get('http://127.0.0.1:3001/', response => {
      response.resume();
      resolve(response.statusCode >= 200 && response.statusCode < 400);
    });
    request.setTimeout(1000, () => request.destroy());
    request.on('error', () => resolve(false));
  });
}
async function ready() {
  const deadline = Date.now() + 45000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) return false;
    if (await probe()) return true;
    await new Promise(resolve => setTimeout(resolve, 250));
  }
  return false;
}
try {
  let input = '';
  for await (const chunk of process.stdin) {
    input += chunk;
    if (Buffer.byteLength(input) > 1048576) throw new Error('input-bound');
  }
  const packet = JSON.parse(input);
  if (packet.manifest.appId !== 'pet.kuma.uptime' || packet.manifest.reviewState !== 'candidate') throw new Error('candidate-profile');
  results.set('candidate-profile', 'passed');
  await mkdir('/work/data', { recursive: true, mode: 0o700 });
  start();
  if (!await ready()) throw new Error('startup');
  results.set('startup-http', 'passed');
  if ((await stat('/work/data/kuma.db')).size <= 0) throw new Error('database');
  results.set('sqlite-created', 'passed');
  await writeFile('/work/data/wazi-synthetic-marker', 'wazi-synthetic-state', { flag: 'wx', mode: 0o600 });
  await stop();
  start();
  if (!await ready()) throw new Error('restart');
  results.set('restart-http', 'passed');
  if (await readFile('/work/data/wazi-synthetic-marker', 'utf8') !== 'wazi-synthetic-state') throw new Error('persistence');
  results.set('synthetic-file-persistence', 'passed');
} catch {
  // Only closed result identifiers cross the runner boundary.
} finally {
  await stop();
  process.stdout.write(JSON.stringify({ tests: ids.map(testId => ({ testId, result: results.get(testId) })) }));
  process.exitCode = [...results.values()].every(value => value === 'passed') ? 0 : 1;
}
