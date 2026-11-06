// @ts-check
import { Component } from "../core/component.js";

/**
 * CommandPalette is the Ctrl+K (⌘K on a Mac) jump list: a Tabler modal whose
 * search box filters a list of links to every page and every action a page
 * offers. Arrow keys move through what matches, Enter follows it, Escape
 * closes. The list is rendered into the page by the server, so the palette
 * shows nothing the user could not reach by the menu.
 *
 * Markup: the modal carries data-command-palette; its input carries
 * data-palette-input and each link data-palette-item, with its words to
 * match in data-palette-keywords. Options:
 *
 *   data-command-palette-hotkey="k"   the key that opens it with Ctrl or ⌘
 *
 * @extends {Component<{hotkey: string}>}
 */
export class CommandPalette extends Component {
  static attribute = "command-palette";
  static defaults = { hotkey: "k" };

  /** @type {HTMLInputElement | null} */
  #input = null;
  /** @type {HTMLAnchorElement[]} */
  #items = [];
  #active = -1;
  /** @type {any} */
  #modal = null;

  init() {
    const Modal = /** @type {any} */ (window).tabler?.Modal;
    if (!Modal) {
      return this;
    }
    this.#modal = Modal.getOrCreateInstance(this.el);
    this.#input = this.el.querySelector("[data-palette-input]");
    this.#items = Array.from(this.el.querySelectorAll("a[data-palette-item]"));
    document.addEventListener("keydown", (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === this.options.hotkey) {
        e.preventDefault();
        this.open();
      }
    });
    // The hint says what this keyboard has: ⌘K on a Mac, Ctrl K elsewhere.
    if (/Mac|iPhone|iPad/.test(navigator.platform)) {
      for (const hint of document.querySelectorAll("[data-palette-hint]")) {
        hint.textContent = `⌘${this.options.hotkey.toUpperCase()}`;
      }
    }
    this.el.addEventListener("shown.bs.modal", () => this.#input?.focus());
    this.#input?.addEventListener("input", () => this.filter(this.#input?.value ?? ""));
    this.#input?.addEventListener("keydown", (e) => this.#onKey(e));
    return this.filter("");
  }

  /** Opens the palette with an empty search. */
  open() {
    if (this.#input) {
      this.#input.value = "";
    }
    this.filter("");
    this.#modal?.show();
    return this;
  }

  /** Shows the items whose label or keywords hold every word of query. @param {string} query */
  filter(query) {
    const words = query.toLowerCase().split(/\s+/).filter(Boolean);
    let first = -1;
    this.#items.forEach((a, i) => {
      const hay = `${a.textContent ?? ""} ${a.dataset.paletteKeywords ?? ""}`.toLowerCase();
      const hit = words.every((w) => hay.includes(w));
      a.classList.toggle("d-none", !hit);
      if (hit && first < 0) {
        first = i;
      }
    });
    return this.select(first);
  }

  /** Marks one item as the one Enter follows. @param {number} i */
  select(i) {
    this.#active = i;
    this.#items.forEach((a, j) => {
      a.classList.toggle("active", j === i);
      a.setAttribute("aria-selected", String(j === i));
    });
    this.#items[i]?.scrollIntoView({ block: "nearest" });
    if (this.#input && this.#items[i]) {
      this.#input.setAttribute("aria-activedescendant", this.#items[i].id);
    }
    return this;
  }

  /** @param {KeyboardEvent} e */
  #onKey(e) {
    const visible = this.#items.map((a, i) => ({ a, i })).filter(({ a }) => !a.classList.contains("d-none"));
    const pos = visible.findIndex(({ i }) => i === this.#active);
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      if (visible.length === 0) {
        return;
      }
      const next = e.key === "ArrowDown" ? (pos + 1) % visible.length : (pos - 1 + visible.length) % visible.length;
      this.select(visible[next].i);
    } else if (e.key === "Enter" && this.#items[this.#active]) {
      e.preventDefault();
      this.#items[this.#active].click();
    }
  }
}

/**
 * PanelFromHash opens the slide-in panel an address names: /hosts#add-host
 * opens the add-host panel, so a link from another page (the dashboard, the
 * palette) lands on the form, not just the page. Markup: the panel carries
 * data-panel-from-hash.
 *
 * @extends {Component<{}>}
 */
export class PanelFromHash extends Component {
  static attribute = "panel-from-hash";

  init() {
    const Offcanvas = /** @type {any} */ (window).tabler?.Offcanvas;
    const open = () => {
      if (Offcanvas && this.el.id && window.location.hash === `#${this.el.id}`) {
        Offcanvas.getOrCreateInstance(this.el).show();
      }
    };
    window.addEventListener("hashchange", open);
    open();
    return this;
  }
}
