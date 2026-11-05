// @ts-check
import { Component } from "../core/component.js";
import { El } from "../core/el.js";
import { Prefs } from "../core/prefs.js";
import { Ui } from "../core/ui.js";

/**
 * @typedef {{ columns: string[], header: string[], rows: string[][], total: number, kept: number }} CsvPreview
 * @typedef {{ drop: string[], exclude: string }} CsvChoice
 */

/**
 * CsvExport turns a CSV download link into a dialog: choose the columns, leave
 * out rows containing some text, see the first rows as they will be, then
 * download. The server does the shaping (?drop=, ?exclude=, ?preview=1), so
 * the file is exactly what the preview showed; the choice is remembered per
 * list in this browser. Without the script the link downloads the whole file.
 *
 *   <a href="/exports/hosts.csv" download data-csv-export>CSV</a>
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
  #choice = { drop: [], exclude: "" };

  /** @type {string[]} */
  #columns = [];

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

  /** The list's name, from the link: the key its choice is kept under. */
  get name() {
    return new URL(this.#link.href).pathname.split("/").pop() ?? "export";
  }

  /** Opens the dialog for this link's list. */
  open() {
    const dialog = /** @type {HTMLElement} */ (CsvExport.#dialog());
    this.#choice = Prefs.get(`csv.${this.name}`, { drop: [], exclude: "" });
    this.#columns = [];
    CsvExport.#part(dialog, "name").textContent = this.name;
    const exclude = /** @type {HTMLInputElement} */ (CsvExport.#part(dialog, "exclude"));
    exclude.value = this.#choice.exclude;
    // The dialog is shared: rebind its controls to this link each time.
    exclude.oninput = () => this.#update({ exclude: exclude.value }, true);
    CsvExport.#part(dialog, "all").onclick = () => this.#update({ drop: [] });
    CsvExport.#part(dialog, "none").onclick = () => this.#update({ drop: [...this.#columns] });
    CsvExport.#part(dialog, "reset").onclick = () => {
      exclude.value = "";
      this.#update({ drop: [], exclude: "" });
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

  /** The link's URL with the choice applied. @param {boolean} preview */
  url(preview) {
    const u = new URL(this.#link.href);
    u.searchParams.delete("drop");
    u.searchParams.delete("exclude");
    u.searchParams.delete("preview");
    if (this.#choice.drop.length) {
      u.searchParams.set("drop", this.#choice.drop.join(","));
    }
    if (this.#choice.exclude.trim()) {
      u.searchParams.set("exclude", this.#choice.exclude.trim());
    }
    if (preview) {
      u.searchParams.set("preview", "1");
    }
    return u;
  }

  async #refresh() {
    const dialog = /** @type {HTMLElement} */ (CsvExport.#dialog());
    const download = /** @type {HTMLAnchorElement} */ (CsvExport.#part(dialog, "download"));
    download.href = this.url(false).href;
    download.setAttribute("download", "");
    this.#inflight?.abort();
    this.#inflight = new AbortController();
    try {
      const res = await fetch(this.url(true), { credentials: "same-origin", signal: this.#inflight.signal });
      if (!res.ok) {
        throw new Error(`the server answered ${res.status}`);
      }
      /** @type {CsvPreview} */
      const p = await res.json();
      this.#columns = p.columns;
      this.#renderColumns(dialog, p.columns);
      this.#renderPreview(dialog, p);
    } catch (err) {
      if (!(err instanceof DOMException && err.name === "AbortError")) {
        CsvExport.#part(dialog, "summary").textContent = `The preview could not be loaded (${err}). The download still works.`;
      }
    }
    return this;
  }

  /** @param {HTMLElement} dialog @param {string[]} columns */
  #renderColumns(dialog, columns) {
    const box = CsvExport.#part(dialog, "columns");
    box.replaceChildren(
      ...columns.map((col, i) => {
        const id = `csv-col-${i}`;
        const input = El.create("input")
          .cls("form-check-input")
          .prop("type", "checkbox")
          .prop("id", id)
          .prop("checked", !this.#choice.drop.includes(col))
          .on("change", (e) => {
            const on = /** @type {HTMLInputElement} */ (e.target).checked;
            const drop = this.#choice.drop.filter((d) => d !== col);
            this.#update({ drop: on ? drop : [...drop, col] });
          });
        return El.create("div")
          .cls("form-check")
          .append(input, El.create("label").cls("form-check-label").attr("for", id).text(col)).node;
      }),
    );
  }

  /** @param {HTMLElement} dialog @param {CsvPreview} p */
  #renderPreview(dialog, p) {
    const left = p.total - p.kept;
    CsvExport.#part(dialog, "summary").textContent =
      `${p.kept} of ${p.total} row${p.total === 1 ? "" : "s"}` +
      (left ? `, ${left} left out` : "") +
      `, ${p.header.length} of ${p.columns.length} columns.` +
      (p.kept > p.rows.length ? ` The first ${p.rows.length} are shown.` : "");
    const table = El.create("table").cls("table table-sm table-vcenter card-table mb-0");
    table.append(
      El.create("thead").append(El.create("tr").append(...p.header.map((h) => El.create("th").attr("scope", "col").text(h)))),
      El.create("tbody").append(
        ...(p.rows.length
          ? p.rows.map((r) => El.create("tr").append(...r.map((c) => El.create("td").text(c))))
          : [El.create("tr").append(El.create("td").attr("colspan", String(Math.max(p.header.length, 1))).cls("text-secondary").text("No rows."))]),
      ),
    );
    CsvExport.#part(dialog, "preview").replaceChildren(table.node);
  }

  static #dialog() {
    return document.querySelector("[data-csv-dialog]");
  }

  /** @param {Element} dialog @param {string} name @returns {HTMLElement} */
  static #part(dialog, name) {
    return /** @type {HTMLElement} */ (dialog.querySelector(`[data-csv-${name}]`));
  }
}
