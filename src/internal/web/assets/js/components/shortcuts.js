// @ts-check
import { Component } from "../core/component.js";
import { Prefs } from "../core/prefs.js";

/** The pref that turns the single-key shortcuts off (WCAG 2.1.4). */
const PREF = "shortcuts";

/** How long the second key of a sequence such as "g h" is waited for. */
const SEQUENCE_MS = 1500;

/**
 * Shortcuts is the app's keyboard: "/" opens the command palette, "?" opens
 * the shortcuts sheet, and two-key sequences follow the palette's entries:
 * "g h" goes to Hosts, "n h" opens the add-host form. The sequences are not
 * written here: each palette link carries its own in data-palette-keys, so
 * the palette, this sheet and the keyboard read one list the server renders.
 *
 * Single-key shortcuts never fire while a field has the focus, while a
 * modifier is held, or while a dialog or a panel is open, and the sheet's
 * switch turns them off for this browser. Ctrl+K (⌘K) is the palette's own
 * and works regardless.
 *
 * Markup: the sheet (a Tabler modal) carries data-shortcuts, its switch
 * data-shortcuts-toggle; anything with data-shortcuts-open opens it.
 *
 * @extends {Component<{}>}
 */
export class Shortcuts extends Component {
  static attribute = "shortcuts";

  /** @type {Map<string, HTMLAnchorElement>} */
  #sequences = new Map();
  /** The first key of a sequence, while its second is awaited. */
  #pending = "";
  #timer = 0;
  /** @type {any} */
  #modal = null;

  init() {
    const Modal = /** @type {any} */ (window).tabler?.Modal;
    if (!Modal) {
      return this;
    }
    this.#modal = Modal.getOrCreateInstance(this.el);
    for (const a of document.querySelectorAll("a[data-palette-keys]")) {
      const keys = /** @type {HTMLAnchorElement} */ (a).dataset.paletteKeys ?? "";
      this.#sequences.set(keys.trim().replace(/\s+/g, " "), /** @type {HTMLAnchorElement} */ (a));
    }
    const toggle = /** @type {HTMLInputElement | null} */ (this.el.querySelector("[data-shortcuts-toggle]"));
    if (toggle) {
      toggle.checked = this.enabled;
      toggle.addEventListener("change", () => Prefs.set(PREF, toggle.checked));
    }
    for (const opener of document.querySelectorAll("[data-shortcuts-open]")) {
      opener.addEventListener("click", () => this.open());
    }
    document.addEventListener("keydown", (e) => this.#onKey(e));
    return this;
  }

  /** Whether the single-key shortcuts are on in this browser. */
  get enabled() {
    return Prefs.get(PREF, /** @type {boolean} */ (true));
  }

  /** Opens the shortcuts sheet, closing the palette first if it is open. */
  open() {
    const palette = document.querySelector("[data-command-palette].show");
    const Modal = /** @type {any} */ (window).tabler?.Modal;
    if (palette && Modal) {
      palette.addEventListener("hidden.bs.modal", () => this.#modal?.show(), { once: true });
      Modal.getOrCreateInstance(palette).hide();
    } else {
      this.#modal?.show();
    }
    return this;
  }

  /** @param {EventTarget | null} t */
  static #typing(t) {
    if (!(t instanceof HTMLElement)) {
      return false;
    }
    return t.isContentEditable || t.matches("input, textarea, select, [role=combobox], [role=textbox]");
  }

  /** Whether something on top of the page owns the keyboard now. */
  static #covered() {
    return Boolean(document.querySelector(".modal.show, .offcanvas.show, .dropdown-menu.show, .popover.tour-popover"));
  }

  #clear() {
    this.#pending = "";
    window.clearTimeout(this.#timer);
  }

  /** @param {KeyboardEvent} e */
  #onKey(e) {
    if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey || e.isComposing || !this.enabled) {
      return;
    }
    if (Shortcuts.#typing(e.target) || Shortcuts.#covered()) {
      this.#clear();
      return;
    }
    const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
    if (this.#pending) {
      const target = this.#sequences.get(`${this.#pending} ${key}`);
      this.#clear();
      if (target) {
        e.preventDefault();
        target.click();
      }
      return;
    }
    if (e.key === "?") {
      e.preventDefault();
      this.open();
    } else if (e.key === "/") {
      e.preventDefault();
      const palette = document.querySelector("[data-command-palette]");
      const Modal = /** @type {any} */ (window).tabler?.Modal;
      if (palette && Modal) {
        Modal.getOrCreateInstance(palette).show();
      }
    } else if (!e.shiftKey && [...this.#sequences.keys()].some((s) => s.startsWith(`${key} `))) {
      this.#pending = key;
      this.#timer = window.setTimeout(() => this.#clear(), SEQUENCE_MS);
    }
  }
}
