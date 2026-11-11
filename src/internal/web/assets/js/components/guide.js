// @ts-check
import { Component } from "../core/component.js";

/**
 * Tour walks a first-time user through the page: one Tabler popover at a
 * time, pointing at each element marked
 *
 *   data-tour="2" data-tour-title="Search" data-tour-text="Ctrl+K jumps anywhere."
 *
 * in the order of their numbers, with Back, Next and Done. Escape ends it.
 * The launcher carries data-tour-start; options on it:
 *
 *   data-tour-start-auto="true"   start by itself the first time this browser
 *                                 sees the page (remembered in localStorage)
 *   data-tour-start-key="tour"    the name it is remembered under
 *
 * Everything is built with DOM nodes, never HTML strings, and a step whose
 * element is hidden (a menu folded on a phone) is skipped.
 *
 * @extends {Component<{auto: boolean, key: string}>}
 */
export class Tour extends Component {
  static attribute = "tour-start";
  static defaults = { auto: false, key: "authama.tour" };

  /** @type {HTMLElement[]} */
  #steps = [];
  #at = -1;
  /** The pending automatic start, if any. */
  #auto = 0;
  /** @type {any} */
  #popover = null;
  /** @type {HTMLElement | null} */
  #anchor = null;
  /** @type {(e: KeyboardEvent) => void} */
  #onKey = (e) => {
    if (e.key === "Escape") {
      this.end();
    }
  };

  init() {
    this.el.addEventListener("click", (e) => {
      e.preventDefault();
      this.start();
    });
    if (this.options.auto && !Tour.seen(this.options.key)) {
      // After the page has laid out, so the first target has a place. A tour
      // the user started meanwhile is left alone: restarting it would throw
      // them back to the first step.
      this.#auto = window.setTimeout(() => {
        this.#auto = 0;
        if (this.#at < 0) {
          this.start();
        }
      }, 400);
    }
    return this;
  }

  /** Starts from the first visible step. */
  start() {
    window.clearTimeout(this.#auto);
    this.#auto = 0;
    this.#steps = Array.from(document.querySelectorAll("[data-tour]"))
      .filter((el) => el instanceof HTMLElement && el.getClientRects().length > 0)
      .map((el) => /** @type {HTMLElement} */ (el))
      .sort((a, b) => Number(a.dataset.tour) - Number(b.dataset.tour));
    if (this.#steps.length === 0) {
      return this;
    }
    document.addEventListener("keydown", this.#onKey);
    return this.go(0);
  }

  /** Shows step i, closing the one before. @param {number} i */
  go(i) {
    const Popover = /** @type {any} */ (window).tabler?.Popover;
    this.#popover?.dispose();
    this.#popover = null;
    if (!Popover || i < 0 || i >= this.#steps.length) {
      return this.end();
    }
    this.#at = i;
    const target = this.#steps[i];
    target.scrollIntoView({ block: "center" });
    // Bootstrap allows one component per element, and a step is often a
    // dropdown's toggle: the popover goes on an anchor laid over it instead.
    const anchor = this.anchor(target);
    this.#popover = new Popover(anchor, {
      title: `${target.dataset.tourTitle ?? ""} (${i + 1} of ${this.#steps.length})`,
      content: this.body(target.dataset.tourText ?? ""),
      html: true,
      trigger: "manual",
      placement: "auto",
      customClass: "tour-popover",
    });
    this.#popover.show();
    return this;
  }

  /** The step's text and its buttons, as nodes. @param {string} text */
  body(text) {
    const wrap = document.createElement("div");
    const p = document.createElement("p");
    p.className = "mb-2";
    p.textContent = text;
    const bar = document.createElement("div");
    bar.className = "d-flex gap-2 justify-content-end";
    /** @param {string} label @param {string} cls @param {() => void} fn */
    const button = (label, cls, fn) => {
      const b = document.createElement("button");
      b.type = "button";
      b.className = `btn btn-sm ${cls}`;
      b.textContent = label;
      b.addEventListener("click", fn);
      return b;
    };
    bar.append(button("Skip", "btn-link link-secondary me-auto", () => this.end()));
    if (this.#at > 0) {
      bar.append(button("Back", "btn-outline-secondary", () => this.go(this.#at - 1)));
    }
    const last = this.#at === this.#steps.length - 1;
    bar.append(button(last ? "Done" : "Next", "btn-primary", () => (last ? this.end() : this.go(this.#at + 1))));
    wrap.append(p, bar);
    return wrap;
  }

  /** An empty element laid exactly over target, for the popover to point from. @param {HTMLElement} target */
  anchor(target) {
    this.#anchor?.remove();
    const r = target.getBoundingClientRect();
    const a = document.createElement("span");
    a.className = "tour-anchor";
    a.setAttribute("aria-hidden", "true");
    a.style.position = "absolute";
    a.style.left = `${r.left + window.scrollX}px`;
    a.style.top = `${r.top + window.scrollY}px`;
    a.style.width = `${r.width}px`;
    a.style.height = `${r.height}px`;
    a.style.pointerEvents = "none";
    document.body.appendChild(a);
    this.#anchor = a;
    return a;
  }

  /** Ends the tour and remembers it was seen. */
  end() {
    this.#popover?.dispose();
    this.#popover = null;
    this.#anchor?.remove();
    this.#anchor = null;
    this.#at = -1;
    document.removeEventListener("keydown", this.#onKey);
    Tour.remember(this.options.key);
    return this;
  }

  /** @param {string} key */
  static seen(key) {
    try {
      return window.localStorage.getItem(key) === "seen";
    } catch {
      return true; // no storage: never start by itself
    }
  }

  /** @param {string} key */
  static remember(key) {
    try {
      window.localStorage.setItem(key, "seen");
    } catch {
      // private window or blocked storage: the tour may offer itself again
    }
  }
}

/**
 * SetupCard is the Overview's "Get started" card. Its Hide button hides it
 * in this browser; the card comes back by itself when the steps it lists
 * change, since the key it is remembered under names how many are done.
 *
 *   <div data-setup-card data-setup-card-done="2">… <button data-setup-hide> …</div>
 *
 * @extends {Component<{done: string}>}
 */
export class SetupCard extends Component {
  static attribute = "setup-card";
  static defaults = { done: "" };

  get #key() {
    return `authama.setup.hidden.${this.options.done}`;
  }

  init() {
    try {
      if (window.localStorage.getItem(this.#key) === "1") {
        this.el.hidden = true;
        return this;
      }
    } catch {
      // Without storage the card always shows.
    }
    this.el.querySelector("[data-setup-hide]")?.addEventListener("click", () => {
      this.el.hidden = true;
      try {
        window.localStorage.setItem(this.#key, "1");
      } catch {
        // Hidden for this page only.
      }
    });
    return this;
  }
}
