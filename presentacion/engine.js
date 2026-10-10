// Motor de animaciones: dibuja una escena (nodos, aristas y grupos) en SVG y
// reproduce sus pasos en bucle. Cada escena es solo datos (ver scenes.js).
//
// Pasos disponibles:
//   ["cap", "texto"]                    cambia el subtítulo
//   ["send", edgeId, "etiqueta", {back}] viaja un mensaje por una arista
//   ["show"|"hide"|"dim"|"undim", id | [ids]]
//   ["on"|"off"|"ok"|"bad"|"clear", id | [ids]]
//   ["flash", id]                       destello en un nodo
//   ["text", nodeId, label, sub]        cambia el texto de un nodo
//   ["wait", ms]
//   ["par", [pasos...]]                 ejecuta varios pasos a la vez

const SVGNS = "http://www.w3.org/2000/svg";

function el(tag, attrs = {}, parent) {
  const e = document.createElementNS(SVGNS, tag);
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
  if (parent) parent.appendChild(e);
  return e;
}

function asList(x) {
  return Array.isArray(x) ? x : [x];
}

class Cancelled extends Error {}

class Scene {
  constructor(container, def, setCaption) {
    this.def = def;
    this.setCaption = setCaption;
    this.speed = 1;
    this.paused = false;
    this.runId = 0;

    const w = def.w || 1000;
    const h = def.h || 440;
    this.svg = el("svg", { viewBox: `0 0 ${w} ${h}`, preserveAspectRatio: "xMidYMid meet" });
    container.appendChild(this.svg);

    const defs = el("defs", {}, this.svg);
    const marker = el("marker", {
      id: "arrow", viewBox: "0 0 10 10", refX: "9", refY: "5",
      markerWidth: "7", markerHeight: "7", orient: "auto-start-reverse",
    }, defs);
    el("path", { d: "M0,0 L10,5 L0,10 z", fill: "#ffffff66" }, marker);

    this.layers = {
      groups: el("g", {}, this.svg),
      edges: el("g", {}, this.svg),
      nodes: el("g", {}, this.svg),
      tokens: el("g", {}, this.svg),
    };

    this.items = {};
    this.nodes = {};
    for (const g of def.groups || []) this.drawGroup(g);
    for (const n of def.nodes || []) this.drawNode(n);
    for (const e of def.edges || []) this.drawEdge(e);
    this.reset();
  }

  // ---------- dibujo ----------

  drawGroup(g) {
    const grp = el("g", { class: "group" }, this.layers.groups);
    el("rect", { x: g.x, y: g.y, width: g.w, height: g.h, rx: 16 }, grp);
    const t = el("text", { x: g.x + 16, y: g.y + 24 }, grp);
    t.textContent = g.label || "";
    this.items[g.id] = { el: grp, def: g, type: "group" };
  }

  drawNode(n) {
    const w = n.w || 170;
    const h = n.h || (n.sub ? 62 : 48);
    const grp = el("g", { class: `node k-${n.kind || "agent"}`, transform: `translate(${n.x},${n.y})` }, this.layers.nodes);
    if (n.shape === "diamond") {
      el("polygon", { points: `0,${-h / 2 - 8} ${w / 2},0 0,${h / 2 + 8} ${-w / 2},0` }, grp);
    } else {
      el("rect", { x: -w / 2, y: -h / 2, width: w, height: h, rx: n.kind === "user" ? h / 2 : 12 }, grp);
    }
    const label = el("text", { class: "label", "text-anchor": "middle" }, grp);
    const sub = el("text", { class: "sub", "text-anchor": "middle" }, grp);
    const node = { el: grp, def: n, type: "node", w, h, label, sub };
    this.items[n.id] = node;
    this.nodes[n.id] = node;
  }

  setNodeText(node, label, sub) {
    node.label.textContent = label;
    while (node.sub.firstChild) node.sub.removeChild(node.sub.firstChild);
    const lines = sub ? String(sub).split("\n") : [];
    const total = 1 + lines.length;
    const lineH = 17;
    const top = -((total - 1) * lineH) / 2;
    node.label.setAttribute("y", top + 5);
    lines.forEach((line, i) => {
      const ts = el("tspan", { x: 0, y: top + 5 + (i + 1) * lineH }, node.sub);
      ts.textContent = line;
    });
  }

