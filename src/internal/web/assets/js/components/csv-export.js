// @ts-check
import { Component } from "../core/component.js";
import { El } from "../core/el.js";
import { Prefs } from "../core/prefs.js";
import { Ui } from "../core/ui.js";

/**
 * @typedef {{ name: string, rows: number }} CsvProfile
 * @typedef {{ name: string, profile?: string, rows: number, hosts?: number }} CsvKey
 * @typedef {{ profiles?: CsvProfile[], keys?: CsvKey[], key_list?: boolean, columns: string[], header: string[], rows: string[][],
 *   total: number, in_profiles: number, in_keys: number, kept: number }} CsvPreview
 * @typedef {{ drop: string[], exclude: string, profiles: string[] | null, keys: string[] | null, tab: string, narrow: boolean, format?: string }} CsvChoice
 * @typedef {"profiles" | "keys" | "columns"} CsvList
 * @typedef {{ value: string, rows?: number, sample?: string, group?: string }} PickItem
 * @typedef {{ label: string, always: boolean }} CountColumn
 */

/** The grain each list's preset is, by the list's name. */
const GRAINS = /** @type {Record<string, string>} */ ({
  hosts: "host", keys: "key", profiles: "profile", tokens: "token",
  history: "op", events: "event", snapshots: "snapshot", deployments: "deployment",
});

/** The heading of each list's name column, and the name of its select-all box. */
const LABELS = {
  profiles: { col: "Profile", all: "All profiles" },
  keys: { col: "Key", all: "All keys" },
  columns: { col: "Column", all: "All columns" },
};

/**
 * CsvExport turns a CSV download link into a dialog: a table per list
 * (profiles, keys, columns) to tick what the file holds, a box to leave out
 * rows containing some text, and the first rows as they will be. The server
 * does the shaping (?profile=, ?key=, ?drop=, ?exclude=, ?preview=1), so the
 * file is exactly what the preview showed. Without the script the link
 * downloads the whole file.
 *
 *   <a href="/exports/hosts.csv" download data-csv-export>CSV</a>
 *
 * Everything is ticked to start with, which means everything, including
 * profiles and keys added later. Untick to leave something out; ticking
 * nothing in a list disables the download and says why. The choice is
 * remembered per list in this browser.
 *
 * The dialog is the layout's one #csv-dialog (data-csv-dialog), shared by
 * every link on the page.
 *
 * @extends {Component<{ delay: number }>}
 */
export class CsvExport extends Component {
  static attribute = "csv-export";
  static defaults = { delay: 250 };

  /** @type {HTMLAnchorElement} */
  #link = /** @type {any} */ (null);

  /** @type {CsvChoice} */
  #choice = CsvExport.#fresh();

  /** The last preview, for re-rendering the lists without a fetch. @type {CsvPreview | null} */
  #preview = null;

  #timer = 0;

  /** @type {AbortController | null} */
  #inflight = null;

