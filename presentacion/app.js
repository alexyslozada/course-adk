// Arma la lista de diapositivas a partir del temario y maneja la navegación.

const pad = (n) => String(n).padStart(2, "0");

function esc(s) {
  return String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
}

function buildSlides() {
  const slides = [{ type: "cover" }, { type: "agenda" }];
  MODULES.forEach((mod, mi) => {
    slides.push({ type: "module", mi });
    mod.classes.forEach((cls, ci) => {
      slides.push({ type: cls.scene ? "anim" : "title", mi, ci });
    });
  });
  slides.push({ type: "closing" });
  return slides;
}

const slides = buildSlides();
const stage = document.getElementById("stage");
const content = document.getElementById("content");
const moduleChip = document.getElementById("module-chip");
const bar = document.querySelector(".footer .bar");
const counter = document.querySelector(".counter");

let current = 0;
let scene = null;

const LEGEND = [
  ["Persona", "--k-user"], ["Agente", "--k-agent"], ["Modelo", "--k-model"],
  ["Código Go", "--k-code"], ["Otro proceso", "--k-ext"], ["Datos", "--k-store"],
];

function folderChip(folder) {
  return folder ? `<span class="chip code">${esc(folder)}/</span>` : "";
}

function render() {
  if (scene) { scene.stop(); scene = null; }
  const s = slides[current];
  const mod = MODULES[s.mi];
  const cls = mod && s.ci !== undefined ? mod.classes[s.ci] : null;

  moduleChip.textContent = mod ? `Módulo ${s.mi + 1} · ${mod.title}` : "";
  moduleChip.style.visibility = mod ? "visible" : "hidden";

  let html = "";
  switch (s.type) {
    case "cover":
      html = `
        <section class="slide cover">
          <div class="kicker">Taller presencial · EDteam</div>
          <h1 class="gradient-text">${esc(COURSE.title)}</h1>
          <p class="lead">${esc(COURSE.subtitle)}</p>
          <div class="meta">${COURSE.meta.map((m) => `<span class="chip sky">${esc(m)}</span>`).join("")}</div>
        </section>`;
      break;

    case "agenda": {
      const max = Math.max(...MODULES.map((m) => m.minutes));
      const total = MODULES.reduce((a, m) => a + m.minutes, 0);
      html = `
        <section class="slide agenda">
          <div class="kicker">El recorrido</div>
          <h2>5 módulos, del primer agente a un sistema completo</h2>
          <div class="agenda-list">
            ${MODULES.map((m, i) => `
              <div class="agenda-row">
                <div class="num">${pad(i + 1)}</div>
                <div class="name">${esc(m.title)}</div>
                <div class="track"><div class="fill" style="width:${(m.minutes / max) * 100}%"></div></div>
                <div class="min">${m.minutes} min</div>
              </div>`).join("")}
          </div>
          <div class="agenda-total">Total: ${total} minutos</div>
        </section>`;
      break;
    }

    case "module":
      html = `
        <section class="slide module">
          <div class="big-num">${pad(s.mi + 1)}</div>
          <h2>${esc(mod.title)}</h2>
          <p class="desc">${esc(mod.desc)} <span class="chip">${mod.minutes} min</span></p>
          <div class="classes">
            ${mod.classes.map((c, i) => `<div><b>${pad(i + 1)}</b>${esc(c.title)}</div>`).join("")}
          </div>
        </section>`;
      break;

    case "title":
      html = `
        <section class="slide class-title">
          <div class="kicker">Clase ${s.ci + 1}</div>
          <h2>${esc(cls.title)}</h2>
          <div class="row">${folderChip(cls.folder)}</div>
        </section>`;
      break;

    case "anim":
      html = `
        <section class="slide class-anim">
          <div class="head">
            <div>
              <div class="kicker">Clase ${s.ci + 1}</div>
              <h2>${esc(cls.title)}</h2>
            </div>
            ${folderChip(cls.folder)}
          </div>
          <div class="anim-wrap"></div>
          <div class="anim-bar">
            <div class="legend">${LEGEND.map(([n, v]) => `<span><i style="background:var(${v})"></i>${n}</span>`).join("")}</div>
            <div class="anim-controls">
              <button data-act="pause" title="Pausa (P)">Pausa</button>
              <button data-act="replay" title="Repetir (R)">Repetir</button>
            </div>
          </div>
          <div class="caption"></div>
        </section>`;
      break;

    case "closing":
      html = `
        <section class="slide closing">
          <div class="kicker">Fin del taller</div>
          <h1 class="gradient-text">¡Gracias!</h1>
          <p>Ahora tienes el mapa completo: agentes, herramientas, skills, estado, todos los flujos y un sistema real en contenedores.</p>
          <span class="chip code">github.com/alexyslozada/course-adk</span>
        </section>`;
      break;
  }
  content.innerHTML = html;

  if (s.type === "anim") {
    const wrap = content.querySelector(".anim-wrap");
    const caption = content.querySelector(".caption");
    let timer;
    const setCaption = (text) => {
      caption.classList.add("fade");
      clearTimeout(timer);
      timer = setTimeout(() => {
        caption.innerHTML = esc(text).replace(/`([^`]+)`/g, "<code>$1</code>");
        caption.classList.remove("fade");
      }, 180);
    };
    scene = new Scene(wrap, SCENES[cls.scene], setCaption);
    content.querySelector('[data-act="pause"]').onclick = togglePause;
    content.querySelector('[data-act="replay"]').onclick = () => scene.start();
    scene.start();
  }

  bar.style.width = `${((current + 1) / slides.length) * 100}%`;
  counter.textContent = `${current + 1} / ${slides.length}`;
  if (location.hash !== `#${current + 1}`) history.replaceState(null, "", `#${current + 1}`);
}

function togglePause() {
  if (!scene) return;
  const paused = scene.togglePause();
  const btn = content.querySelector('[data-act="pause"]');
  if (btn) btn.textContent = paused ? "Seguir" : "Pausa";
}

function go(i) {
  const next = Math.max(0, Math.min(slides.length - 1, i));
  if (next === current) return;
  current = next;
  render();
}

function fit() {
  const scale = Math.min(window.innerWidth / 1280, window.innerHeight / 720);
  stage.style.transform = `scale(${scale})`;
}

document.addEventListener("keydown", (e) => {
  switch (e.key) {
    case "ArrowRight": case "PageDown": case " ": case "Enter": go(current + 1); break;
    case "ArrowLeft": case "PageUp": case "Backspace": go(current - 1); break;
    case "Home": go(0); break;
    case "End": go(slides.length - 1); break;
    case "r": case "R": if (scene) scene.start(); break;
    case "p": case "P": togglePause(); break;
    case "f": case "F":
      if (document.fullscreenElement) document.exitFullscreen();
      else document.documentElement.requestFullscreen();
      break;
    default: return;
  }
  e.preventDefault();
});

stage.addEventListener("click", (e) => {
  if (e.target.closest("button")) return;
  const rect = stage.getBoundingClientRect();
  go(e.clientX - rect.left > rect.width / 3 ? current + 1 : current - 1);
});

window.addEventListener("resize", fit);
window.addEventListener("hashchange", () => {
  const n = parseInt(location.hash.slice(1), 10);
  if (n && n - 1 !== current) go(n - 1);
});

const start = parseInt(location.hash.slice(1), 10);
current = start ? Math.max(0, Math.min(slides.length - 1, start - 1)) : 0;
fit();
render();