  // punto del borde del nodo en dirección (dx, dy) desde su centro
  border(node, dx, dy) {
    const hw = node.w / 2 + 4;
    const hh = node.h / 2 + 4;
    const len = Math.hypot(dx, dy) || 1;
    const ux = dx / len;
    const uy = dy / len;
    const t = Math.min(ux ? hw / Math.abs(ux) : Infinity, uy ? hh / Math.abs(uy) : Infinity);
    return [node.def.x + ux * t, node.def.y + uy * t];
  }

  drawEdge(e) {
    const a = this.nodes[e.from];
    const b = this.nodes[e.to];
    const [ax, ay, bx, by] = [a.def.x, a.def.y, b.def.x, b.def.y];
    const bend = e.bend || 0;
    let d;
    let mid;
    if (bend) {
      const mx = (ax + bx) / 2;
      const my = (ay + by) / 2;
      const len = Math.hypot(bx - ax, by - ay) || 1;
      const cx = mx + (-(by - ay) / len) * bend;
      const cy = my + ((bx - ax) / len) * bend;
      const p1 = this.border(a, cx - ax, cy - ay);
      const p2 = this.border(b, cx - bx, cy - by);
      d = `M${p1[0]},${p1[1]} Q${cx},${cy} ${p2[0]},${p2[1]}`;
      mid = [0.25 * p1[0] + 0.5 * cx + 0.25 * p2[0], 0.25 * p1[1] + 0.5 * cy + 0.25 * p2[1]];
    } else {
      const p1 = this.border(a, bx - ax, by - ay);
      const p2 = this.border(b, ax - bx, ay - by);
      d = `M${p1[0]},${p1[1]} L${p2[0]},${p2[1]}`;
      mid = [(p1[0] + p2[0]) / 2, (p1[1] + p2[1]) / 2];
    }
    const grp = el("g", { class: `edge${e.dashed ? " dashed" : ""}` }, this.layers.edges);
    const path = el("path", { d, "marker-end": e.noArrow ? "" : "url(#arrow)" }, grp);
    if (e.label) {
      const bg = el("rect", { class: "elabel-bg", rx: 4 }, grp);
      const t = el("text", { class: "elabel", x: mid[0], y: mid[1] + 4, "text-anchor": "middle" }, grp);
      t.textContent = e.label;
      requestAnimationFrame(() => {
        const bb = t.getBBox();
        bg.setAttribute("x", bb.x - 5);
        bg.setAttribute("y", bb.y - 2);
        bg.setAttribute("width", bb.width + 10);
        bg.setAttribute("height", bb.height + 4);
      });
    }
    this.items[e.id] = { el: grp, def: e, type: "edge", path };
  }

  // ---------- estado ----------

  reset() {
    this.layers.tokens.replaceChildren();
    for (const it of Object.values(this.items)) {
      it.el.classList.remove("is-on", "is-ok", "is-bad", "is-flash", "dim");
      it.el.classList.toggle("hid", !!it.def.hidden);
      if (it.type === "node") this.setNodeText(it, it.def.label, it.def.sub);
    }
    this.setCaption(this.def.caption || "");
  }

  // ---------- reproducción ----------

  start() {
    this.stop();
    const id = ++this.runId;
    this.loop(id);
  }

  stop() {
    this.runId++;
    this.paused = false;
  }

  togglePause() {
    this.paused = !this.paused;
    return this.paused;
  }

  async loop(id) {
    try {
      for (;;) {
        this.reset();
        await this.sleep(500, id);
        await this.runSteps(this.def.steps, id);
        await this.sleep(3200, id);
      }
    } catch (err) {
      if (!(err instanceof Cancelled)) throw err;
    }
  }

  check(id) {
    if (id !== this.runId) throw new Cancelled();
  }

