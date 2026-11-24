// @ts-check
import { Component } from "../core/component.js";
import { CopyCells } from "./copy-cells.js";

/**
 * StackTable lets a wide table fit a narrow screen without scrolling
 * sideways: it labels each body cell with its column's header (data-label),
 * and app.css lays a labelled table out as one card per row below the
 * extra-large breakpoint (1200px: laptops, tablets and phones), each cell under its header's name. Markup: `<table data-stack>`.
 * Without this script the table keeps its normal layout and scrolls.
 *
 * @extends {Component<{}>}
 */
export class StackTable extends Component {
  static attribute = "stack";

  init() {
    const table = /** @type {HTMLTableElement} */ (this.el);
    const head = table.tHead?.rows[0];
    if (!head) {
      return this;
    }
    // A header read only by a screen reader (Actions) labels nothing on screen.
    const labels = Array.from(head.cells).map((c) => CopyCells.headerLabel(c));
    for (const body of table.tBodies) {
      for (const row of body.rows) {
        let col = 0;
        for (const cell of row.cells) {
          cell.dataset.label = labels[col] ?? "";
          col += cell.colSpan || 1;
        }
      }
    }
    table.dataset.stackReady = "";
    return this;
  }
}
