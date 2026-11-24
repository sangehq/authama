// @ts-check
import { Component } from "../core/component.js";
import { CopyText } from "./behaviours.js";

/**
 * CopyCells lets a reader copy what a table shows: right-click a cell (or
 * focus it and press the context-menu key or Shift+F10) for a menu that copies
 * the cell, its row, its column, or the whole table. Rows and the table are
 * tab-separated, so they paste into a spreadsheet as cells. Markup:
 *
 *   <table data-copy-cells>
 *     <td data-copy-value="">never written</td>   copies its data-copy-value, not its text
 *     <th data-nocopy>Actions</th>                a column (its header) left out of every copy
 *
 * Only rows a search or page has not hidden are copied. The menu is built
 * from DOM nodes, never from markup strings, and copies only what the page
 * already shows.
 *
 * @extends {Component<{}>}
 */
export class CopyCells extends Component {
  static attribute = "copy-cells";

  /** @type {HTMLElement | null} the open menu, one for the whole page */
  static #menu = null;

  init() {
    const table = /** @type {HTMLTableElement} */ (this.el);
    for (const cell of table.querySelectorAll("tbody td, tbody th")) {
      if (cell instanceof HTMLElement && !cell.hasAttribute("tabindex")) {
        cell.tabIndex = -1; // focusable by a click, so the keyboard can open the menu on it
      }
    }
    table.addEventListener("contextmenu", (e) => {
      const cell = CopyCells.cellOf(e.target);
      if (!cell || !table.tBodies[0]?.contains(cell) || window.getSelection()?.toString()) {
        return; // a selection keeps the browser's own menu, to copy exactly that
      }
      e.preventDefault();
      this.open(cell, e.clientX, e.clientY);
    });
    table.addEventListener("keydown", (e) => {
      if (e.key !== "ContextMenu" && !(e.key === "F10" && e.shiftKey)) {
        return;
      }
      const cell = CopyCells.cellOf(e.target);
      if (cell && table.tBodies[0]?.contains(cell)) {
        e.preventDefault();
        const r = cell.getBoundingClientRect();
        this.open(cell, r.left + 8, r.bottom);
      }
    });
    return this;
  }

  /** The table cell an event target is in. @param {EventTarget | null} target */
  static cellOf(target) {
    const el = target instanceof Element ? target.closest("td, th") : null;
    return el instanceof HTMLTableCellElement ? el : null;
  }

  /** A cell's value: its data-copy-value, else its text with whitespace collapsed. @param {Element} cell */
  static value(cell) {
    if (cell instanceof HTMLElement && cell.dataset.copyValue !== undefined) {
      return cell.dataset.copyValue;
    }
    return (cell.textContent ?? "").replace(/\s+/g, " ").trim();
  }

  /**
   * A header cell's name as the page shows it: "" for one only a screen
   * reader reads (an Actions column), whose cells hold controls, not values.
   * @param {HTMLTableCellElement} th
   */
  static headerLabel(th) {
    const hidden = th.querySelector(".visually-hidden");
    if (hidden && hidden.textContent === th.textContent) {
      return "";
    }
    return CopyCells.value(th);
  }

  /** @param {HTMLTableCellElement | undefined} th */
  static copyable(th) {
    return !th || (!th.hasAttribute("data-nocopy") && CopyCells.headerLabel(th) !== "");
  }

  /** The columns copied: the header cells' indexes, less Actions and data-nocopy. */
  columns() {
    const head = /** @type {HTMLTableElement} */ (this.el).tHead?.rows[0];
    const n = head ? head.cells.length : 0;
    const out = [];
    for (let i = 0; i < n; i++) {
      if (CopyCells.copyable(head?.cells[i])) {
        out.push(i);
      }
    }
    return out;
  }

  /** The body rows a search or page has not hidden. */
  rows() {
    const body = /** @type {HTMLTableElement} */ (this.el).tBodies[0];
    return body ? Array.from(body.rows).filter((r) => !r.hidden && r.style.display !== "none") : [];
  }

