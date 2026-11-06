// @ts-check
import { Component } from "../core/component.js";
import { El } from "../core/el.js";
import { Prefs } from "../core/prefs.js";

/**
 * @typedef {{ field: string, as?: string }} Column
 * @typedef {{ field: string, desc?: boolean }} SortKey
 * @typedef {Record<string, string[] | string | undefined>} Where
 * @typedef {{ version: 1, grain: string, where: Where, narrow?: boolean, columns?: Column[], sort?: SortKey[], limit?: number }} Selection
 * @typedef {{ value: string, profile?: string, rows: number }} FacetValue
 * @typedef {{ name: string, rows: number }} Stage
 * @typedef {{ name: string, label: string, extension: string }} Format
 * @typedef {{ grain: string, columns: string[], rows: string[][], total: number, kept: number, stages: Stage[],
 *   fields: string[], facets: Record<string, FacetValue[]>, formats: Format[], defaults: Column[] }} Preview
 */

/** The facets the page offers, in its order: heading, and one value's name. */
const FACETS = [
  ["profile", "Profiles", "Profile"],
  ["key", "Keys", "Key"],
  ["env", "Environments", "Environment"],
  ["role", "Roles", "Role"],
  ["tag", "Tags", "Tag"],
  ["status", "Status", "Status"],
  ["target", "Tools", "Tool"],
  ["kind", "Kinds", "Kind"],
  ["area", "Areas", "Area"],
  ["source", "Sources", "Source"],
];

/** The grains whose rows have a time, so since/until apply. */
const TIMED = ["op", "event", "snapshot", "deployment"];

/** The formats each option applies to. */
const APPLIES = {
  header: ["csv", "tsv", "markdown", "html"],
  compact: ["json", "js", "php"],
  table: ["json", "js", "php", "yaml"],
};

