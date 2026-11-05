// Exercise the shipped progressive-enhancement script against the generated
// release data. Built-in Node modules only; no browser packages to install.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const os = require('node:os');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');

const site = path.resolve(__dirname, '../../dist/site');
const release = JSON.parse(fs.readFileSync(path.join(site, 'releases.json'), 'utf8'));
const script = fs.readFileSync(path.join(site, 'js/main.js'), 'utf8');

function node(extra = {}) {
  return {
    dataset: {}, textContent: '', value: '', handlers: {},
    addEventListener(name, handler) { this.handlers[name] = handler; },
    toggleAttribute() {}, setAttribute() {},
    ...extra,
  };
}

function page(data = release, linux = false) {
  let copied = '';
  const elements = {
    '#release-data': node({ textContent: JSON.stringify(data) }),
    '#install-platform': node({ value: 'darwin-arm64' }),
    '#verify': node(), '#c1': node(), '#ex': node(), '#manual-install': node(),
  };
  const links = data.artifacts.map((artifact) => node({ dataset: {
    platform: `${artifact.os}-${artifact.arch}`,
    os: artifact.os === 'darwin' ? 'macos' : 'linux',
  } }));
  const copy = node({ dataset: { copy: '#c1' } });
  const document = {
    documentElement: { dataset: {}, classList: { add() {} } },
    querySelector(selector) { return elements[selector] || null; },
    querySelectorAll(selector) {
      if (selector === '[data-platform]' || selector === '[data-os]') return links;
      if (selector === '[data-copy]') return [copy];
      return [];
    },
  };
  vm.runInNewContext(script, {
    document, window: {},
    navigator: { platform: linux ? 'Linux x86_64' : 'MacIntel', userAgent: linux ? 'Linux' : 'Mac', clipboard: {
      async writeText(text) { copied = text; },
    } },
    matchMedia: () => ({ matches: true }),
    localStorage: { getItem() { return null; }, setItem() {} },
    addEventListener() {}, setTimeout() {},
  });
  return { elements, links, copy, copied: () => copied };
}

test('platform selection updates all install commands and copy buttons', async () => {
  assert.deepEqual(release.artifacts.map(a => a.os + '-' + a.arch), ['darwin-arm64', 'linux-amd64']);
  const p = page();
  for (const link of p.links) {
    link.handlers.click();
    const artifact = release.artifacts.find(a => a.os + '-' + a.arch === link.dataset.platform);
    const command = p.elements['#c1'].textContent;
    assert.equal(p.elements['#install-platform'].value, link.dataset.platform);
    assert.ok(command.includes(artifact.sha256));
    assert.ok(command.includes('cd "$HOME/Downloads"'));
    assert.equal(p.elements['#ex'].textContent, command);
    assert.ok(p.elements['#manual-install'].textContent.includes('$HOME/Downloads/relops-' + link.dataset.platform));
    await p.copy.handlers.click();
    assert.equal(p.copied(), command);
    p.elements['#install-platform'].value = link.dataset.platform;
    p.elements['#install-platform'].handlers.change();
    assert.equal(p.elements['#c1'].textContent, command);
  }
});

test('Linux visitors initially see the Linux install command', () => {
  const p = page(release, true);
  assert.equal(p.elements['#install-platform'].value, 'linux-amd64');
  assert.ok(p.elements['#ex'].textContent.includes('sha256sum -c -'));
});

test('pasted install command works outside Downloads and rejects incorrect bytes', t => {
  const base = fs.mkdtempSync(path.join(os.tmpdir(), 'relops-install-'));
  t.after(() => fs.rmSync(base, { recursive: true, force: true }));
  const binary = '#!/bin/sh\nprintf "relops test\\n"\n';
  const data = JSON.parse(JSON.stringify(release));
  for (const artifact of data.artifacts) artifact.sha256 = crypto.createHash('sha256').update(binary).digest('hex');
  const p = page(data);
  for (const artifact of data.artifacts) {
    const platform = artifact.os + '-' + artifact.arch;
    const home = path.join(base, platform + ' home');
    const downloadDir = path.join(home, 'Downloads');
    fs.mkdirSync(downloadDir, { recursive: true });
    const download = path.join(downloadDir, path.basename(artifact.path));
    const installed = path.join(home, '.local/bin/relops');
    p.elements['#install-platform'].value = platform;
    p.elements['#install-platform'].handlers.change();
    const command = p.elements['#c1'].textContent;
    const run = () => spawnSync('/bin/sh', ['-c', command], { cwd: base, env: { ...process.env, HOME: home }, encoding: 'utf8' });
    fs.writeFileSync(download, binary);
    const success = run();
    assert.equal(success.status, 0, success.stderr);
    assert.ok(success.stdout.includes('relops test'));
    assert.equal(fs.readFileSync(installed, 'utf8'), binary);
    fs.writeFileSync(download, '<html>Sign in</html>');
    assert.notEqual(run().status, 0);
    assert.equal(fs.readFileSync(installed, 'utf8'), binary, 'bad download overwrote installed launcher');
    fs.unlinkSync(download);
    assert.notEqual(run().status, 0);
  }
});
