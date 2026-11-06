// @ts-check
import { Component } from "../core/component.js";

/**
 * CommandPalette is the Ctrl+K (⌘K on a Mac) jump list: a Tabler modal whose
 * search box filters a list of links to every page and every action a page
 * offers, grouped under Pages, Actions and Downloads. Arrow keys move through
 * what matches (Home and End jump to the ends), Enter follows it, Escape
 * closes. The list is rendered into the page by the server, so the palette
 * shows nothing the user could not reach by the menu.
 *
 * Markup: the modal carries data-command-palette; its input carries
 * data-palette-input, each link data-palette-item with its words to match in
 * data-palette-keywords, each heading's run data-palette-section, and the
 * no-match note data-palette-empty (its query in data-palette-query).
 * Options:
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
  /** @type {HTMLElement[]} */
  #sections = [];
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
    this.#sections = Array.from(this.el.querySelectorAll("[data-palette-section]"));
    document.addEventListener("keydown", (e) => {
      if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === this.options.hotkey) {
        e.preventDefault();
        this.toggle();
      }
    });
    // The hint says what this keyboard has: ⌘K on a Mac, Ctrl K elsewhere.
    if (/Mac|iPhone|iPad/.test(navigator.platform)) {
      for (const hint of document.querySelectorAll("[data-palette-hint]")) {
        hint.textContent = `⌘${this.options.hotkey.toUpperCase()}`;
      }
    }
    // Opened by the header's button too, so start every opening empty.
    this.el.addEventListener("show.bs.modal", () => this.#reset());
    this.el.addEventListener("shown.bs.modal", () => this.#input?.focus());
    this.#input?.addEventListener("input", () => this.filter(this.#input?.value ?? ""));
    this.#input?.addEventListener("keydown", (e) => this.#onKey(e));
    // The pointer chooses too, without stealing the focus from the input.
    for (const [i, a] of this.#items.entries()) {
      a.addEventListener("mousemove", () => {
        if (this.#active !== i) {
          this.select(i, false);
        }
      });
    }
    return this.filter("");
  }

  /** Whether the palette is showing. */
  get isOpen() {
    return this.el.classList.contains("show");
  }

  /** Opens the palette with an empty search. */
  open() {
    this.#modal?.show();
    return this;
  }

  /** Closes the palette. */
  close() {
    this.#modal?.hide();
    return this;
  }

  /** Opens the palette, or closes it when it is open. */
  toggle() {
    return this.isOpen ? this.close() : this.open();
  }

  #reset() {
    if (this.#input) {
      this.#input.value = "";
    }
    this.filter("");
    this.el.querySelector(".palette-list")?.scrollTo(0, 0);
  }

  /** Shows the items whose label or keywords hold every word of query. @param {string} query */
  filter(query) {
    const words = query.toLowerCase().split(/\s+/).filter(Boolean);
    let first = -1;
    this.#items.forEach((a, i) => {
      const hay = `${a.textContent ?? ""} ${a.dataset.paletteKeywords ?? ""}`.toLowerCase();
      const hit = words.every((w) => hay.includes(w));
      a.hidden = !hit;
      if (hit && first < 0) {
        first = i;
      }
    });
    for (const sec of this.#sections) {
      sec.hidden = !sec.querySelector("a[data-palette-item]:not([hidden])");
    }
    const empty = /** @type {HTMLElement | null} */ (this.el.querySelector("[data-palette-empty]"));
    if (empty) {
      empty.hidden = first >= 0;
      const q = empty.querySelector("[data-palette-query]");
      if (q) {
        q.textContent = query.trim();
      }
    }
    this.#input?.setAttribute("aria-expanded", String(first >= 0));
    return this.select(first);
  }

  /**
   * Marks one item as the one Enter follows.
   * @param {number} i
   * @param {boolean} [scroll] bring it into view (not when the pointer chose it)
   */
  select(i, scroll = true) {
    this.#active = i;
    this.#items.forEach((a, j) => {
      a.classList.toggle("active", j === i);
      a.setAttribute("aria-selected", String(j === i));
    });
    if (scroll) {
      this.#items[i]?.scrollIntoView({ block: "nearest" });
    }
    if (this.#input) {
      if (this.#items[i]) {
        this.#input.setAttribute("aria-activedescendant", this.#items[i].id);
      } else {
        this.#input.removeAttribute("aria-activedescendant");
      }
    }
    return this;
  }

  /** @param {KeyboardEvent} e */
  #onKey(e) {
    const visible = this.#items.map((a, i) => ({ a, i })).filter(({ a }) => !a.hidden);
    const pos = visible.findIndex(({ i }) => i === this.#active);
    let next = -1;
    switch (e.key) {
      case "ArrowDown":
        next = (pos + 1) % visible.length;
        break;
      case "ArrowUp":
        next = (pos - 1 + visible.length) % visible.length;
        break;
      case "Home":
      case "End":
        if (!(e.ctrlKey || e.metaKey) && this.#input?.value) {
          return; // the caret moves in what was typed
        }
        next = e.key === "Home" ? 0 : visible.length - 1;
        break;
      case "Enter":
        if (this.#items[this.#active]) {
          e.preventDefault();
          this.#items[this.#active].click();
        }
        return;
      default:
        return;
    }
    e.preventDefault();
    if (visible.length > 0) {
      this.select(visible[next].i);
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
