// @ts-check
import { Checklist } from "../core/checklist.js";
import { Component } from "../core/component.js";
import { El } from "../core/el.js";
import { Prefs } from "../core/prefs.js";

/**
 * @typedef {{ field: string, as?: string }} Column
 * @typedef {{ field: string, desc?: boolean }} SortKey
 * @typedef {Record<string, string[] | string | undefined>} Where
 * @typedef {{ op: string, field?: string, as?: string }} Aggregate
 * @typedef {{ kind: string, field?: string, as?: string, template?: string, values?: Record<string, string>, default?: string,
 *   format?: string, layout?: string, sep?: string, rows?: boolean, by?: string[], aggregates?: Aggregate[], totals?: boolean }} Step
 * @typedef {{ version: 1, grain: string, where: Where, narrow?: boolean, transforms?: Step[], columns?: Column[], sort?: SortKey[], limit?: number }} Selection
 * @typedef {{ name: string, store: string, selection: Selection, shadowed?: boolean, error?: string }} Saved
 * @typedef {{ selections: Saved[], save_to: string, read_only: boolean }} SavedList
 * @typedef {{ value: string, profile?: string, rows: number }} FacetValue
 * @typedef {{ name: string, rows: number }} Stage
 * @typedef {{ name: string, label: string, extension: string, hosts?: boolean }} Format
 * @typedef {{ grain: string, columns: string[], rows: string[][], total: number, kept: number, stages: Stage[],
 *   fields: string[], facets: Record<string, FacetValue[]>, formats: Format[], defaults: Column[], skipped?: string[],
 *   steps: { kinds: string[], formats: string[], aggregates: string[] } }} Preview
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

/** What each transform is called on the page. */
const STEP_LABELS = {
  template: "Column from a template",
  map: "Map values",
  format: "Format",
  split: "Split",
  join: "Join a list",
  dedupe: "Remove duplicates",
  group: "Group",
  pivot: "Pivot",
};

/**
 * Whether a step has what it needs to run. An unfinished step stays on the
 * page and is left out of the selection until it is.
 * @param {Step} t
 */
export function stepReady(t) {
  switch (t.kind) {
    case "template":
      return Boolean(t.as && t.template);
    case "map":
      return Boolean(t.field && t.values && Object.keys(t.values).length);
    case "format":
      return Boolean(t.field && t.format);
    case "split":
    case "join":
      return Boolean(t.field);
    case "group":
      return Boolean(t.by?.length);
    case "pivot":
      return Boolean(t.by?.length && t.field);
    default:
      return true;
  }
}

/** The field a step writes that the grain does not have, or "". @param {Step} t */
function written(t) {
  return t.kind === "template" ? (t.as ?? "") : (t.as && t.as !== t.field ? t.as : "");
}

/** A comma list as values. @param {string} s */
function commaList(s) {
  return s.split(",").map((x) => x.trim()).filter(Boolean);
}

/**
 * Aggregates as the command line writes them: count, or op:field[=as].
 * @param {string} s @returns {Aggregate[]}
 */
export function parseAggregates(s) {
  return commaList(s).map((a) => {
    const [spec, as] = a.split("=", 2);
    const [op, field] = spec.split(":", 2);
    /** @type {Aggregate} */
    const out = { op: op.trim() };
    if (field?.trim()) {
      out.field = field.trim();
    }
    if (as?.trim()) {
      out.as = as.trim();
    }
    return out;
  });
}

/** @param {Aggregate[] | undefined} aggs */
function aggregatesText(aggs) {
  return (aggs ?? []).map((a) => `${a.op}${a.field ? `:${a.field}` : ""}${a.as ? `=${a.as}` : ""}`).join(", ");
}

/** A map step's values, one from=to a line. @param {string} s */
function parseValues(s) {
  /** @type {Record<string, string>} */
  const out = {};
  for (const line of s.split("\n")) {
    const i = line.indexOf("=");
    if (i > 0) {
      out[line.slice(0, i).trim()] = line.slice(i + 1).trim();
    }
  }
  return out;
}

/**
 * Fields grouped by the relation they follow: the row's own first, then one
 * group per prefix (key.fingerprint is under key), in the order they come.
 * @param {string[]} fields @returns {[string, string[]][]}
 */
export function fieldGroups(fields) {
  /** @type {Map<string, string[]>} */
  const groups = new Map([["", []]]);
  for (const f of fields) {
    const dot = f.indexOf(".");
    const g = dot > 0 ? f.slice(0, dot) : "";
    groups.set(g, [...(groups.get(g) ?? []), f]);
  }
  return [...groups].filter(([, fs]) => fs.length > 0);
}

/** The grains whose rows have a time, so since/until apply. */
const TIMED = ["op", "event", "snapshot", "deployment"];

/** The formats each option applies to; "git" applies to every host format. */
const APPLIES = {
  header: ["csv", "tsv", "markdown", "html"],
  compact: ["json", "js", "php"],
  table: ["json", "js", "php", "yaml"],
};