/** base64url of a string, UTF-8 safe. @param {string} s */
function b64url(s) {
  const bytes = new TextEncoder().encode(s);
  let bin = "";
  for (const b of bytes) {
    bin += String.fromCharCode(b);
  }
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/** The string a base64url value holds. @param {string} s */
function unb64url(s) {
  const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
  return new TextDecoder().decode(Uint8Array.from(bin, (c) => c.charCodeAt(0)));
}

/**
 * ExportPage drives the Export page: one selection is its state, kept in the
 * address (#s=…) so a reload, back and a shared link show the same file. Each
 * change asks /exports/run?preview=1 and redraws the filters, columns and
 * first rows; the download, copy and link all use the same selection.
 *
 *   <div data-export-page data-run="/exports/run">…</div>
 *
 * @extends {Component<{ delay: number }>}
 */
export class ExportPage extends Component {
  static attribute = "export-page";
  static defaults = { delay: 250 };

  /** @type {Selection} */
  #sel = { version: 1, grain: "host", where: {} };

  /** @type {Preview | null} */
  #p = null;

  #format = "csv";

  #timer = 0;

  /** @type {AbortController | null} */
  #inflight = null;

  init() {
    this.#read();
    this.#bind();
    this.#refresh();
    window.addEventListener("hashchange", () => {
      this.#read();
      this.#refresh();
    });
    return this;
  }

  /** @param {string} name @returns {HTMLElement} */
  #part(name) {
    return /** @type {HTMLElement} */ (this.el.querySelector(`[data-export-${name}]`));
  }

  /** Reads the selection from the address, or the last one used. */
  #read() {
    const m = /[#&]s=([A-Za-z0-9_-]+)/.exec(location.hash);
    /** @type {any} */
    let s = null;
    if (m) {
      try {
        s = JSON.parse(unb64url(m[1]));
      } catch {
        s = null;
      }
    }
    s ??= Prefs.get("export.selection", null);
    if (s && s.version === 1 && typeof s.grain === "string") {
      this.#sel = { version: 1, grain: s.grain, where: s.where ?? {}, narrow: s.narrow, columns: s.columns, sort: s.sort };
    }
    const f = /[#&]f=([a-z]+)/.exec(location.hash);
    this.#format = f ? f[1] : Prefs.get("export.format", "csv");
  }

  /** Records the selection in the address and this browser. */
  #write() {
    const hash = `#s=${this.#encoded()}&f=${this.#format}`;
    if (location.hash !== hash) {
      history.replaceState(null, "", hash);
    }
    Prefs.set("export.selection", this.#sel);
    Prefs.set("export.format", this.#format);
  }

  /** The selection to send: empty filters and default columns left out. */
  #clean() {
    /** @type {Where} */
    const where = {};
    for (const [k, v] of Object.entries(this.#sel.where)) {
      if (Array.isArray(v) ? v.length : typeof v === "string" && v.trim()) {
        where[k] = Array.isArray(v) ? v : String(v).trim();
      }
    }
    /** @type {Selection} */
    const out = { version: 1, grain: this.#sel.grain, where };
    if (this.#sel.grain === "profile" && this.#sel.narrow !== false && where.key) {
      out.narrow = true;
    }
    if (this.#sel.columns?.length) {
      out.columns = this.#sel.columns;
    }
    if (this.#sel.sort?.length) {
      out.sort = this.#sel.sort;
    }
    return out;
  }

  #encoded() {
    return b64url(JSON.stringify(this.#clean()));
  }

  /** The run URL for the selection. @param {boolean} preview */
  #url(preview) {
    const run = this.el.dataset.run ?? "/exports/run";
    const u = new URL(run, location.href);
    u.searchParams.set("s", this.#encoded());
    if (preview) {
      u.searchParams.set("preview", "1");
      return u;
    }
    u.searchParams.set("format", this.#format);
    const opt = (/** @type {string} */ n) => /** @type {HTMLInputElement} */ (this.el.querySelector(`[data-export-opt="${n}"]`));
    if (APPLIES.header.includes(this.#format) && !opt("header").checked) {
      u.searchParams.set("header", "0");
    }
    if (APPLIES.compact.includes(this.#format) && opt("compact").checked) {
      u.searchParams.set("compact", "1");
    }
    if (APPLIES.table.includes(this.#format) && opt("table").checked) {
      u.searchParams.set("shape", "table");
    }
    return u;
  }

  /** @param {Partial<Selection>} change @param {boolean} [later] */
  #update(change, later = false) {
    this.#sel = { ...this.#sel, ...change };
    window.clearTimeout(this.#timer);
    if (later) {
      this.#timer = window.setTimeout(() => this.#refresh(), Number(this.options.delay));
    } else {
      this.#refresh();
    }
  }

  /** @param {string} facet @param {string[] | null} values */
  #setFacet(facet, values) {
    const where = { ...this.#sel.where };
    if (values === null) {
      delete where[facet];
    } else {
      where[facet] = values;
    }
    // Choosing profiles drops the chosen keys of profiles no longer chosen.
    if (facet === "profile" && Array.isArray(where.key) && this.#p) {
      const keep = new Set((this.#p.facets.key ?? []).filter((k) => !values || values.includes(k.profile ?? "")).map((k) => k.value));
      where.key = where.key.filter((k) => keep.has(k));
    }
    this.#update({ where });
  }

  #bind() {
    for (const b of /** @type {HTMLElement[]} */ ([...this.el.querySelectorAll("[data-export-grain]")])) {
      b.addEventListener("click", (e) => {
        e.preventDefault(); // a tab link, not a navigation
        const grain = b.dataset.exportGrain ?? "host";
        if (grain !== this.#sel.grain) {
          this.#sel = { version: 1, grain, where: {} };
          for (const t of /** @type {HTMLInputElement[]} */ ([...this.el.querySelectorAll("[data-export-text]")])) {
            t.value = "";
          }
          this.#refresh();
        }
      });
    }
    for (const t of /** @type {HTMLInputElement[]} */ ([...this.el.querySelectorAll("[data-export-text]")])) {
      t.addEventListener("input", () => {
        this.#update({ where: { ...this.#sel.where, [t.dataset.exportText ?? ""]: t.value } }, true);
      });
    }
    this.#part("clear").addEventListener("click", () => {
      for (const t of /** @type {HTMLInputElement[]} */ ([...this.el.querySelectorAll("[data-export-text]")])) {
        t.value = "";
      }
      this.#update({ where: {} });
    });
    this.#part("columns-reset").addEventListener("click", () => this.#update({ columns: undefined, sort: undefined }));
    const format = /** @type {HTMLSelectElement} */ (this.#part("format"));
    format.addEventListener("change", () => {
      this.#format = format.value;
      this.#renderOutput();
    });
    for (const o of /** @type {HTMLInputElement[]} */ ([...this.el.querySelectorAll("[data-export-opt]")])) {
      o.addEventListener("change", () => {
        if (o.dataset.exportOpt === "narrow") {
          this.#update({ narrow: o.checked });
        } else {
          this.#renderOutput();
        }
      });
    }
    this.#part("copy").addEventListener("click", () => this.#copy());
    this.#part("share").addEventListener("click", () => this.#share());
    const download = this.#part("download");
    download.addEventListener("click", (e) => {
      if (download.getAttribute("aria-disabled") === "true") {
        e.preventDefault();
      }
    });
  }

  async #refresh() {
    this.#write();
    this.#inflight?.abort();
    this.#inflight = new AbortController();
    try {
      const res = await fetch(this.#url(true), { credentials: "same-origin", signal: this.#inflight.signal });
      const body = await res.json();
      if (!res.ok) {
        throw new Error(body.error ?? `the server answered ${res.status}`);
      }
      this.#p = /** @type {Preview} */ (body);
      this.#render();
    } catch (err) {
      if (!(err instanceof DOMException && err.name === "AbortError")) {
        this.#part("summary").textContent = `The preview could not be loaded: ${err instanceof Error ? err.message : err}`;
      }
    }
  }

  #render() {
    const p = /** @type {Preview} */ (this.#p);
    for (const b of /** @type {HTMLElement[]} */ ([...this.el.querySelectorAll("[data-export-grain]")])) {
      const on = b.dataset.exportGrain === this.#sel.grain;
      b.setAttribute("aria-selected", String(on));
      b.classList.toggle("active", on);
    }
    for (const t of /** @type {HTMLInputElement[]} */ ([...this.el.querySelectorAll("[data-export-text]")])) {
      const v = this.#sel.where[t.dataset.exportText ?? ""];
      if (document.activeElement !== t) {
        t.value = typeof v === "string" ? v : "";
      }
    }
    this.#part("time").hidden = !TIMED.includes(this.#sel.grain);
    this.#renderFacets(p);
    this.#renderColumns(p);
    this.#renderPreview(p);
    this.#renderOutput();
  }

  /** An icon from the page's sprite. @param {string} name */
  static icon(name) {
    const ns = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(ns, "svg");
    svg.setAttribute("class", "icon");
    svg.setAttribute("aria-hidden", "true");
    svg.setAttribute("focusable", "false");
    const use = document.createElementNS(ns, "use");
    use.setAttribute("href", `#tabler-${name}`);
    svg.appendChild(use);
    return svg;
  }

  /** The facets as a Tabler accordion, one item each, open as they were. @param {Preview} p */
  #renderFacets(p) {
    const box = this.#part("facets");
    const open = new Set([...box.querySelectorAll(".accordion-collapse.show")].map((e) => e.id));
    const first = box.children.length === 0;
    /** @type {Node[]} */
    const items = [];
    const profiles = /** @type {string[] | undefined} */ (this.#sel.where.profile);
    for (const [facet, heading, one] of FACETS) {
      let values = p.facets[facet] ?? [];
      if (facet === "key" && profiles) {
        values = values.filter((v) => !v.profile || profiles.includes(v.profile));
      }
      if (values.length === 0 || (values.length === 1 && facet !== "key" && facet !== "profile")) {
        continue;
      }
      const id = `export-facet-${facet}`;
      items.push(this.#facet(facet, heading, one, values, id, first ? items.length < 2 : open.has(id)));
    }
    box.replaceChildren(...items);
  }

  /**
   * One facet: an accordion item holding a list-group, a check box per
   * value with its count, an "All" box that ticks all, none or some, and
   * keys under a header per profile when several show.
   * @param {string} facet @param {string} heading @param {string} one @param {FacetValue[]} values
   * @param {string} id @param {boolean} expanded
   */
  #facet(facet, heading, one, values, id, expanded) {
    const chosen = /** @type {string[] | undefined} */ (this.#sel.where[facet]);
    const on = (/** @type {string} */ v) => !chosen || chosen.includes(v);
    const ticked = values.filter((v) => on(v.value)).length;
    const all = values.map((v) => v.value);
    /** @param {string[]} next */
    const commit = (next) => this.#setFacet(facet, all.every((v) => next.includes(v)) ? null : next);
    const head = El.create("input").cls("form-check-input m-0 me-2").prop("type", "checkbox").attr("aria-label", `All ${heading.toLowerCase()}`)
      .on("change", (e) => commit(/** @type {HTMLInputElement} */ (e.target).checked ? all : []));
    head.node.checked = ticked === values.length;
    head.node.indeterminate = ticked > 0 && ticked < values.length;
    const several = new Set(values.map((v) => v.profile ?? "")).size > 1;
    /** @type {Node[]} */
    const list = [
      El.create("label").cls("list-group-item d-flex align-items-center bg-body-tertiary").append(
        head, El.create("span").cls("fw-medium").text(`All ${heading.toLowerCase()}`),
        El.create("span").cls("ms-auto text-secondary small").text(`${ticked} of ${values.length}`),
      ).node,
    ];
    let group = "";
    values.forEach((v) => {
      if (several && v.profile && v.profile !== group) {
        group = v.profile;
        list.push(El.create("div").cls("list-group-header sticky-top").text(group).node);
      }
      const box = El.create("input").cls("form-check-input list-group-selectable-check m-0").prop("type", "checkbox").prop("checked", on(v.value))
        .attr("aria-label", v.value)
        .on("change", (e) => {
          const rest = all.filter(on).filter((x) => x !== v.value);
          commit(/** @type {HTMLInputElement} */ (e.target).checked ? [...rest, v.value] : rest);
        });
      // Tabler's selectable list item: the whole row is the label, and a
      // ticked box gives it the active look.
      list.push(El.create("label").cls("list-group-item").attr("title", v.value).append(
        El.create("span").cls("list-group-item-media").append(box),
        El.create("span").cls("list-group-item-content text-truncate").append(El.create("span").cls("list-group-item-title text-truncate").text(v.value)),
        El.create("span").cls("badge bg-secondary-lt ms-auto flex-shrink-0").attr("aria-label", `${v.rows} ${one.toLowerCase()} rows`).text(String(v.rows)),
      ).node);
    });
    const state = ticked === 0 ? "bg-red-lt" : ticked === values.length ? "bg-secondary-lt" : "bg-blue-lt";
    const button = El.create("button").cls(`accordion-button${expanded ? "" : " collapsed"}`).prop("type", "button")
      .attr("data-bs-toggle", "collapse").attr("data-bs-target", `#${id}`).attr("aria-expanded", String(expanded)).attr("aria-controls", id)
      .append(El.create("span").text(heading), El.create("span").cls(`badge ${state} ms-2`).text(ticked === values.length ? String(values.length) : `${ticked}/${values.length}`));
    return El.create("div").cls("accordion-item").append(
      El.create("h4").cls("accordion-header").append(button),
      El.create("div").cls(`accordion-collapse collapse${expanded ? " show" : ""}`).prop("id", id).append(
        El.create("div").cls("accordion-body p-0").append(El.create("div").cls("list-group list-group-flush list-group-selectable export-list").append(...list)),
      ),
    ).node;
  }

  /** The columns the file has: the chosen ones, or the defaults. */
  #columns() {
    return this.#sel.columns?.length ? this.#sel.columns : (this.#p?.defaults ?? []);
  }

  /**
   * The file's columns, in order: each with a heading to rename it and icon
   * buttons to move or remove it; below, a list of every other field to add.
   * @param {Preview} p
   */
  #renderColumns(p) {
    const cols = this.#columns();
    this.#part("colcount").textContent = `${cols.length} of ${p.fields.length}`;
    const set = (/** @type {Column[]} */ next) => this.#update({ columns: next });
    const move = (/** @type {number} */ i, /** @type {number} */ d) => {
      const next = [...cols];
      const [c] = next.splice(i, 1);
      next.splice(Math.max(0, Math.min(next.length, i + d)), 0, c);
      set(next);
    };
    const iconBtn = (/** @type {string} */ icon, /** @type {string} */ label, /** @type {() => void} */ fn, /** @type {boolean} */ off) =>
      El.create("button").cls("btn btn-icon btn-sm btn-ghost-secondary").prop("type", "button").attr("aria-label", label).attr("title", label)
        .prop("disabled", off).append(ExportPage.icon(icon)).on("click", fn);
    const rows = cols.map((c, i) =>
      El.create("tr").append(
        El.create("td").cls("text-secondary w-1").text(String(i + 1)),
        El.create("td").append(El.create("code").text(c.field)),
        El.create("td").append(El.create("input").cls("form-control form-control-sm").prop("value", c.as ?? "").attr("placeholder", c.field)
          .attr("aria-label", `Heading for ${c.field}`)
          .on("change", (e) => {
            const v = /** @type {HTMLInputElement} */ (e.target).value.trim();
            set(cols.map((x, j) => (j === i ? (v ? { field: x.field, as: v } : { field: x.field }) : x)));
          })),
        El.create("td").cls("w-1").append(El.create("div").cls("btn-list flex-nowrap").append(
          iconBtn("arrow-up", `Move ${c.field} up`, () => move(i, -1), i === 0),
          iconBtn("arrow-down", `Move ${c.field} down`, () => move(i, 1), i === cols.length - 1),
          iconBtn("x", `Remove ${c.field}`, () => set(cols.filter((_, j) => j !== i)), cols.length === 1),
        )),
      ).node);
    const table = El.create("table").cls("table table-vcenter card-table table-sm mb-0").append(
      El.create("thead").append(El.create("tr").append(
        El.create("th").attr("scope", "col").text("#"),
        El.create("th").attr("scope", "col").text("Field"),
        El.create("th").attr("scope", "col").text("Heading in the file"),
        El.create("th").attr("scope", "col").append(El.create("span").cls("visually-hidden").text("Move or remove")),
      )),
      El.create("tbody").append(...rows),
    );
    this.#part("columns").replaceChildren(table.node);

    const used = new Set(cols.map((c) => c.field));
    const rest = p.fields.filter((f) => !used.has(f));
    const add = /** @type {HTMLSelectElement} */ (this.#part("add"));
    add.replaceChildren(
      El.create("option").prop("value", "").text(rest.length ? `Add one of ${rest.length} more fields…` : "Every field is a column").node,
      ...rest.map((f) => El.create("option").prop("value", f).text(f).node),
    );
    add.disabled = rest.length === 0;
    const button = /** @type {HTMLButtonElement} */ (this.#part("add-button"));
    button.disabled = rest.length === 0;
    button.onclick = () => {
      if (add.value) {
        set([...cols, { field: add.value }]);
      }
    };
    add.onchange = () => button.click();
  }

  /** @param {Preview} p */
  #renderPreview(p) {
    const stage = (/** @type {string} */ n) => p.stages.find((s) => s.name === n)?.rows ?? 0;
    this.#part("summary").textContent =
      `${p.kept} of ${p.total} row${p.total === 1 ? "" : "s"} · ${p.columns.length} column${p.columns.length === 1 ? "" : "s"}` +
      (p.kept > p.rows.length ? ` · first ${p.rows.length} shown` : "");
    /** @type {Node[]} */
    const reasons = [];
    /** @param {number} n @param {string} what @param {string[]} facets */
    const reason = (n, what, facets) => {
      if (n <= 0) {
        return;
      }
      reasons.push(El.create("span").cls("badge bg-secondary-lt d-inline-flex align-items-center gap-1").append(
        document.createTextNode(`${n} ${what}`),
        El.create("button").cls("btn btn-sm btn-link p-0 border-0 lh-1").prop("type", "button").attr("aria-label", `Stop leaving out rows ${what}`)
          .text("×").on("click", () => {
            const where = { ...this.#sel.where };
            for (const f of facets) {
              delete where[f];
            }
            this.#update({ where });
          }),
      ).node);
    };
    reason(stage("all") - stage("profile"), "in other profiles", ["profile"]);
    reason(stage("profile") - stage("key"), "for other keys", ["key"]);
    reason(stage("key") - stage("facets"), "left out by filters", ["env", "role", "tag", "status", "target", "kind", "area", "source", "command", "since", "until"]);
    reason(stage("facets") - stage("text"), "matching the text", ["include", "exclude"]);
    this.#part("reasons").replaceChildren(...reasons);

    const cols = this.#columns();
    const sort = this.#sel.sort?.[0];
    const table = El.create("table").cls("table table-vcenter card-table table-striped mb-0");
    table.append(
      El.create("thead").append(El.create("tr").append(...p.columns.map((h, i) => {
        const field = cols[i]?.field ?? h;
        const dir = sort?.field === field ? (sort.desc ? "descending" : "ascending") : "none";
        // Tabler's sortable header: the button's asc/desc class draws the arrow.
        return El.create("th").attr("scope", "col").attr("aria-sort", dir).append(
          El.create("button").cls(`table-sort${dir === "ascending" ? " asc" : dir === "descending" ? " desc" : ""}`).prop("type", "button")
            .attr("data-sort", field).text(h)
            .on("click", () => this.#update({ sort: [{ field, desc: sort?.field === field && !sort.desc }] })),
        );
      }))),
      El.create("tbody").append(...(p.rows.length
        ? p.rows.map((r) => El.create("tr").append(...r.map((c) => El.create("td").attr("title", c).text(c))))
        : [El.create("tr").append(El.create("td").attr("colspan", String(Math.max(p.columns.length, 1))).cls("text-secondary text-center py-4").text("No rows match."))])),
    );
    this.#part("preview").replaceChildren(table.node);
  }

  /** Why nothing can be downloaded, or "". */
  #blocked() {
    for (const [facet, heading] of FACETS) {
      const v = this.#sel.where[facet];
      if (Array.isArray(v) && v.length === 0) {
        return `Tick at least one of ${heading.toLowerCase()}.`;
      }
    }
    return "";
  }

  #renderOutput() {
    const p = this.#p;
    const select = /** @type {HTMLSelectElement} */ (this.#part("format"));
    if (p && select.options.length !== p.formats.length) {
      select.replaceChildren(...p.formats.map((f) => El.create("option").prop("value", f.name).text(f.label).node));
    }
    if (p && !p.formats.some((f) => f.name === this.#format)) {
      this.#format = "csv";
    }
    select.value = this.#format;
    for (const o of /** @type {HTMLInputElement[]} */ ([...this.el.querySelectorAll("[data-export-opt]")])) {
      const name = /** @type {keyof typeof APPLIES | "narrow"} */ (o.dataset.exportOpt);
      const label = /** @type {HTMLElement} */ (o.closest("label"));
      label.hidden = name === "narrow" ? this.#sel.grain !== "profile" : !APPLIES[name].includes(this.#format);
      if (name === "narrow") {
        o.checked = this.#sel.narrow !== false;
      }
    }
    const why = this.#blocked();
    const download = /** @type {HTMLAnchorElement} */ (this.#part("download"));
    const note = this.#part("blocked");
    note.textContent = why;
    this.#part("blocked-row").hidden = why === "";
    for (const b of [this.#part("copy"), this.#part("share")]) {
      /** @type {HTMLButtonElement} */ (b).disabled = why !== "";
    }
    if (why) {
      download.removeAttribute("href");
      download.setAttribute("aria-disabled", "true");
      download.classList.add("disabled");
      return;
    }
    download.href = this.#url(false).toString();
    download.removeAttribute("aria-disabled");
    download.classList.remove("disabled");
    this.#write();
  }

  async #copy() {
    const btn = this.#part("copy");
    try {
      const res = await fetch(this.#url(false), { credentials: "same-origin" });
      if (!res.ok) {
        throw new Error(`the server answered ${res.status}`);
      }
      await navigator.clipboard.writeText(await res.text());
      this.#flash(btn, "Copied");
    } catch (err) {
      this.#flash(btn, `Could not copy: ${err instanceof Error ? err.message : err}`);
    }
  }

  async #share() {
    const btn = this.#part("share");
    try {
      await navigator.clipboard.writeText(location.href);
      this.#flash(btn, "Link copied");
    } catch {
      this.#flash(btn, "Could not copy the link");
    }
  }

  /** Says text in the summary for a moment. @param {HTMLElement} _btn @param {string} text */
  #flash(_btn, text) {
    const s = this.#part("summary");
    const was = s.textContent;
    s.textContent = text;
    window.setTimeout(() => {
      if (s.textContent === text) {
        s.textContent = was;
      }
    }, 2000);
  }
}