  init() {
    if (!(this.el instanceof HTMLAnchorElement) || !CsvExport.#dialog()) {
      return this;
    }
    this.#link = this.el;
    this.#link.addEventListener("click", (e) => {
      if (e.metaKey || e.ctrlKey || e.shiftKey) {
        return; // a modified click still just downloads
      }
      e.preventDefault();
      this.open();
    });
    return this;
  }

  /** @returns {CsvChoice} */
  static #fresh() {
    return { drop: [], exclude: "", profiles: null, keys: null, tab: "", narrow: true };
  }

  /** The list's name, from the link: the key its choice is kept under. */
  get name() {
    return new URL(this.#link.href).pathname.split("/").pop() ?? "export";
  }

  /** Opens the dialog for this link's list. */
  open() {
    const dialog = /** @type {HTMLElement} */ (CsvExport.#dialog());
    const saved = Prefs.get(`csv.${this.name}`, {});
    this.#choice = { ...CsvExport.#fresh(), ...saved };
    // An empty list once meant "all"; it now means "none", so an old choice
    // is read as it was meant.
    for (const list of /** @type {const} */ (["profiles", "keys"])) {
      if (Array.isArray(this.#choice[list]) && this.#choice[list].length === 0) {
        this.#choice[list] = null;
      }
    }
    this.#preview = null;
    CsvExport.#part(dialog, "name").textContent = this.name;
    const exclude = /** @type {HTMLInputElement} */ (CsvExport.#part(dialog, "exclude"));
    exclude.value = this.#choice.exclude;
    // The dialog is shared: rebind its controls to this link each time.
    exclude.oninput = () => this.#update({ exclude: exclude.value }, true);
    for (const list of /** @type {CsvList[]} */ (["profiles", "keys", "columns"])) {
      const find = /** @type {HTMLInputElement} */ (dialog.querySelector(`[data-csv-find="${list}"]`));
      find.value = "";
      find.oninput = () => this.#filter(dialog, list);
      const tab = /** @type {HTMLElement} */ (dialog.querySelector(`[data-csv-tab="${list}"]`));
      tab.onclick = () => this.#select(dialog, list, false);
      tab.onkeydown = (e) => this.#tabKey(dialog, e);
    }
    const narrow = /** @type {HTMLInputElement} */ (CsvExport.#part(dialog, "narrow"));
    narrow.onchange = () => this.#update({ narrow: narrow.checked });
    CsvExport.#part(dialog, "reset").onclick = () => {
      exclude.value = "";
      this.#update({ drop: [], exclude: "", profiles: null, keys: null, narrow: true });
    };
    const download = CsvExport.#part(dialog, "download");
    download.onclick = (e) => {
      if (download.getAttribute("aria-disabled") === "true") {
        e.preventDefault();
      }
    };
    Ui.modal(dialog)?.show();
    return this.#refresh();
  }

  /** @param {Partial<CsvChoice>} change @param {boolean} [later] */
  #update(change, later = false) {
    this.#choice = { ...this.#choice, ...change };
    Prefs.set(`csv.${this.name}`, this.#choice);
    window.clearTimeout(this.#timer);
    if (later) {
      this.#timer = window.setTimeout(() => this.#refresh(), Number(this.options.delay));
    } else {
      this.#refresh();
    }
    return this;
  }

  /** Why nothing can be downloaded, or "" when it can. */
  get blocked() {
    const p = this.#preview;
    if (this.#choice.profiles?.length === 0 && (p?.profiles?.length ?? 0) > 0) {
      return "Tick at least one profile.";
    }
    if (this.#choice.keys?.length === 0 && this.#shownKeys().length > 0) {
      return "Tick at least one key.";
    }
    if (p && p.columns.length > 0 && p.columns.every((c) => this.#choice.drop.includes(c))) {
      return "Tick at least one column.";
    }
    return "";
  }

  /**
   * The link's URL with the choice applied. An empty list is left off, so a
   * choice that ticks nothing still previews the lists to tick from.
   * @param {boolean} preview
   */
  url(preview) {
    const u = new URL(this.#link.href);
    for (const k of ["drop", "exclude", "preview", "profile", "key", "narrow"]) {
      u.searchParams.delete(k);
    }
    const { profiles, keys, drop, exclude } = this.#choice;
    if (profiles?.length) {
      u.searchParams.set("profile", profiles.join(","));
    }
    if (keys?.length) {
      u.searchParams.set("key", keys.join(","));
      if (this.#preview?.key_list && this.#choice.narrow) {
        u.searchParams.set("narrow", "1");
      }
    }
    const all = this.#preview?.columns ?? [];
    if (drop.length && !(all.length && all.every((c) => drop.includes(c)))) {
      u.searchParams.set("drop", drop.join(","));
    }
    if (exclude.trim()) {
      u.searchParams.set("exclude", exclude.trim());
    }
    if (preview) {
      u.searchParams.set("preview", "1");
    }
    return u;
  }

  async #refresh() {
    const dialog = /** @type {HTMLElement} */ (CsvExport.#dialog());
    this.#inflight?.abort();
    this.#inflight = new AbortController();
    try {
      const res = await fetch(this.url(true), { credentials: "same-origin", signal: this.#inflight.signal });
      if (!res.ok) {
        throw new Error(`the server answered ${res.status}`);
      }
      /** @type {CsvPreview} */
      const p = await res.json();
      this.#preview = p;
      this.#prune();
      this.#render(dialog);
    } catch (err) {
      if (!(err instanceof DOMException && err.name === "AbortError")) {
        CsvExport.#part(dialog, "summary").textContent = `The preview could not be loaded (${err}). The download still works.`;
        CsvExport.#part(dialog, "reasons").replaceChildren();
        this.#renderDownload(dialog);
      }
    }
    return this;
  }

  /** Drops from the choice what the list no longer offers. */
  #prune() {
    const p = /** @type {CsvPreview} */ (this.#preview);
    const names = (p.profiles ?? []).map((x) => x.name);
    const keys = (p.keys ?? []).map((x) => x.name);
    /** @param {string[] | null} list @param {string[]} offered */
    const keep = (list, offered) => (list === null ? null : list.filter((v) => offered.includes(v)));
    const next = {
      ...this.#choice,
      profiles: keep(this.#choice.profiles, names),
      keys: keep(this.#choice.keys, keys),
      drop: this.#choice.drop.filter((c) => p.columns.includes(c)),
    };
    if (JSON.stringify(next) !== JSON.stringify(this.#choice)) {
      this.#choice = next;
      Prefs.set(`csv.${this.name}`, next);
    }
  }

  /** The keys of the profiles ticked: every key when every profile is. */
  #shownKeys() {
    const chosen = this.#choice.profiles;
    return (this.#preview?.keys ?? []).filter((k) => chosen === null || !k.profile || chosen.includes(k.profile));
  }

  /** @param {HTMLElement} dialog */
  #render(dialog) {
    const p = /** @type {CsvPreview} */ (this.#preview);
    const profiles = p.profiles ?? [];
    const keys = this.#shownKeys();
    const several = new Set(keys.map((k) => k.profile ?? "")).size > 1;
    const first = p.rows[0] ?? [];
    this.#renderList(
      dialog,
      "profiles",
      profiles.map((x) => ({ value: x.name, rows: x.rows })),
      this.#choice.profiles,
      (profiles) => this.#update({ profiles, keys: this.#keysFor(profiles) }),
    );
    this.#renderList(
      dialog,
      "keys",
      keys.map((k) => ({ value: k.name, rows: k.hosts ?? k.rows, group: several ? k.profile : undefined })),
      this.#choice.keys,
      (keys) => this.#update({ keys }),
      p.key_list ? { label: "Hosts", always: true } : undefined,
    );
    this.#renderList(
      dialog,
      "columns",
      p.columns.map((c) => ({ value: c, sample: first[p.header.indexOf(c)] ?? "" })),
      p.columns.filter((c) => !this.#choice.drop.includes(c)),
      (kept) => this.#update({ drop: kept === null ? [] : p.columns.filter((c) => !kept.includes(c)) }),
    );
    // A list with profiles but no key column (the Termius CSV) says where
    // keys are chosen, rather than leaving the Keys tab silently missing.
    CsvExport.#part(dialog, "keys-note").hidden = !(profiles.length > 0 && !p.key_list && (p.keys ?? []).length === 0);
    this.#showTabs(dialog);
    this.#renderSummary(dialog);
    this.#renderPreview(dialog);
    this.#renderDownload(dialog);
  }

  /**
   * The keys to keep ticked when the profiles ticked change: every key stays
   * "all" while it is; otherwise a profile ticked brings its keys in and a
   * profile unticked takes its keys out.
   * @param {string[] | null} profiles
   */
  #keysFor(profiles) {
    const chosen = this.#choice.keys;
    if (chosen === null) {
      return null;
    }
    const all = this.#preview?.keys ?? [];
    const before = this.#choice.profiles;
    /** @param {string | undefined} prof @param {string[] | null} set */
    const inSet = (prof, set) => set === null || !prof || set.includes(prof);
    const added = all.filter((k) => inSet(k.profile, profiles) && !inSet(k.profile, before)).map((k) => k.name);
    const kept = chosen.filter((name) => {
      const k = all.find((x) => x.name === name);
      return !k || inSet(k.profile, profiles);
    });
    return [...new Set([...kept, ...added])];
  }

  /**
   * Renders one list as a table: a tick box per row, a header box that ticks
   * all, none or (indeterminate) some of the rows shown, a row count only
   * when some item has more than one row, and keys grouped under their
   * profile when several profiles are shown.
   * @param {HTMLElement} dialog @param {CsvList} list @param {PickItem[]} items
   * @param {string[] | null} chosen null when everything is ticked
   * @param {(chosen: string[] | null) => void} set
   * @param {CountColumn} [count] the count column's heading, and whether it shows even when every count is 1
   */
  #renderList(dialog, list, items, chosen, set, count = { label: "Rows", always: false }) {
    const tab = /** @type {HTMLElement} */ (dialog.querySelector(`[data-csv-tab="${list}"]`));
    tab.hidden = items.length === 0;
    if (items.length === 0) {
      return;
    }
    const values = items.map((it) => it.value);
    const on = (/** @type {string} */ v) => chosen === null || chosen.includes(v);
    const ticked = values.filter(on).length;
    /** @param {string[]} next */
    const commit = (next) => set(values.every((v) => next.includes(v)) ? null : next);
    const badge = /** @type {HTMLElement} */ (dialog.querySelector(`[data-csv-count="${list}"]`));
    badge.textContent = ticked === items.length ? String(items.length) : `${ticked}/${items.length}`;
    badge.className = `badge ms-1 ${ticked === 0 ? "badge-bad" : ticked === items.length ? "badge-muted" : "badge-info"}`;

    const counts = count.always || items.some((it) => (it.rows ?? 0) > 1);
    const samples = items.some((it) => it.sample);
    const all = El.create("input")
      .cls("form-check-input m-0 align-middle")
      .prop("type", "checkbox")
      .attr("aria-label", LABELS[list].all)
      .on("change", (e) => {
        const box = /** @type {HTMLInputElement} */ (e.target);
        const shown = this.#visible(dialog, list);
        const rest = values.filter(on).filter((v) => !shown.includes(v));
        commit(box.checked ? [...rest, ...shown] : rest);
      });
    all.node.checked = ticked === items.length;
    all.node.indeterminate = ticked > 0 && ticked < items.length;
    const head = /** @type {HTMLElement} */ (dialog.querySelector(`[data-csv-head="${list}"]`));
    head.replaceChildren(
      El.create("th").attr("scope", "col").append(all).node,
      El.create("th").attr("scope", "col").text(LABELS[list].col).node,
      ...(counts ? [El.create("th").attr("scope", "col").cls("csv-num").text(count.label).node] : []),
      ...(samples ? [El.create("th").attr("scope", "col").text("First row").node] : []),
    );

    const width = 2 + (counts ? 1 : 0) + (samples ? 1 : 0);
    /** @type {Node[]} */
    const rows = [];
    let group = "";
    items.forEach((it, i) => {
      if (it.group && it.group !== group) {
        group = it.group;
        rows.push(El.create("tr").cls("csv-group").attr("data-csv-group", "").append(El.create("td").cls("subheader").attr("colspan", String(width)).text(group)).node);
      }
      const id = `csv-${list}-${i}`;
      const box = El.create("input")
        .cls("form-check-input m-0 align-middle")
        .prop("type", "checkbox")
        .prop("id", id)
        .prop("checked", on(it.value))
        .on("change", (e) => {
          const checked = /** @type {HTMLInputElement} */ (e.target).checked;
          const rest = values.filter(on).filter((v) => v !== it.value);
          commit(checked ? [...rest, it.value] : rest);
        });
      const tr = El.create("tr")
        .attr("data-csv-item", it.value.toLowerCase())
        .attr("data-csv-value", it.value)
        .append(
          El.create("td").append(box),
          El.create("td").append(El.create("label").cls("d-block text-truncate m-0").attr("for", id).text(it.value)),
        );
      if (counts) {
        tr.append(El.create("td").cls("csv-num").text(String(it.rows ?? 0)));
      }
      if (samples) {
        tr.append(El.create("td").cls("csv-sample").attr("title", it.sample ?? "").text(it.sample || "—"));
      }
      rows.push(tr.node);
    });
    /** @type {HTMLElement} */ (dialog.querySelector(`[data-csv-list="${list}"]`)).replaceChildren(...rows);
    this.#filter(dialog, list);
  }

  /** The values of list's rows the search leaves shown. @param {HTMLElement} dialog @param {CsvList} list */
  #visible(dialog, list) {
    return [...dialog.querySelectorAll(`[data-csv-list="${list}"] tr[data-csv-value]`)]
      .filter((tr) => !(/** @type {HTMLElement} */ (tr).hidden))
      .map((tr) => tr.getAttribute("data-csv-value") ?? "");
  }

  /**
   * Shows only list's rows whose name holds the search text, and only the
   * group rows with a row still shown under them.
   * @param {HTMLElement} dialog @param {CsvList} list
   */
  #filter(dialog, list) {
    const want = /** @type {HTMLInputElement} */ (dialog.querySelector(`[data-csv-find="${list}"]`)).value.trim().toLowerCase();
    const body = /** @type {HTMLElement} */ (dialog.querySelector(`[data-csv-list="${list}"]`));
    /** @type {HTMLElement | null} */
    let heading = null;
    let any = false;
    const close = () => {
      if (heading) {
        heading.hidden = !any;
      }
    };
    for (const tr of /** @type {HTMLElement[]} */ ([...body.children])) {
      if (tr.hasAttribute("data-csv-group")) {
        close();
        heading = tr;
        any = false;
        continue;
      }
      tr.hidden = want !== "" && !(tr.getAttribute("data-csv-item") ?? "").includes(want);
      any = any || !tr.hidden;
    }
    close();
    const find = /** @type {HTMLElement} */ (dialog.querySelector(`[data-csv-find="${list}"]`)).closest(".input-icon")?.parentElement;
    if (find instanceof HTMLElement) {
      El.of(find).hide(body.querySelectorAll("tr[data-csv-value]").length <= 8 && want === "");
    }
  }

  /** The tabs shown, in order. @param {HTMLElement} dialog @returns {HTMLElement[]} */
  static #tabs(dialog) {
    return /** @type {HTMLElement[]} */ ([...dialog.querySelectorAll("[data-csv-tab]")]).filter((t) => !t.hidden);
  }

  /** Selects the remembered tab, or the first one shown. @param {HTMLElement} dialog */
  #showTabs(dialog) {
    const tabs = CsvExport.#tabs(dialog);
    const names = tabs.map((t) => /** @type {CsvList} */ (t.getAttribute("data-csv-tab")));
    const want = /** @type {CsvList} */ (names.includes(/** @type {CsvList} */ (this.#choice.tab)) ? this.#choice.tab : names[0]);
    this.#select(dialog, want, false, false);
  }

  /**
   * Shows list's panel and marks its tab selected.
   * @param {HTMLElement} dialog @param {CsvList} list @param {boolean} focus @param {boolean} [remember]
   */
  #select(dialog, list, focus, remember = true) {
    for (const tab of /** @type {HTMLElement[]} */ ([...dialog.querySelectorAll("[data-csv-tab]")])) {
      const on = tab.getAttribute("data-csv-tab") === list;
      tab.setAttribute("aria-selected", String(on));
      tab.classList.toggle("active", on);
      tab.tabIndex = on ? 0 : -1;
      if (on && focus) {
        tab.focus();
      }
    }
    for (const panel of /** @type {HTMLElement[]} */ ([...dialog.querySelectorAll("[data-csv-panel]")])) {
      panel.hidden = panel.getAttribute("data-csv-panel") !== list;
    }
    // Narrowing applies to a list holding several keys per row, so it is
    // offered beside the keys of such a list only.
    const narrow = /** @type {HTMLInputElement} */ (CsvExport.#part(dialog, "narrow"));
    narrow.checked = this.#choice.narrow;
    CsvExport.#part(dialog, "narrow-box").hidden = !(list === "keys" && this.#preview?.key_list);
    if (remember && this.#choice.tab !== list) {
      this.#choice = { ...this.#choice, tab: list };
      Prefs.set(`csv.${this.name}`, this.#choice);
    }
  }

  /** Arrow keys, Home and End move between the tabs. @param {HTMLElement} dialog @param {KeyboardEvent} e */
  #tabKey(dialog, e) {
    const tabs = CsvExport.#tabs(dialog);
    const at = tabs.indexOf(/** @type {HTMLElement} */ (e.currentTarget));
    const to = { ArrowRight: at + 1, ArrowLeft: at - 1, Home: 0, End: tabs.length - 1 }[e.key];
    if (to === undefined || at < 0) {
      return;
    }
    e.preventDefault();
    const tab = tabs[(to + tabs.length) % tabs.length];
    this.#select(dialog, /** @type {CsvList} */ (tab.getAttribute("data-csv-tab")), true);
  }

  /** The counts, and a badge per reason rows are left out that clears it. @param {HTMLElement} dialog */
  #renderSummary(dialog) {
    const p = /** @type {CsvPreview} */ (this.#preview);
    const blocked = this.blocked;
    const kept = p.columns.filter((c) => !this.#choice.drop.includes(c)).length;
    CsvExport.#part(dialog, "summary").textContent = blocked
      ? "Nothing to download yet."
      : `${p.kept} of ${p.total} row${p.total === 1 ? "" : "s"} · ${kept} of ${p.columns.length} columns` +
        (p.kept > p.rows.length ? ` · first ${p.rows.length} shown` : "");
    /** @type {Node[]} */
    const reasons = [];
    /** @param {number} n @param {string} what @param {Partial<CsvChoice>} clear @param {string} label */
    const reason = (n, what, clear, label) => {
      if (n <= 0 || blocked) {
        return;
      }
      reasons.push(
        El.create("span")
          .cls("tag")
          .append(
            document.createTextNode(`${n} ${what}`),
            El.create("button")
              .cls("btn-close")
              .prop("type", "button")
              .attr("aria-label", label)
              .attr("title", label)
              .on("click", () => {
                if (clear.exclude === "") {
                  /** @type {HTMLInputElement} */ (CsvExport.#part(dialog, "exclude")).value = "";
                }
                this.#update(clear);
              }),
          ).node,
      );
    };
    reason(p.total - p.in_profiles, "in other profiles", { profiles: null, keys: this.#keysFor(null) }, "Tick every profile");
    reason(p.in_profiles - p.in_keys, "for other keys", { keys: null }, "Tick every key");
    reason(p.in_keys - p.kept, `containing “${this.#choice.exclude.trim()}”`, { exclude: "" }, "Stop leaving these out");
    CsvExport.#part(dialog, "reasons").replaceChildren(...reasons);
  }

  /** @param {HTMLElement} dialog */
  #renderPreview(dialog) {
    const p = /** @type {CsvPreview} */ (this.#preview);
    const blocked = this.blocked;
    const box = CsvExport.#part(dialog, "preview");
    if (blocked) {
      box.replaceChildren(El.create("div").cls("empty py-5").append(El.create("p").cls("empty-title").text(blocked)).node);
      return;
    }
    const table = El.create("table").cls("table table-sm table-striped table-vcenter card-table mb-0");
    table.append(
      El.create("thead").append(El.create("tr").append(...p.header.map((h) => El.create("th").attr("scope", "col").text(h)))),
      El.create("tbody").append(
        ...(p.rows.length
          ? p.rows.map((r) => El.create("tr").append(...r.map((c) => El.create("td").attr("title", c).text(c))))
          : [El.create("tr").append(El.create("td").attr("colspan", String(Math.max(p.header.length, 1))).cls("text-secondary text-center py-4").text("No rows match."))]),
      ),
    );
    box.replaceChildren(table.node);
  }

  /** The built-in preset this list is, or "" for one with none (the Termius CSV). */
  get preset() {
    const name = this.name.replace(/\.csv$/, "");
    return name in GRAINS ? name : "";
  }

  /**
   * The URL of the file in the format chosen: the list's own CSV link for
   * CSV, else /exports/run with its preset and the same options.
   */
  #fileURL() {
    const csv = this.url(false);
    const format = this.#choice.format ?? "csv";
    if (format === "csv" || !this.preset) {
      return csv;
    }
    const u = new URL(Ui.path("/exports/run"), location.href);
    for (const [k, v] of csv.searchParams) {
      u.searchParams.set(k, v);
    }
    u.searchParams.set("preset", this.preset);
    u.searchParams.set("format", format);
    return u;
  }

  /** The Export page with this choice: its rows, profiles, keys and text. */
  #exportURL() {
    /** @type {Record<string, unknown>} */
    const where = {};
    if (this.#choice.profiles) {
      where.profile = this.#choice.profiles;
    }
    if (this.#choice.keys) {
      where.key = this.#choice.keys;
    }
    if (this.#choice.exclude.trim()) {
      where.exclude = this.#choice.exclude.trim();
    }
    const sel = JSON.stringify({ version: 1, grain: GRAINS[this.preset], where, ...(this.#choice.keys ? { narrow: true } : {}) });
    const bytes = new TextEncoder().encode(sel);
    let bin = "";
    for (const b of bytes) {
      bin += String.fromCharCode(b);
    }
    const s = btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
    return `${Ui.path("/export")}#s=${s}&f=${this.#choice.format ?? "csv"}`;
  }

  /** @param {HTMLElement} dialog */
  #renderDownload(dialog) {
    const download = /** @type {HTMLAnchorElement} */ (CsvExport.#part(dialog, "download"));
    const formats = CsvExport.#part(dialog, "formats");
    const open = /** @type {HTMLAnchorElement} */ (CsvExport.#part(dialog, "open"));
    formats.hidden = open.hidden = !this.preset;
    const select = /** @type {HTMLSelectElement} */ (CsvExport.#part(dialog, "format"));
    select.value = this.preset ? (this.#choice.format ?? "csv") : "csv";
    select.onchange = () => {
      this.#choice = { ...this.#choice, format: select.value };
      Prefs.set(`csv.${this.name}`, this.#choice);
      this.#renderDownload(dialog);
    };
    if (this.preset) {
      open.href = this.#exportURL();
    }
    const why = this.blocked;
    const note = CsvExport.#part(dialog, "blocked");
    note.textContent = why;
    note.hidden = why === "";
    if (why) {
      download.removeAttribute("href");
      download.setAttribute("aria-disabled", "true");
      download.classList.add("disabled");
    } else {
      download.href = this.#fileURL().href;
      download.setAttribute("download", "");
      download.removeAttribute("aria-disabled");
      download.classList.remove("disabled");
    }
  }

  static #dialog() {
    return document.querySelector("[data-csv-dialog]");
  }

  /** @param {Element} dialog @param {string} name @returns {HTMLElement} */
  static #part(dialog, name) {
    return /** @type {HTMLElement} */ (dialog.querySelector(`[data-csv-${name}]`));
  }
}
