// relops site — progressive enhancement only. The page is fully readable without this.
(() => {
  "use strict";
  document.documentElement.classList.add("js");
  const reduceMotion = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const $ = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];
  const store = {
    get: (k) => { try { return localStorage.getItem(k); } catch { return null; } },
    set: (k, v) => { try { localStorage.setItem(k, v); } catch { /* private mode */ } },
  };

  /* ---- Accent + theme ------------------------------------------------- */
  const ACCENTS = ["orange", "lime", "cyan", "violet", "pink"];
  const root = document.documentElement;
  function setAccent(name) {
    root.dataset.accent = name;
    store.set("relops-accent", name);
    $$("[data-set-accent]").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.setAccent === name)));
  }
  function setTheme(name) {
    if (name === "auto") { delete root.dataset.theme; } else { root.dataset.theme = name; }
    store.set("relops-theme", name);
    $$("[data-set-theme]").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.setTheme === name)));
  }
  setAccent(ACCENTS.includes(store.get("relops-accent")) ? store.get("relops-accent") : "orange");
  setTheme(["dark", "light", "auto"].includes(store.get("relops-theme")) ? store.get("relops-theme") : "auto");
  $$("[data-set-accent]").forEach((b) => b.addEventListener("click", () => setAccent(b.dataset.setAccent)));
  $$("[data-set-theme]").forEach((b) => b.addEventListener("click", () => setTheme(b.dataset.setTheme)));
  // Easter egg: poke the brand to cycle accents.
  $("[data-brand]")?.addEventListener("click", () => {
    setAccent(ACCENTS[(ACCENTS.indexOf(root.dataset.accent) + 1) % ACCENTS.length]);
  });

  /* ---- Terminal demo -------------------------------------------------- */
  const SHA = "9c8c7c7e41b0a5d3f2186a4c7be0d95f13a2e6b8"; // pragma: allowlist secret (fake demo SHA)
  const SCENARIOS = {
    default: {
      label: "default branch",
      args: "relops-infra/path/to/check.sh --verbose",
      prov: `relops: mozilla-platform-ops/relops-infra  ref=<default:main>  sha=${SHA}  online`,
    },
    branch: {
      label: "my branch",
      args: "--ref my-feature relops-infra/path/to/check.sh --verbose",
      prov: `relops: mozilla-platform-ops/relops-infra  ref=my-feature  sha=${SHA}  online`,
    },
    sha: {
      label: "exact sha",
      args: `--ref ${SHA.slice(0, 12)}… relops-infra/path/to/check.sh`,
      prov: `relops: mozilla-platform-ops/relops-infra  ref=${SHA.slice(0, 12)}…  sha=${SHA}  online`,
    },
    offline: {
      label: "offline",
      args: "--offline --ref my-feature relops-infra/path/to/check.sh",
      prov: `relops: mozilla-platform-ops/relops-infra  ref=my-feature  sha=${SHA}  OFFLINE (cached)`,
    },
  };
  const OUTPUT = ["checking 42 hosts…", "  ok   t-linux64-ms-001", "  ok   t-linux64-ms-002", "  warn t-linux64-ms-003  (stale)", "done. exit 0"];

  const body = $("#term-body");
  const tabs = $("#term-tabs");
  let run = 0;
  const esc = (s) => s.replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c]));
  const sleep = (ms) => new Promise((r) => setTimeout(r, reduceMotion ? 0 : ms));

  async function play(key) {
    if (!body) return;
    const id = ++run;
    const sc = SCENARIOS[key];
    $$("button", tabs).forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.scenario === key)));
    const live = () => id === run;
    const cmd = `relops ${sc.args}`;
    body.innerHTML = '<span class="term__prompt">$ </span><span id="typed"></span><span class="cursor"></span>';
    const typed = $("#typed", body);
    for (let i = 1; i <= cmd.length; i++) {
      if (!live()) return;
      typed.textContent = cmd.slice(0, i);
      await sleep(14 + Math.random() * 22);
    }
    await sleep(350);
    if (!live()) return;
    $(".cursor", body)?.remove();
    typed.insertAdjacentHTML("afterend", `\n<span class="term__meta">${esc(sc.prov).replace(SHA, `<span class="term__sha">${SHA}</span>`)}</span>\n`);
    for (const line of OUTPUT) {
      await sleep(170);
      if (!live()) return;
      body.insertAdjacentHTML("beforeend", `<span class="term__out">${esc(line)}</span>\n`);
    }
    body.insertAdjacentHTML("beforeend", '<span class="term__prompt">$ </span><span class="cursor"></span>');
  }

  if (tabs && body) {
    Object.entries(SCENARIOS).forEach(([key, sc]) => {
      const b = document.createElement("button");
      b.className = "chip";
      b.type = "button";
      b.dataset.scenario = key;
      b.textContent = sc.label;
      b.addEventListener("click", () => play(key));
      tabs.append(b);
    });
    play("branch");
  }

  /* ---- Copy buttons --------------------------------------------------- */
  $$("[data-copy]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const text = $(btn.dataset.copy)?.textContent.trim();
      if (!text) return;
      try { await navigator.clipboard.writeText(text); btn.textContent = "copied"; }
      catch { btn.textContent = "select + ⌘C"; }
      setTimeout(() => (btn.textContent = "copy"), 1400);
    });
  });

  /* ---- Platform detection for the download button --------------------- */
  const ua = navigator.userAgent || "";
  const isLinux = /Linux/.test(ua) && !/Android/.test(ua);
  const isMac = /Mac/.test(navigator.platform || ua);
  const downloads = $$("[data-os]");
  const want = isLinux ? "linux" : isMac ? "macos" : null;
  downloads.forEach((a) => a.toggleAttribute("data-detected", a.dataset.os === want));
  const primary = $("#download-primary");
  if (primary && want) primary.textContent = `Get relops for ${want === "macos" ? "macOS" : "Linux"} ↓`;
  const unsupported = $("#unsupported");
  if (unsupported) unsupported.hidden = !!want;

  /* ---- Scroll reveal -------------------------------------------------- */
  const items = $$(".reveal");
  if ("IntersectionObserver" in window && !reduceMotion) {
    const io = new IntersectionObserver((es) => es.forEach((e) => {
      if (e.isIntersecting) { e.target.classList.add("is-in"); io.unobserve(e.target); }
    }), { rootMargin: "0px 0px -8% 0px" });
    items.forEach((el) => io.observe(el));
    // Safety net: never leave content hidden if the observer doesn't fire.
    const vh = innerHeight;
    items.forEach((el) => { if (el.getBoundingClientRect().top < vh) el.classList.add("is-in"); });
    setTimeout(() => items.forEach((el) => el.classList.add("is-in")), 4000);
  } else {
    items.forEach((el) => el.classList.add("is-in"));
  }

  /* ---- Moiré: pointer nudges the second grating ----------------------- */
  const moire = $(".moire");
  if (moire && !reduceMotion) {
    addEventListener("pointermove", (e) => {
      moire.style.setProperty("--mx", ((e.clientX / innerWidth) * 2 - 1).toFixed(3));
      moire.style.setProperty("--my", ((e.clientY / innerHeight) * 2 - 1).toFixed(3));
    }, { passive: true });
  }

  /* ---- Keyboard: "/" jumps to install --------------------------------- */
  addEventListener("keydown", (e) => {
    if (e.key === "/" && !/INPUT|TEXTAREA/.test(document.activeElement?.tagName || "")) {
      e.preventDefault();
      $("#install")?.scrollIntoView({ behavior: reduceMotion ? "auto" : "smooth" });
    }
  });
})();
