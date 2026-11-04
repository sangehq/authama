// @ts-check
import { Component } from "../core/component.js";
import { El } from "../core/el.js";

/**
 * DataTable gives a table Tabler's list features: sortable column headers, and
 * for a list longer than `minRows`, a search box, a page size and pages. Every
 * row is in the page already, so without this script the table is the whole
 * list, unpaginated.
 *
 * Markup: `<table data-table="hosts">`, the value naming the rows in the
 * footer. Options, as data attributes:
 *
 *   data-table-page-size="25"   rows per page (0: all); default 10
 *   data-table-min-rows="10"    a list this long or shorter gets no search or pages
 *   data-table-searchable       "false" leaves out the search box
 *
 * A header cell with data-nosort is not sortable; a cell's data-sort value is
 * sorted on instead of its text (a timestamp behind a formatted date).
 *
 * The API chains:
 *
 *   table.sortBy(2, "desc").search("github").pageSize(25).page(1).render();
 *
 * @extends {Component<{pageSize: number, minRows: number, searchable: boolean}>}
 */
export class DataTable extends Component {
  static attribute = "table";
  static defaults = { pageSize: 10, minRows: 10, searchable: true };

  /** @type {HTMLTableRowElement[]} */
  #rows = [];
  #query = "";
  #page = 1;
  #pageSize = 10;
  #sortCol = -1;
  #sortDir = 1;
  /** @type {HTMLTableSectionElement | null} */
  #body = null;
  /** @type {El<HTMLDivElement> | null} */
  #bar = null;
  /** @type {El<HTMLDivElement> | null} */
  #foot = null;
  /** @type {El<HTMLParagraphElement> | null} */
  #info = null;
  /** @type {El<HTMLUListElement> | null} */
  #pager = null;
  /** @type {HTMLTableCellElement[]} */
  #heads = [];

  static #count = 0;