  /** A row's copied cells, tab-separated. @param {HTMLTableRowElement} row */
  rowText(row) {
    return this.columns()
      .map((i) => (row.cells[i] ? CopyCells.value(row.cells[i]) : ""))
      .join("\t");
  }

  /** The header and every visible row, tab-separated. */
  tableText() {
    const head = /** @type {HTMLTableElement} */ (this.el).tHead?.rows[0];
    const lines = head ? [this.rowText(head)] : [];
    return lines.concat(this.rows().map((r) => this.rowText(r))).join("\n");
  }

  /**
   * Opens the menu for cell at (x, y).
   * @param {HTMLTableCellElement} cell
   * @param {number} x
   * @param {number} y
   */
  open(cell, x, y) {
    CopyCells.close();
    const row = /** @type {HTMLTableRowElement} */ (cell.parentElement);
    const col = cell.cellIndex;
    const head = /** @type {HTMLTableElement} */ (this.el).tHead?.rows[0]?.cells[col];
    const name = head ? CopyCells.headerLabel(head) : "";
    const copyable = CopyCells.copyable(head);
    /** @type {[string, () => string][]} */
    const items = [];
    if (copyable) {
      items.push(["Copy this cell", () => CopyCells.value(cell)]);
    }
    items.push(["Copy this row", () => this.rowText(row)]);
    if (copyable) {
      items.push([name ? `Copy the ${name} column` : "Copy this column", () => this.rows().map((r) => (r.cells[col] ? CopyCells.value(r.cells[col]) : "")).join("\n")]);
    }
    items.push(["Copy the table", () => this.tableText()]);

    const menu = document.createElement("div");
    menu.className = "dropdown-menu show copy-cells-menu";
    menu.setAttribute("role", "menu");
    menu.setAttribute("aria-label", "Copy from the table");
    for (const [label, text] of items) {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "dropdown-item";
      item.setAttribute("role", "menuitem");
      item.textContent = label;
      item.addEventListener("click", () => {
        CopyCells.close();
        CopyText.write(text()).then(
          () => CopyText.toast("Copied"),
          () => CopyText.toast("Could not copy: select the text and copy it"),
        );
        cell.focus();
      });
      menu.appendChild(item);
    }
    menu.addEventListener("keydown", (e) => {
      const buttons = Array.from(menu.querySelectorAll("button"));
      const at = buttons.indexOf(/** @type {HTMLButtonElement} */ (document.activeElement));
      if (e.key === "Escape") {
        e.preventDefault();
        CopyCells.close();
        cell.focus();
      } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        const next = (at + (e.key === "ArrowDown" ? 1 : buttons.length - 1)) % buttons.length;
        buttons[next].focus();
      } else if (e.key === "Tab") {
        CopyCells.close();
      }
    });
    document.body.appendChild(menu);
    // Keep it on screen: flip left or up when it would run off an edge.
    const w = menu.offsetWidth;
    const h = menu.offsetHeight;
    menu.style.position = "fixed";
    menu.style.left = `${Math.max(4, Math.min(x, window.innerWidth - w - 4))}px`;
    menu.style.top = `${Math.max(4, y + h > window.innerHeight ? y - h : y)}px`;
    CopyCells.#menu = menu;
    /** @type {HTMLButtonElement | null} */ (menu.querySelector("button"))?.focus();
    const away = (/** @type {Event} */ e) => {
      if (!(e.target instanceof Node) || !menu.contains(e.target)) {
        CopyCells.close();
      }
    };
    setTimeout(() => {
      document.addEventListener("pointerdown", away, { once: true, capture: true });
      window.addEventListener("scroll", () => CopyCells.close(), { once: true, capture: true });
      window.addEventListener("resize", () => CopyCells.close(), { once: true });
    });
  }

  /** Closes the open menu, if any. */
  static close() {
    CopyCells.#menu?.remove();
    CopyCells.#menu = null;
  }
}
