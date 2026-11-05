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
  const ACCENTS = ["volt", "cyan", "violet", "pink", "orange"];
  const root = document.documentElement;
  // Persist only on an explicit user choice, never for defaults,
  // so a changed default can't be masked by a stale saved value.
  function setAccent(name, persist = false) {
    root.dataset.accent = name;
    if (persist) store.set("relops-accent-v2", name);
    $$("[data-set-accent]").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.setAccent === name)));
  }
  function setTheme(name, persist = false) {
    if (name === "auto") { delete root.dataset.theme; } else { root.dataset.theme = name; }
    if (persist) store.set("relops-theme-v2", name);
    $$("[data-set-theme]").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.setTheme === name)));
  }
  const savedAccent = store.get("relops-accent-v2");
  const savedTheme = store.get("relops-theme-v2");
  setAccent(ACCENTS.includes(savedAccent) ? savedAccent : "volt");
  setTheme(["dark", "light", "auto"].includes(savedTheme) ? savedTheme : "auto");
  $$("[data-set-accent]").forEach((b) => b.addEventListener("click", () => setAccent(b.dataset.setAccent, true)));
  $$("[data-set-theme]").forEach((b) => b.addEventListener("click", () => setTheme(b.dataset.setTheme, true)));
  // Easter egg: poke the brand to cycle accents.
  $("[data-brand]")?.addEventListener("click", () => {
    setAccent(ACCENTS[(ACCENTS.indexOf(root.dataset.accent) + 1) % ACCENTS.length]); // easter egg: not persisted
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
  // Architecture stays explicit: browser platform strings cannot reliably
  // distinguish Apple silicon from Intel, or Linux arm64 from amd64.
  const platformPicker = $("#install-platform");
  let release = null;
  try { release = JSON.parse($("#release-data")?.textContent || "null"); } catch { /* source template */ }
  function selectPlatform(platform) {
    const artifact = release?.artifacts?.find((a) => `${a.os}-${a.arch}` === platform);
    if (!artifact || !platformPicker) return;
    platformPicker.value = platform;
    const filename = artifact.path.split("/").pop();
    const checksum = artifact.os === "linux" ? "sha256sum -c -" : "shasum -a 256 -c -";
    const verify = `printf '%s  %s\\n' '${artifact.sha256}' '${filename}' | ${checksum}`;
    const install = `(cd "$HOME/Downloads" && ${verify} && mkdir -p "$HOME/.local/bin" && install -m 755 ${filename} "$HOME/.local/bin/relops") && export PATH="$HOME/.local/bin:$PATH" && relops --version`;
    $("#verify").textContent = `(cd "$HOME/Downloads" && ${verify})`;
    $("#c1").textContent = install;
    if ($("#ex")) $("#ex").textContent = install;
    if ($("#manual-install")) $("#manual-install").textContent = `mkdir -p "$HOME/.local/bin" && install -m 755 "$HOME/Downloads/${filename}" "$HOME/.local/bin/relops"`;
  }
  platformPicker?.addEventListener("change", () => selectPlatform(platformPicker.value));
  $$("[data-platform]").forEach((a) => a.addEventListener("click", () => selectPlatform(a.dataset.platform)));

  const ua = navigator.userAgent || "";
  const isLinux = /Linux/.test(ua) && !/Android/.test(ua);
  const isMac = /Mac/.test(navigator.platform || ua);
  if (platformPicker) selectPlatform(isLinux ? "linux-amd64" : platformPicker.value);
  const downloads = $$("[data-os]");
  const want = isLinux ? "linux" : isMac ? "macos" : null;
  downloads.forEach((a) => a.toggleAttribute("data-detected", a.dataset.os === want));
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

  /* ---- Blob: organic, never-repeating silhouette ------------------------ */
  // The outline is a closed curve whose radius is a sum of slowly drifting sine
  // harmonics (each with its own speed/phase), drawn as a clip-path polygon.
  // Because the speeds are unrelated, the shape never loops. Falls back to the
  // CSS border-radius morph under reduced motion or without JS.
  const blob = $(".blob");
  const blobBody = blob && $(".blob__body", blob);
  if (blobBody && !reduceMotion) {
    const POINTS = 160;
    // [harmonic, amplitude, speed rad/s, phase]
    const H = [
      [2, 0.075, 0.31, 0.4], [3, 0.045, 0.43, 2.1], [4, 0.02, 0.57, 4.3],
      [5, 0.01, 0.71, 1.2], [1, 0.03, 0.23, 3.3],
    ];
    const cos = new Float32Array(POINTS), sin = new Float32Array(POINTS);
    for (let i = 0; i < POINTS; i++) { const a = (i / POINTS) * Math.PI * 2; cos[i] = Math.cos(a); sin[i] = Math.sin(a); }
    const r = new Float32Array(POINTS);
    let visible = true, raf = 0, t0 = performance.now();

    const frame = (now) => {
      const t = (now - t0) / 1000;
      let max = 0;
      for (let i = 0; i < POINTS; i++) {
        const a = (i / POINTS) * Math.PI * 2;
        let v = 1;
        for (const [k, amp, w, ph] of H) v += amp * Math.sin(k * a + ph + w * t);
        r[i] = v; if (v > max) max = v;
      }
      const breathe = 0.955 + 0.045 * Math.sin(t * 0.37);
      const s = (50 / max) * breathe;
      let d = "";
      for (let i = 0; i < POINTS; i++) {
        d += `${(50 + cos[i] * r[i] * s).toFixed(2)}% ${(50 + sin[i] * r[i] * s).toFixed(2)}%${i < POINTS - 1 ? "," : ""}`;
      }
      blobBody.style.clipPath = `polygon(${d})`;
      if (visible) raf = requestAnimationFrame(frame);
    };
    blob.classList.add("blob--live");
    new IntersectionObserver(([e]) => {
      visible = e.isIntersecting;
      if (visible) { cancelAnimationFrame(raf); raf = requestAnimationFrame(frame); }
    }).observe(blob);
    raf = requestAnimationFrame(frame);
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