  get #table() {
    return /** @type {HTMLTableElement} */ (this.el);
  }

  /** The label the footer counts in: the attribute's value, else "entries". */
  get label() {
    return this.el.dataset.table || "entries";
  }

  init() {
    this.#body = this.#table.tBodies[0] ?? null;
    if (!this.#body) {
      return this;
    }
    this.#rows = Array.from(this.#body.rows);
    this.#pageSize = this.options.pageSize;
    this.#buildToolbar().#buildFooter().#buildSortHeaders();
    if (this.#rows.length <= this.options.minRows) {
      this.#bar?.hide();
    }
    return this.render();
  }

  /** Sorts by a column, ascending unless told otherwise. @param {number} col @param {"asc" | "desc"} [dir] */
  sortBy(col, dir = "asc") {
    this.#sortCol = col;
    this.#sortDir = dir === "desc" ? -1 : 1;
    this.#rows.sort((a, b) => this.#sortDir * DataTable.compare(DataTable.cellValue(a, col), DataTable.cellValue(b, col)));
    for (const r of this.#rows) {
      this.#body?.appendChild(r);
    }
    this.#heads.forEach((th, i) => {
      const button = th.querySelector(".table-sort");
      th.toggleAttribute("aria-sort", i === col);
      if (i === col) {
        th.setAttribute("aria-sort", this.#sortDir > 0 ? "ascending" : "descending");
      }
      button?.classList.toggle("asc", i === col && this.#sortDir > 0);
      button?.classList.toggle("desc", i === col && this.#sortDir < 0);
    });
    this.#page = 1;
    return this;
  }

  /** Keeps the rows whose text holds query, case-insensitively. @param {string} query */
  search(query) {
    this.#query = query.trim().toLowerCase();
    this.#page = 1;
    return this;
  }

  /** Rows per page; 0 shows every row. @param {number} size */
  pageSize(size) {
    this.#pageSize = Math.max(0, size);
    this.#page = 1;
    return this;
  }

  /** Goes to a page (1-based), clamped to the pages there are. @param {number} n */
  page(n) {
    this.#page = Math.max(1, n);
    return this;
  }

  /** The rows the search leaves. */
  get matching() {
    return this.#query ? this.#rows.filter((r) => (r.textContent ?? "").toLowerCase().includes(this.#query)) : this.#rows;
  }

  /** Shows the current page of the matching rows, and the footer for it. */
  render() {
    const shown = this.matching;
    const per = this.#pageSize > 0 ? this.#pageSize : Math.max(shown.length, 1);
    const pages = Math.max(1, Math.ceil(shown.length / per));
    this.#page = Math.min(this.#page, pages);
    const from = (this.#page - 1) * per;
    const to = Math.min(shown.length, from + per);
    const visible = new Set(shown.slice(from, to));
    for (const r of this.#rows) {
      r.hidden = !visible.has(r);
    }
    this.#info?.text(
      shown.length === 0
        ? `No matching ${this.label}`
        : `Showing ${from + 1} to ${to} of ${shown.length} ${this.label}` +
            (this.#query ? ` (filtered from ${this.#rows.length})` : ""),
    );
    this.#foot?.hide(this.#rows.length <= this.options.minRows);
    this.#renderPager(pages);
    return this;
  }

  #buildToolbar() {
    const n = ++DataTable.#count;
    const sizeId = `dt-size-${n}`;
    const searchId = `dt-search-${n}`;
    const size = El.create("select").cls("form-select form-select-sm w-auto").prop("id", sizeId);
    for (const v of [10, 25, 50, 100, 0]) {
      const o = El.create("option").text(v === 0 ? "all" : String(v)).prop("value", String(v));
      if (v === this.#pageSize) {
        o.prop("selected", true);
      }
      size.append(o);
    }
    size.on("change", () => this.pageSize(Number.parseInt(size.node.value, 10)).render());
    const left = El.create("div")
      .cls("text-secondary d-flex align-items-center gap-2")
      .append(El.create("label").cls("mb-0").text("Show").prop("htmlFor", sizeId), size, El.create("span").text(this.label));
    const row = El.create("div").cls("d-flex flex-wrap gap-2 align-items-center").append(left);
    if (this.options.searchable) {
      const input = El.create("input").cls("form-control form-control-sm").prop("type", "search").prop("id", searchId);
      if (this.el.id) {
        input.attr("aria-controls", this.el.id);
      }
      input.on("input", () => this.search(input.node.value).render());
      row.append(
        El.create("div")
          .cls("ms-auto text-secondary d-flex align-items-center gap-2")
          .append(El.create("label").cls("mb-0").text("Search:").prop("htmlFor", searchId), input),
      );
    }
    this.#bar = El.create("div").cls("card-body border-bottom py-3 d-print-none").append(row).before(this.#wrap);
    return this;
  }

  #buildFooter() {
    this.#info = El.create("p").cls("m-0 text-secondary").attr("aria-live", "polite");
    this.#pager = El.create("ul").cls("pagination m-0");
    const nav = El.create("nav").cls("ms-auto").attr("aria-label", `${this.label} pages`).append(this.#pager);
    this.#foot = El.create("div").cls("card-footer d-flex flex-wrap align-items-center gap-2 d-print-none").append(this.#info, nav).after(this.#wrap);
    return this;
  }

  #buildSortHeaders() {
    const head = this.#table.tHead?.rows[0];
    this.#heads = head ? Array.from(head.cells) : [];
    this.#heads.forEach((th, i) => {
      const label = (th.textContent ?? "").trim();
      if (th.hasAttribute("data-nosort") || !label) {
        return;
      }
      const button = El.create("button").cls("table-sort").prop("type", "button").text(label);
      button.on("click", () => this.sortBy(i, this.#sortCol === i && this.#sortDir > 0 ? "desc" : "asc").render());
      El.of(th).empty().append(button);
    });
    return this;
  }

  /** @param {number} pages */
  #renderPager(pages) {
    if (!this.#pager) {
      return;
    }
    this.#pager.empty();
    if (pages <= 1) {
      return;
    }
    const item = (/** @type {string} */ text, /** @type {number} */ target, { active = false, disabled = false, label = "" } = {}) => {
      const button = El.create("button").cls("page-link").prop("type", "button").text(text);
      if (label) {
        button.attr("aria-label", label);
      }
      if (active) {
        button.attr("aria-current", "page");
      }
      if (disabled) {
        button.prop("disabled", true);
      } else {
        button.on("click", () => this.page(target).render());
      }
      this.#pager?.append(El.create("li").cls("page-item").toggle("active", active).toggle("disabled", disabled).append(button));
    };
    item("‹", this.#page - 1, { disabled: this.#page === 1, label: "Previous page" });
    for (let i = 1; i <= pages; i++) {
      if (pages > 7 && i > 2 && i < pages - 1 && Math.abs(i - this.#page) > 1) {
        if (i === 3 || i === pages - 2) {
          item("…", i, { disabled: true });
        }
        continue;
      }
      item(String(i), i, { active: i === this.#page, label: `Page ${i}` });
    }
    item("›", this.#page + 1, { disabled: this.#page === pages, label: "Next page" });
  }

  /** The element the toolbar and footer sit around: the scroller, else the table. */
  get #wrap() {
    return this.el.closest(".table-responsive") ?? this.el;
  }

  /** A cell's sort value: its data-sort, else its text. @param {HTMLTableRowElement} row @param {number} i */
  static cellValue(row, i) {
    const c = row.cells[i];
    return (c?.dataset.sort ?? c?.textContent ?? "").trim();
  }

  /** Numbers as numbers, everything else naturally. @param {string} a @param {string} b */
  static compare(a, b) {
    const numeric = /^-?\d+(\.\d+)?$/;
    if (numeric.test(a) && numeric.test(b)) {
      return Number(a) - Number(b);
    }
    return a.localeCompare(b, undefined, { numeric: true, sensitivity: "base" });
  }
}