  // espera que respeta la pausa y la cancelación
  sleep(ms, id) {
    return new Promise((resolve, reject) => {
      let left = ms / this.speed;
      let last = performance.now();
      const tick = (now) => {
        if (id !== this.runId) return reject(new Cancelled());
        if (!this.paused) left -= now - last;
        last = now;
        if (left <= 0) resolve();
        else requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    });
  }

  async runSteps(steps, id) {
    for (const step of steps) {
      this.check(id);
      await this.runStep(step, id);
    }
  }

  async runStep([op, ...args], id) {
    const each = (ids, fn) => asList(ids).forEach((x) => {
      const it = this.items[x];
      if (!it) throw new Error(`no existe el elemento "${x}"`);
      fn(it.el, it);
    });
    switch (op) {
      case "cap":
        this.setCaption(args[0]);
        return this.sleep(args[1] ?? 1100, id);
      case "show": each(args[0], (e) => e.classList.remove("hid", "dim")); return this.sleep(350, id);
      case "hide": each(args[0], (e) => e.classList.add("hid")); return this.sleep(300, id);
      case "dim": each(args[0], (e) => e.classList.add("dim")); return this.sleep(300, id);
      case "undim": each(args[0], (e) => e.classList.remove("dim")); return this.sleep(300, id);
      case "on": each(args[0], (e) => e.classList.add("is-on")); return this.sleep(250, id);
      case "off": each(args[0], (e) => e.classList.remove("is-on")); return undefined;
      case "ok": each(args[0], (e) => { e.classList.remove("is-on", "is-bad"); e.classList.add("is-ok"); }); return this.sleep(450, id);
      case "bad": each(args[0], (e) => { e.classList.remove("is-on", "is-ok"); e.classList.add("is-bad"); }); return this.sleep(450, id);
      case "clear": each(args[0], (e) => e.classList.remove("is-on", "is-ok", "is-bad")); return undefined;
      case "flash":
        each(args[0], (e) => { e.classList.remove("is-flash"); void e.getBBox(); e.classList.add("is-flash"); });
        return this.sleep(600, id);
      case "text": {
        const node = this.nodes[args[0]];
        this.setNodeText(node, args[1] ?? node.def.label, args[2]);
        node.el.classList.remove("is-flash");
        void node.el.getBBox();
        node.el.classList.add("is-flash");
        return this.sleep(700, id);
      }
      case "wait": return this.sleep(args[0], id);
      case "par": return Promise.all(args[0].map((s) => this.runStep(s, id)));
      case "send": return this.send(args[0], args[1], args[2] || {}, id);
      default: throw new Error(`paso desconocido: ${op}`);
    }
  }

  // un mensaje que viaja por la arista; back = en sentido contrario (respuesta)
  async send(edgeId, label, opts, id) {
    const edge = this.items[edgeId];
    if (!edge) throw new Error(`no existe la arista "${edgeId}"`);
    edge.el.classList.remove("hid");
    edge.el.classList.add("is-on");
    const path = edge.path;
    const total = path.getTotalLength();
    const dur = (opts.dur || Math.min(1500, Math.max(750, total * 2.4))) / this.speed;

    const tok = el("g", { class: `token${opts.back ? " back" : ""}` }, this.layers.tokens);
    el("circle", { r: 7 }, tok);
    let bg;
    let t;
    if (label) {
      bg = el("rect", { rx: 9 }, tok);
      t = el("text", { "text-anchor": "middle", y: -18 }, tok);
      t.textContent = label;
      const bb = t.getBBox();
      bg.setAttribute("x", bb.x - 8);
      bg.setAttribute("y", bb.y - 3);
      bg.setAttribute("width", bb.width + 16);
      bg.setAttribute("height", bb.height + 6);
    }

    await new Promise((resolve, reject) => {
      let elapsed = 0;
      let last = performance.now();
      const tick = (now) => {
        if (id !== this.runId) { tok.remove(); return reject(new Cancelled()); }
        if (!this.paused) elapsed += now - last;
        last = now;
        const k = Math.min(1, elapsed / dur);
        const ease = k < 0.5 ? 2 * k * k : 1 - Math.pow(-2 * k + 2, 2) / 2;
        const p = path.getPointAtLength((opts.back ? 1 - ease : ease) * total);
        tok.setAttribute("transform", `translate(${p.x},${p.y})`);
        if (k < 1) requestAnimationFrame(tick);
        else resolve();
      };
      requestAnimationFrame(tick);
    });
    await this.sleep(220, id);
    tok.remove();
    edge.el.classList.remove("is-on");
  }
}

window.Scene = Scene;