/** The command-line flag of each filter, as `authama export` names it. */
const FLAGS = {
  profile: "--profile",
  key: "--key",
  env: "--env",
  role: "--role",
  tag: "--tag",
  status: "--status",
  target: "--tool",
  kind: "--kind",
  area: "--area",
  source: "--source",
  command: "--command",
  include: "--include",
  exclude: "--exclude",
  since: "--since",
  until: "--until",
};

/** The filters whose flag takes a list, which the command line reads as CSV. */
const LISTS = ["profile", "key", "env", "role", "tag", "status", "target", "kind", "area", "source", "command"];

/**
 * A word for a POSIX shell: as it is when nothing in it is special, else in
 * single quotes, with a quote inside written '\''.
 * @param {string} s
 */
export function shellWord(s) {
  return /^[A-Za-z0-9_@%+=:,./-]+$/.test(s) ? s : `'${s.replace(/'/g, "'\\''")}'`;
}

/**
 * One value of a list flag. The command line splits a list flag's value as
 * CSV, so a value holding a comma or a quote goes in double quotes.
 * @param {string} s
 */
function listValue(s) {
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

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

  /** The saved selections, once loaded. @type {SavedList | null} */
  #saved = null;

  /** The step whose first field takes the focus after the next render. @type {number | null} */
  #focusStep = null;

  /** The column whose handle takes the focus after the next render. @type {number | null} */
  #focusHandle = null;

  #timer = 0;

  /** @type {AbortController | null} */
  #inflight = null;

  init() {
    this.#read();
    this.#bind();
    this.#refresh();
    void this.#loadSaved();
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
      this.#sel = { version: 1, grain: s.grain, where: s.where ?? {}, narrow: s.narrow, transforms: Array.isArray(s.transforms) ? s.transforms : undefined, columns: s.columns, sort: s.sort };
    }
    const f = /[#&]f=([a-z0-9_-]+)/.exec(location.hash);
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
    const steps = (this.#sel.transforms ?? []).filter(stepReady);
    if (steps.length) {
      out.transforms = steps;
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
    u.searchParams.set("format", this.#format);
    const opt = (/** @type {string} */ n) => /** @type {HTMLInputElement} */ (this.el.querySelector(`[data-export-opt="${n}"]`));
    if (this.#isHosts() && opt("git").checked) {
      u.searchParams.set("include_git", "1");
    }
    if (preview) {
      u.searchParams.set("preview", "1");
      return u;
    }
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
    const stepAdd = /** @type {HTMLSelectElement} */ (this.#part("step-add"));
    this.#part("step-add-button").addEventListener("click", () => {
      if (stepAdd.value) {
        /** @type {Step} */
        const step = { kind: stepAdd.value };
        if (step.kind === "format") {
          step.format = "upper";
        }
        if (step.kind === "group") {
          step.aggregates = [{ op: "count" }];
        }
        this.#focusStep = (this.#sel.transforms ?? []).length;
        this.#setSteps([...(this.#sel.transforms ?? []), step]);
        stepAdd.value = "";
      }
    });
    stepAdd.addEventListener("change", () => this.#part("step-add-button").click());
    this.#part("save")?.addEventListener("click", () => void this.#saveCurrent());
    this.#part("save-store")?.addEventListener("change", (e) => {
      /** @type {HTMLElement} */ (e.target).dataset.touched = "1";
    });
    this.#part("save-name")?.addEventListener("keydown", (e) => {
      if (/** @type {KeyboardEvent} */ (e).key === "Enter") {
        e.preventDefault();
        void this.#saveCurrent();
      }
    });
    this.#part("columns-reset").addEventListener("click", () => this.#update({ columns: undefined, sort: undefined }));
    const format = /** @type {HTMLSelectElement} */ (this.#part("format"));
    format.addEventListener("change", () => {
      const was = this.#isHosts();
      this.#format = format.value;
      // A host format can leave hosts out, which the preview says: ask again.
      if (was || this.#isHosts()) {
        this.#refresh();
      } else {
        this.#renderOutput();
      }
    });
    for (const o of /** @type {HTMLInputElement[]} */ ([...this.el.querySelectorAll("[data-export-opt]")])) {
      o.addEventListener("change", () => {
        if (o.dataset.exportOpt === "narrow") {
          this.#update({ narrow: o.checked });
        } else if (o.dataset.exportOpt === "git") {
          this.#refresh();
        } else {
          this.#renderOutput();
        }
      });
    }
    this.#part("copy").addEventListener("click", () => this.#copy());
    this.#part("share").addEventListener("click", () => this.#share());
    this.#part("command").addEventListener("click", () => this.#copyCommand());
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
    this.#renderSteps(p);
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
    const pick = new Checklist(values.map((v) => v.value), /** @type {string[] | undefined} */ (this.#sel.where[facet]));
    const ticked = pick.ticked;
    const head = El.create("input").cls("form-check-input m-0 me-2").prop("type", "checkbox").attr("aria-label", `All ${heading.toLowerCase()}`)
      .on("change", (e) => this.#setFacet(facet, pick.setAll(/** @type {HTMLInputElement} */ (e.target).checked)));
    head.node.checked = pick.all.checked;
    head.node.indeterminate = pick.all.indeterminate;
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
        list.push(El.create("div").cls("list-group-header").text(group).node);
      }
      const box = El.create("input").cls("form-check-input list-group-selectable-check m-0").prop("type", "checkbox").prop("checked", pick.on(v.value))
        .attr("aria-label", v.value)
        .on("change", (e) => this.#setFacet(facet, pick.toggle(v.value, /** @type {HTMLInputElement} */ (e.target).checked)));
      // Tabler's selectable list item: the whole row is the label, and a
      // ticked box gives it the active look.
      list.push(El.create("label").cls("list-group-item").attr("title", v.value).append(
        El.create("span").cls("list-group-item-media").append(box),
        El.create("span").cls("list-group-item-content text-truncate").append(El.create("span").cls("list-group-item-title text-truncate").text(v.value)),
        El.create("span").cls("badge badge-muted ms-auto flex-shrink-0").attr("aria-label", `${v.rows} ${one.toLowerCase()} rows`).text(String(v.rows)),
      ).node);
    });
    const state = pick.badge.state;
    const button = El.create("button").cls(`accordion-button${expanded ? "" : " collapsed"}`).prop("type", "button")
      .attr("data-bs-toggle", "collapse").attr("data-bs-target", `#${id}`).attr("aria-expanded", String(expanded)).attr("aria-controls", id)
      .append(El.create("span").text(heading), El.create("span").cls(`badge ${state} ms-2`).text(pick.badge.text),
        // Tabler draws an accordion's chevron only in this element.
        El.create("span").cls("accordion-button-toggle").attr("aria-hidden", "true"));
    return El.create("div").cls("accordion-item").append(
      El.create("h4").cls("accordion-header").append(button),
      El.create("div").cls(`accordion-collapse collapse${expanded ? " show" : ""}`).prop("id", id).append(
        El.create("div").cls("accordion-body p-0").append(El.create("div").cls("list-group list-group-flush list-group-selectable export-list").append(...list)),
      ),
    ).node;
  }

  /** The CSRF token the page carries for its POSTs. */
  static csrf() {
    return document.querySelector('meta[name="csrf-token"]')?.getAttribute("content") ?? "";
  }

  /** Loads the saved selections, both stores. */
  async #loadSaved() {
    const url = this.el.dataset.savedUrl;
    if (!url) {
      return;
    }
    try {
      const res = await fetch(url, { credentials: "same-origin" });
      if (!res.ok) {
        throw new Error(`the server answered ${res.status}`);
      }
      this.#saved = /** @type {SavedList} */ (await res.json());
      const store = /** @type {HTMLSelectElement | null} */ (this.#part("save-store"));
      if (store && !store.dataset.touched) {
        store.value = this.#saved.save_to;
      }
      this.#renderSaved();
    } catch (err) {
      this.#part("saved-empty").textContent = `The saved selections could not be loaded: ${err instanceof Error ? err.message : err}`;
    }
  }

  /**
   * The saved selections as a list: a button that opens each, its store,
   * what its rows are, and a button to delete it.
   */
  #renderSaved() {
    const list = this.#saved?.selections ?? [];
    this.#part("saved-count").textContent = list.length ? String(list.length) : "";
    this.#part("saved-empty").hidden = list.length > 0;
    const ro = this.#saved?.read_only ?? true;
    this.#part("saved").replaceChildren(...list.map((s) => {
      const where = s.store === "manifest" ? "manifest" : "mine";
      const note = s.error ? `Cannot be read: ${s.error}` : s.shadowed ? "Hidden by your own of the same name" : "";
      const open = El.create("button").cls("btn btn-link p-0 text-start text-truncate flex-fill").prop("type", "button")
        .prop("disabled", Boolean(s.error) || Boolean(s.shadowed)).attr("title", note || `Open ${s.name}`)
        .text(s.name).on("click", () => this.#openSaved(s));
      const item = El.create("div").cls("list-group-item").attr("data-saved", s.name).append(
        open,
        El.create("span").cls(`badge ${s.store === "manifest" ? "badge-info" : "badge-muted"}`).text(where),
        El.create("span").cls("text-secondary small").text(s.selection?.grain ?? ""),
      );
      if (note) {
        item.append(El.create("span").cls("badge badge-warning").attr("title", note).text(s.error ? "unreadable" : "hidden"));
      } else {
        item.append(El.create("button").cls("btn btn-icon btn-sm btn-ghost-secondary").prop("type", "button")
          .attr("aria-label", `Compare with ${s.name}`).attr("title", `Compare with ${s.name}`)
          .append(ExportPage.icon("git-compare")).on("click", () => void this.#compare(s)));
      }
      if (!ro) {
        item.append(El.create("button").cls("btn btn-icon btn-sm btn-ghost-secondary").prop("type", "button")
          .attr("aria-label", `Delete ${s.name} (${where})`).attr("title", `Delete ${s.name} (${where})`)
          .append(ExportPage.icon("x")).on("click", () => void this.#deleteSaved(s)));
      }
      return item.node;
    }));
  }

  /**
   * Compares the page's selection with a saved one: rows matched on the
   * columns both have, and the rows only one holds, side by side.
   * @param {Saved} s
   */
  async #compare(s) {
    const box = this.#part("compare");
    const u = new URL(this.el.dataset.compareUrl ?? "/exports/compare", location.href);
    u.searchParams.set("a", this.#encoded());
    u.searchParams.set("b", b64url(JSON.stringify(s.selection)));
    box.hidden = false;
    try {
      const res = await fetch(u, { credentials: "same-origin" });
      const c = await res.json();
      if (!res.ok) {
        throw new Error(c.error ?? `the server answered ${res.status}`);
      }
      /** @param {string} title @param {string[][]} rows @param {number} count */
      const side = (title, rows, count) => El.create("div").cls("col-md-6").append(
        El.create("h4").cls("mb-2").text(`${title} (${count})`),
        El.create("div").cls("table-responsive border rounded export-compare-side").append(
          El.create("table").cls("table table-sm card-table mb-0").append(
            El.create("thead").append(El.create("tr").append(...c.on.map((/** @type {string} */ h) => El.create("th").attr("scope", "col").text(h)))),
            El.create("tbody").append(...(rows.length
              ? rows.map((r) => El.create("tr").append(...r.map((x) => El.create("td").text(x))))
              : [El.create("tr").append(El.create("td").attr("colspan", String(c.on.length)).cls("text-secondary").text("None."))])),
          ),
        ),
        count > rows.length ? El.create("small").cls("text-secondary").text(`First ${rows.length} shown.`) : El.create("span"),
      );
      box.replaceChildren(
        El.create("div").cls("d-flex align-items-center mb-2").append(
          El.create("h3").cls("card-title mb-0").append(ExportPage.icon("git-compare"), El.create("span").cls("ms-2").text(`Compared with ${s.name}`)),
          El.create("button").cls("btn-close ms-auto").prop("type", "button").attr("aria-label", "Close the comparison").on("click", () => {
            box.hidden = true;
            box.replaceChildren();
          }),
        ).node,
        El.create("p").cls("text-secondary small").text(
          `Matched on ${c.on.join(", ")}: ${c.both} in both, ${c.only_a_count} only here, ${c.only_b_count} only in ${s.name}.`).node,
        El.create("div").cls("row g-3").append(side("Only here", c.only_a, c.only_a_count), side(`Only in ${s.name}`, c.only_b, c.only_b_count)).node,
      );
    } catch (err) {
      box.replaceChildren(El.create("div").cls("alert alert-warning mb-0").attr("role", "alert")
        .text(`Not compared: ${err instanceof Error ? err.message : err}`).node);
    }
  }

  /** Opens a saved selection: the page shows and offers it. @param {Saved} s */
  #openSaved(s) {
    const x = s.selection;
    this.#sel = { version: 1, grain: x.grain, where: x.where ?? {}, narrow: x.narrow, transforms: x.transforms, columns: x.columns, sort: x.sort };
    const name = /** @type {HTMLInputElement | null} */ (this.#part("save-name"));
    if (name) {
      name.value = s.name;
    }
    this.#refresh();
    this.#flash(this.#part("saved-count"), `Opened ${s.name}`);
  }

  /**
   * Saves the selection under the name given. Mine is a POST; the manifest's
   * goes through the change preview, which the hidden form opens.
   */
  async #saveCurrent() {
    const nameBox = /** @type {HTMLInputElement} */ (this.#part("save-name"));
    const store = /** @type {HTMLSelectElement} */ (this.#part("save-store")).value;
    const name = nameBox.value.trim();
    const btn = this.#part("save");
    if (!/^[a-z0-9][a-z0-9._-]{0,62}$/.test(name)) {
      this.#flash(btn, "Name it with a-z, 0-9, '.', '_' and '-'");
      nameBox.focus();
      return;
    }
    const selection = this.#clean();
    if (store === "manifest") {
      const form = /** @type {HTMLFormElement} */ (this.#part("manifest-save"));
      /** @type {HTMLInputElement} */ (form.elements.namedItem("name")).value = name;
      /** @type {HTMLInputElement} */ (form.elements.namedItem("selection")).value = JSON.stringify(selection);
      form.submit();
      return;
    }
    try {
      const res = await fetch(this.el.dataset.savedUrl ?? "", {
        method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": ExportPage.csrf() },
        body: JSON.stringify({ name, selection }),
      });
      const body = await res.json();
      if (!res.ok) {
        throw new Error(body.error ?? `the server answered ${res.status}`);
      }
      this.#flash(btn, `Saved ${name}`);
      await this.#loadSaved();
    } catch (err) {
      this.#flash(btn, `Not saved: ${err instanceof Error ? err.message : err}`);
    }
  }

  /** Deletes a saved selection from its store. @param {Saved} s */
  async #deleteSaved(s) {
    if (s.store === "manifest") {
      const form = /** @type {HTMLFormElement} */ (this.#part("manifest-delete"));
      /** @type {HTMLInputElement} */ (form.elements.namedItem("name")).value = s.name;
      form.submit();
      return;
    }
    try {
      const res = await fetch(`${this.el.dataset.savedUrl ?? ""}/delete`, {
        method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": ExportPage.csrf() },
        body: JSON.stringify({ name: s.name }),
      });
      // Read the reply either way: an unread body is cut off, and reported
      // as a failed request, when the page navigates next.
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        throw new Error(body.error ?? `the server answered ${res.status}`);
      }
      await this.#loadSaved();
    } catch (err) {
      this.#flash(this.#part("saved-count"), `Not deleted: ${err instanceof Error ? err.message : err}`);
    }
  }

  /**
   * Sets the steps. Chosen columns naming a field no step writes any more are
   * dropped, and a group, which replaces the fields, starts the columns and
   * the sort afresh, so the preview never asks for a column that is gone.
   * @param {Step[]} next
   */
  #setSteps(next) {
    const was = this.#sel.transforms ?? [];
    /** @type {Partial<Selection>} */
    const change = { transforms: next.length ? next : undefined };
    // A group or a pivot replaces the fields.
    const grouped = (/** @type {Step[]} */ l) => l.some((t) => (t.kind === "group" || t.kind === "pivot") && stepReady(t));
    if (grouped(was) || grouped(next)) {
      change.columns = undefined;
      change.sort = undefined;
    } else if (this.#sel.columns?.length) {
      const still = new Set(next.filter(stepReady).map(written));
      const gone = new Set(was.map(written).filter((f) => f && !still.has(f)));
      const cols = this.#sel.columns.filter((c) => !gone.has(c.field));
      change.columns = cols.length ? cols : undefined;
    }
    this.#update(change);
  }

  /**
   * The transform steps, in order: each with its own fields, its row count
   * after it ran, and buttons to move or remove it; below, a list of the
   * transforms to add.
   * @param {Preview} p
   */
  #renderSteps(p) {
    const steps = this.#sel.transforms ?? [];
    const kinds = p.steps?.kinds ?? Object.keys(STEP_LABELS);
    const add = /** @type {HTMLSelectElement} */ (this.#part("step-add"));
    if (add.options.length !== kinds.length + 1) {
      add.replaceChildren(
        El.create("option").prop("value", "").text("Add a step…").node,
        ...kinds.map((k) => El.create("option").prop("value", k).text(STEP_LABELS[/** @type {keyof typeof STEP_LABELS} */ (k)] ?? k).node),
      );
    }
    this.#part("fieldlist").replaceChildren(...p.fields.map((f) => El.create("option").prop("value", f).node));
    this.#part("stepcount").textContent = steps.length ? String(steps.length) : "";
    let ran = 0;
    const items = steps.map((t, i) => {
      const ready = stepReady(t);
      const stage = ready ? p.stages.find((s) => s.name === `transform ${++ran} ${t.kind}`) : undefined;
      /** @param {Partial<Step>} change */
      // Read the steps when the change happens, not when this step was drawn:
      // the inputs are redrawn only after the preview answers, so a second
      // field committed before then would otherwise write back the steps as
      // they were and undo the first.
      const set = (change) => this.#setSteps((this.#sel.transforms ?? []).map((x, j) => (j === i ? { ...x, ...change } : x)));
      const n = i + 1;
      const label = STEP_LABELS[/** @type {keyof typeof STEP_LABELS} */ (t.kind)] ?? t.kind;
      /** A labelled input. @param {string} text @param {string} value @param {(v: string) => void} on @param {object} [o] */
      const input = (text, value, on, o = {}) => {
        const { list = false, placeholder = "", wide = false } = /** @type {{list?: boolean, placeholder?: string, wide?: boolean}} */ (o);
        const el = El.create("input").cls("form-control form-control-sm").prop("value", value).attr("placeholder", placeholder)
          .attr("aria-label", `Step ${n}: ${text}`).on("change", (e) => on(/** @type {HTMLInputElement} */ (e.target).value.trim()));
        if (list) {
          el.attr("list", "export-fields");
        }
        return El.create("label").cls(wide ? "col-12" : "col-sm-6 col-lg-4").append(El.create("span").cls("form-label small mb-1").text(text), el).node;
      };
      /** @param {string} text @param {string[]} options @param {string} value @param {(v: string) => void} on */
      const select = (text, options, value, on) =>
        El.create("label").cls("col-sm-6 col-lg-4").append(
          El.create("span").cls("form-label small mb-1").text(text),
          El.create("select").cls("form-select form-select-sm").attr("aria-label", `Step ${n}: ${text}`)
            .append(...options.map((o) => El.create("option").prop("value", o).prop("selected", o === value).text(o)))
            .on("change", (e) => on(/** @type {HTMLSelectElement} */ (e.target).value)),
        ).node;
      /** @param {string} text @param {boolean} value @param {(v: boolean) => void} on */
      const toggle = (text, value, on) =>
        El.create("label").cls("col-12 form-check form-switch mb-0 ms-2").append(
          El.create("input").cls("form-check-input").prop("type", "checkbox").prop("checked", value).attr("aria-label", `Step ${n}: ${text}`)
            .on("change", (e) => on(/** @type {HTMLInputElement} */ (e.target).checked)),
          El.create("span").cls("form-check-label").text(text),
        ).node;
      const into = (/** @type {string} */ v) => set({ as: v || undefined });
      /** @type {Node[]} */
      let body = [];
      switch (t.kind) {
        case "template":
          body = [input("Column name", t.as ?? "", into, { placeholder: "where" }),
            input("Template", t.template ?? "", (v) => set({ template: v }), { placeholder: "{profile}/{alias}", wide: true })];
          break;
        case "map": {
          const values = El.create("textarea").cls("form-control form-control-sm").attr("aria-label", `Step ${n}: values, one from=to a line`)
            .attr("placeholder", "needs-redeploy=pending")
            .prop("value", Object.entries(t.values ?? {}).map(([k, v]) => `${k}=${v}`).join("\n"))
            .on("change", (e) => set({ values: parseValues(/** @type {HTMLTextAreaElement} */ (e.target).value) }));
          body = [input("Field", t.field ?? "", (v) => set({ field: v }), { list: true }), input("Write to", t.as ?? "", into, { placeholder: "the same field" }),
            input("Any other value", t.default ?? "", (v) => set({ default: v || undefined }), { placeholder: "kept as it is" }),
            El.create("label").cls("col-12").append(El.create("span").cls("form-label small mb-1").text("Values, one from=to a line"), values).node];
          break;
        }
        case "format":
          body = [input("Field", t.field ?? "", (v) => set({ field: v }), { list: true }),
            select("Format", p.steps?.formats ?? ["upper"], t.format ?? "upper", (v) => set({ format: v })),
            input("Write to", t.as ?? "", into, { placeholder: "the same field" })];
          if (t.format === "date") {
            body.push(input("Layout", t.layout ?? "", (v) => set({ layout: v || undefined }), { placeholder: "2006-01-02" }));
          }
          break;
        case "split":
          body = [input("Field", t.field ?? "", (v) => set({ field: v }), { list: true }), input("Separator", t.sep ?? "", (v) => set({ sep: v || undefined }), { placeholder: "spaces" }),
            input("Write to", t.as ?? "", into, { placeholder: "the same field" }), toggle("One row per item", Boolean(t.rows), (v) => set({ rows: v || undefined }))];
          break;
        case "join":
          body = [input("Field", t.field ?? "", (v) => set({ field: v }), { list: true }), input("Separator", t.sep ?? "", (v) => set({ sep: v || undefined }), { placeholder: ", " }),
            input("Write to", t.as ?? "", into, { placeholder: "the same field" })];
          break;
        case "dedupe":
          body = [input("By these fields", (t.by ?? []).join(", "), (v) => set({ by: commaList(v) }), { placeholder: "every field", wide: true })];
          break;
        case "group":
          body = [input("Group by", (t.by ?? []).join(", "), (v) => set({ by: commaList(v) }), { placeholder: "profile, env", wide: true }),
            input("Aggregates", aggregatesText(t.aggregates), (v) => set({ aggregates: parseAggregates(v) }), { placeholder: `count, list:env=envs (${(p.steps?.aggregates ?? []).join(", ")})`, wide: true }),
            toggle("A totals row", Boolean(t.totals), (v) => set({ totals: v || undefined }))];
          break;
        case "pivot":
          body = [input("Rows by", (t.by ?? []).join(", "), (v) => set({ by: commaList(v) }), { placeholder: "profile" }),
            input("Columns from", t.field ?? "", (v) => set({ field: v }), { list: true, placeholder: "env" }),
            input("Cells", aggregatesText(t.aggregates), (v) => set({ aggregates: v ? parseAggregates(v).slice(0, 1) : undefined }), { placeholder: "count, or sum:port" }),
            toggle("A total column and row", Boolean(t.totals), (v) => set({ totals: v || undefined }))];
          break;
      }
      const iconBtn = (/** @type {string} */ icon, /** @type {string} */ text, /** @type {() => void} */ fn, /** @type {boolean} */ off) =>
        El.create("button").cls("btn btn-icon btn-sm btn-ghost-secondary").prop("type", "button").attr("aria-label", text).attr("title", text)
          .prop("disabled", off).append(ExportPage.icon(icon)).on("click", fn);
      const move = (/** @type {number} */ to) => {
        const next = [...(this.#sel.transforms ?? [])];
        const [s] = next.splice(i, 1);
        next.splice(to, 0, s);
        this.#setSteps(next);
      };
      const state = !ready
        ? El.create("span").cls("badge badge-warning ms-2").text("Not applied until filled in")
        : El.create("span").cls("badge badge-muted ms-2").attr("aria-label", `${stage?.rows ?? 0} rows after step ${n}`).text(`${stage?.rows ?? 0} rows`);
      return El.create("li").cls("list-group-item d-flex gap-2").attr("data-step", String(i)).append(
        El.create("div").cls("flex-fill").append(
          El.create("div").cls("d-flex align-items-center mb-2").append(
            El.create("strong").text(label), state,
            El.create("span").cls("ms-auto btn-list flex-nowrap").append(
              iconBtn("arrow-up", `Move step ${n} up`, () => move(i - 1), i === 0),
              iconBtn("arrow-down", `Move step ${n} down`, () => move(i + 1), i === steps.length - 1),
              iconBtn("x", `Remove step ${n}`, () => this.#setSteps((this.#sel.transforms ?? []).filter((_, j) => j !== i)), false),
            ),
          ),
          El.create("div").cls("row g-2").append(...body),
        ),
      ).node;
    });
    this.#part("steps").replaceChildren(...items);
    if (this.#focusStep !== null) {
      /** @type {HTMLElement | null} */ (this.#part("steps").querySelectorAll("[data-step]")[this.#focusStep]?.querySelector("input, select, textarea"))?.focus();
      this.#focusStep = null;
    }
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
    /**
     * Sets the columns; after a move, the moved column's handle keeps the
     * focus, so the arrow keys can carry on.
     * @param {Column[]} next @param {number} [_from] @param {number} [landed]
     */
    const set = (next, _from, landed) => {
      this.#focusHandle = landed ?? null;
      this.#update({ columns: next });
    };
    /** Moves the column at from to before index to (to may be cols.length). */
    const moveTo = (/** @type {number} */ from, /** @type {number} */ to) => {
      if (to === from || to === from + 1) {
        return;
      }
      const next = [...cols];
      const [c] = next.splice(from, 1);
      next.splice(to > from ? to - 1 : to, 0, c);
      set(next, from, to > from ? to - 1 : to);
    };
    const iconBtn = (/** @type {string} */ icon, /** @type {string} */ label, /** @type {() => void} */ fn, /** @type {boolean} */ off) =>
      El.create("button").cls("btn btn-icon btn-sm btn-ghost-secondary").prop("type", "button").attr("aria-label", label).attr("title", label)
        .prop("disabled", off).append(ExportPage.icon(icon)).on("click", fn);
    /** @type {number | null} */
    let dragging = null;
    const clearMarks = () => {
      for (const r of this.el.querySelectorAll(".export-drop-before, .export-drop-after")) {
        r.classList.remove("export-drop-before", "export-drop-after");
      }
    };
    const rows = cols.map((c, i) => {
      // The handle: drag the row by it, or, with it focused, move the column
      // with the arrow keys (Home and End to the ends).
      const handle = El.create("button").cls("btn btn-icon btn-sm btn-ghost-secondary export-handle").prop("type", "button")
        .attr("aria-label", `Reorder ${c.field}: drag, or press the up and down arrow keys`).attr("title", "Drag to reorder")
        .append(ExportPage.icon("grip-vertical"))
        .on("keydown", (e) => {
          const k = /** @type {KeyboardEvent} */ (e).key;
          const to = { ArrowUp: i - 1, ArrowDown: i + 2, Home: 0, End: cols.length }[k];
          if (to === undefined) {
            return;
          }
          e.preventDefault();
          moveTo(i, Math.max(0, Math.min(cols.length, to)));
        });
      const tr = El.create("tr").attr("draggable", "true").attr("data-index", String(i)).append(
        El.create("td").cls("w-1 ps-2 pe-0").append(handle),
        El.create("td").cls("text-secondary w-1").text(String(i + 1)),
        El.create("td").append(El.create("code").text(c.field)),
        El.create("td").append(El.create("input").cls("form-control form-control-sm").prop("value", c.as ?? "").attr("placeholder", c.field)
          .attr("aria-label", `Heading for ${c.field}`).attr("draggable", "false")
          .on("change", (e) => {
            const v = /** @type {HTMLInputElement} */ (e.target).value.trim();
            // The columns as they are now, not as drawn: see the steps' set.
            set(this.#columns().map((x, j) => (j === i ? (v ? { field: x.field, as: v } : { field: x.field }) : x)));
          })),
        El.create("td").cls("w-1").append(iconBtn("x", `Remove ${c.field}`, () => set(cols.filter((_, j) => j !== i)), cols.length === 1)),
      );
      tr.on("dragstart", (e) => {
        const ev = /** @type {DragEvent} */ (e);
        if (ev.target instanceof HTMLInputElement) {
          ev.preventDefault();
          return;
        }
        dragging = i;
        tr.node.classList.add("export-dragging");
        ev.dataTransfer?.setData("text/plain", c.field);
        if (ev.dataTransfer) {
          ev.dataTransfer.effectAllowed = "move";
        }
      });
      tr.on("dragend", () => {
        dragging = null;
        tr.node.classList.remove("export-dragging");
        clearMarks();
      });
      tr.on("dragover", (e) => {
        if (dragging === null) {
          return;
        }
        const ev = /** @type {DragEvent} */ (e);
        ev.preventDefault();
        const box = tr.node.getBoundingClientRect();
        const after = ev.clientY > box.top + box.height / 2;
        clearMarks();
        tr.node.classList.add(after ? "export-drop-after" : "export-drop-before");
      });
      tr.on("drop", (e) => {
        e.preventDefault();
        if (dragging === null) {
          return;
        }
        const after = tr.node.classList.contains("export-drop-after");
        const from = dragging;
        dragging = null;
        clearMarks();
        moveTo(from, after ? i + 1 : i);
      });
      return tr.node;
    });
    const table = El.create("table").cls("table table-vcenter card-table table-sm mb-0").append(
      El.create("thead").append(El.create("tr").append(
        El.create("th").attr("scope", "col").append(El.create("span").cls("visually-hidden").text("Reorder")),
        El.create("th").attr("scope", "col").text("#"),
        El.create("th").attr("scope", "col").text("Field"),
        El.create("th").attr("scope", "col").text("Heading in the file"),
        El.create("th").attr("scope", "col").append(El.create("span").cls("visually-hidden").text("Remove")),
      )),
      El.create("tbody").append(...rows),
    );
    this.#part("columns").replaceChildren(table.node);
    if (this.#focusHandle !== null) {
      /** @type {HTMLElement | null} */ (this.#part("columns").querySelectorAll(".export-handle")[this.#focusHandle])?.focus();
      this.#focusHandle = null;
    }

    const used = new Set(cols.map((c) => c.field));
    const rest = p.fields.filter((f) => !used.has(f));
    const add = /** @type {HTMLSelectElement} */ (this.#part("add"));
    add.replaceChildren(
      El.create("option").prop("value", "").text(rest.length ? `Add one of ${rest.length} more fields…` : "Every field is a column").node,
      ...fieldGroups(rest).map(([group, fields]) =>
        El.create("optgroup").attr("label", group ? `${group} (related)` : "This row").append(
          ...fields.map((f) => El.create("option").prop("value", f).text(group ? f.slice(group.length + 1) : f)),
        ).node),
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
      reasons.push(El.create("span").cls("tag").append(
        document.createTextNode(`${n} ${what}`),
        El.create("button").cls("btn-close").prop("type", "button").attr("aria-label", `Stop leaving out rows ${what}`)
          .attr("title", `Stop leaving out rows ${what}`).on("click", () => {
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
    if (p.skipped?.length) {
      // Left out by the format, not the filters: nothing to undo here but the
      // format, so the tag only says why, one host a line.
      const label = this.#p?.formats.find((f) => f.name === this.#format)?.label ?? this.#format;
      reasons.push(El.create("span").cls("tag").attr("title", p.skipped.join("\n"))
        .text(`${p.skipped.length} left out of the ${label} file`).node);
    }
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
      const name = /** @type {keyof typeof APPLIES | "narrow" | "git"} */ (o.dataset.exportOpt);
      const label = /** @type {HTMLElement} */ (o.closest("label"));
      label.hidden =
        name === "narrow" ? this.#sel.grain !== "profile" : name === "git" ? !this.#isHosts() : !APPLIES[name].includes(this.#format);
      if (name === "narrow") {
        o.checked = this.#sel.narrow !== false;
      }
    }
    const why = this.#blocked();
    const download = /** @type {HTMLAnchorElement} */ (this.#part("download"));
    const note = this.#part("blocked");
    note.textContent = why;
    this.#part("blocked-row").hidden = why === "";
    for (const b of [this.#part("copy"), this.#part("share"), this.#part("command")]) {
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

  /** Whether the chosen format writes hosts for an SSH client, not a table. */
  #isHosts() {
    return this.#p?.formats.find((f) => f.name === this.#format)?.hosts === true;
  }

  /**
   * The `authama export` command that writes the file this page would
   * download: the list, its filters, columns, sort, format and options.
   */
  command() {
    const sel = this.#clean();
    const tab = /** @type {HTMLElement | null} */ (this.el.querySelector(`[data-export-grain="${sel.grain}"]`));
    const args = [this.el.dataset.command ?? "authama", "export", tab?.dataset.preset || `${sel.grain}s`];
    for (const [k, v] of Object.entries(sel.where)) {
      const flag = FLAGS[/** @type {keyof typeof FLAGS} */ (k)];
      if (!flag || v === undefined) {
        continue;
      }
      for (const x of Array.isArray(v) ? v : [v]) {
        args.push(`${flag}=${LISTS.includes(k) ? listValue(x) : x}`);
      }
    }
    if (sel.narrow) {
      args.push("--narrow");
    }
    for (const t of sel.transforms ?? []) {
      args.push(`--transform=${JSON.stringify(t)}`);
    }
    for (const c of sel.columns ?? []) {
      args.push(`--columns=${listValue(c.as ? `${c.field}=${c.as}` : c.field)}`);
    }
    for (const k of sel.sort ?? []) {
      args.push(`--sort=${listValue(`${k.desc ? "-" : ""}${k.field}`)}`);
    }
    if (this.#format !== "csv") {
      args.push(`--format=${this.#format}`);
    }
    const opt = (/** @type {string} */ n) => /** @type {HTMLInputElement} */ (this.el.querySelector(`[data-export-opt="${n}"]`)).checked;
    if (APPLIES.header.includes(this.#format) && !opt("header")) {
      args.push("--no-header");
    }
    if (APPLIES.compact.includes(this.#format) && opt("compact")) {
      args.push("--compact");
    }
    if (APPLIES.table.includes(this.#format) && opt("table")) {
      args.push("--shape=table");
    }
    if (this.#isHosts() && opt("git")) {
      args.push("--include-git");
    }
    return args.map(shellWord).join(" ");
  }

  async #copyCommand() {
    const btn = this.#part("command");
    try {
      await navigator.clipboard.writeText(this.command());
      this.#flash(btn, "Command copied");
    } catch {
      this.#flash(btn, "Could not copy the command");
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
