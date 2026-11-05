// Exercise the shipped progressive-enhancement script against the generated
// release data. Built-in Node modules only; no browser packages to install.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

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

function page() {
  let copied = '';
  const elements = {
    '#release-data': node({ textContent: JSON.stringify(release) }),
    '#install-platform': node({ value: 'darwin-arm64' }),
    '#verify': node(), '#c1': node(),
  };
  const links = release.artifacts.map((artifact) => node({ dataset: {
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
    navigator: { platform: 'MacIntel', userAgent: 'Mac', clipboard: {
      async writeText(text) { copied = text; },
    } },
    matchMedia: () => ({ matches: true }),
    localStorage: { getItem() { return null; }, setItem() {} },
    addEventListener() {}, setTimeout() {},
  });
  return { elements, links, copy, copied: () => copied };
}

test('platform selection uses the matching filename and checksum', () => {
  const p = page();
  for (const artifact of release.artifacts) {
    const picker = p.elements['#install-platform'];
    picker.value = `${artifact.os}-${artifact.arch}`;
    picker.handlers.change();
    const filename = path.basename(artifact.path);
    const checksum = artifact.os === 'linux' ? 'sha256sum -c -' : 'shasum -a 256 -c -';
    assert.equal(p.elements['#verify'].textContent,
      `printf '%s  %s\\n' '${artifact.sha256}' '${filename}' | ${checksum}`);
    assert.equal(p.elements['#c1'].textContent,
      `mkdir -p "$HOME/.local/bin" && install -m 755 ${filename} "$HOME/.local/bin/relops"`);
  }
});

test('clicking a download selects its installation command', async () => {
  const p = page();
  for (const link of p.links) {
    link.handlers.click();
    assert.equal(p.elements['#install-platform'].value, link.dataset.platform);
    assert.ok(p.elements['#c1'].textContent.includes(`relops-${link.dataset.platform} `));
    await p.copy.handlers.click();
    assert.equal(p.copied(), p.elements['#c1'].textContent);
  }
});
