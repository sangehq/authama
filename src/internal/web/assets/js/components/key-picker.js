// @ts-check
import { Component } from "../core/component.js";
import { Ui } from "../core/ui.js";

/**
 * KeyPicker makes the key picker's groups behave as groups: ticking a whole
 * profile ticks (and locks) its keys, ticking a category does the same for
 * its profiles, and "Every key" locks everything else. It says how many keys
 * are chosen, and keeps the form's submit button off while none are. Without
 * the script every box still posts and the server resolves the same choice.
 *
 *   <fieldset data-key-picker>…<input data-pick-all>…<input data-pick-profile>…</fieldset>
 *
 * A key the action cannot act on carries data-pick-ineligible and stays off.
 *
 * @extends {Component<{}>}
 */
export class KeyPicker extends Component {
  static attribute = "key-picker";

  init() {
    this.el.addEventListener("change", () => this.sync());
    const filter = /** @type {HTMLInputElement | null} */ (this.el.querySelector("[data-pick-filter]"));
    if (filter) {
      // The filter only helps with a long list, and needs the script.
      const box = this.el.querySelector("[data-pick-filter-box]");
      if (box instanceof HTMLElement && this.#boxes("[data-pick-key]").length > 6) {
        box.hidden = false;
      }
      filter.addEventListener("input", () => this.filter(filter.value));
    }
    this.el.querySelector("[data-pick-clear]")?.addEventListener("click", () => this.clear());
    this.sync();
    return this;
  }

  /**
   * Shows only the keys whose name, profile or hosts contain text. A hidden
   * key keeps its tick: the filter narrows the view, not the choice.
   * @param {string} text
   */
  filter(text) {
    const q = text.trim().toLowerCase();
    let shown = 0;
    for (const group of /** @type {HTMLElement[]} */ ([...this.el.querySelectorAll("[data-pick-group]")])) {
      let any = false;
      for (const row of /** @type {HTMLElement[]} */ ([...group.querySelectorAll("[data-pick-text]")])) {
        const hit = q === "" || (row.dataset.pickText ?? "").toLowerCase().includes(q);
        row.hidden = !hit;
        any ||= hit;
      }
      group.hidden = !any;
      shown += any ? 1 : 0;
    }
    const none = this.el.querySelector("[data-pick-nomatch]");
    if (none instanceof HTMLElement) {
      none.hidden = shown > 0;
    }
    return this;
  }

  /** Unticks everything, "every key" included. */
  clear() {
    for (const b of this.#boxes("input[type=checkbox]")) {
      if (b.hasAttribute("data-pick-ineligible")) {
        continue;
      }
      b.disabled = false;
      b.checked = false;
      b.indeterminate = false;
      delete b.dataset.pickWas;
    }
    this.sync();
    return this;
  }

  /** @param {string} sel @returns {HTMLInputElement[]} */
  #boxes(sel) {
    return /** @type {HTMLInputElement[]} */ ([...this.el.querySelectorAll(sel)]);
  }

  /**
   * Locks a box as covered by a group (ticked, disabled, so it is not posted
   * twice), or gives it back. A box the person ticked themselves keeps its
   * tick when the group lets go.
   * @param {HTMLInputElement} box @param {boolean} covered
   */
  #cover(box, covered) {
    if (box.hasAttribute("data-pick-ineligible")) {
      return;
    }
    if (covered) {
      if (!box.disabled) {
        box.dataset.pickWas = box.checked ? "1" : "";
      }
      box.checked = true;
      box.disabled = true;
    } else if (box.disabled) {
      box.disabled = false;
      box.checked = box.dataset.pickWas === "1";
      delete box.dataset.pickWas;
    }
  }

  sync() {
    const all = this.#boxes("[data-pick-all]")[0]?.checked ?? false;
    this.el.classList.toggle("is-all", all);
    const cats = new Set(this.#boxes("[data-pick-category]").filter((b) => b.checked && !b.disabled).map((b) => b.dataset.pickCategory));
    for (const b of this.#boxes("[data-pick-category]")) {
      this.#cover(b, all);
    }
    for (const group of this.el.querySelectorAll("[data-pick-group]")) {
      const g = /** @type {HTMLElement} */ (group);
      const prof = /** @type {HTMLInputElement | null} */ (g.querySelector("[data-pick-profile]"));
      const byCategory = cats.has(g.dataset.pickCategoryOf ?? "") && (g.dataset.pickCategoryOf ?? "") !== "";
      if (prof) {
        this.#cover(prof, all || byCategory);
      }
      const whole = all || byCategory || (prof?.checked ?? false);
      for (const k of /** @type {HTMLInputElement[]} */ ([...g.querySelectorAll("[data-pick-key]")])) {
        this.#cover(k, whole);
      }
    }
    // Each profile says how many of its keys are chosen, and its box shows a
    // dash while only some are.
    for (const group of /** @type {HTMLElement[]} */ ([...this.el.querySelectorAll("[data-pick-group]")])) {
      const own = /** @type {HTMLInputElement[]} */ ([...group.querySelectorAll("[data-pick-key]:not([data-pick-ineligible])")]);
      const on = own.filter((k) => k.checked).length;
      const prof = /** @type {HTMLInputElement | null} */ (group.querySelector("[data-pick-profile]"));
      if (prof) {
        prof.indeterminate = !prof.checked && on > 0 && on < own.length;
      }
      const label = group.querySelector("[data-pick-group-count]");
      if (label) {
        label.textContent = own.length === 0 ? "none eligible" : `${on} of ${own.length} chosen`;
      }
    }
    const note = this.el.querySelector("[data-pick-all-note]");
    if (note instanceof HTMLElement) {
      note.hidden = !all;
    }
    // Count what will be acted on: every eligible key that is ticked.
    const keys = this.#boxes("[data-pick-key]").filter((k) => !k.hasAttribute("data-pick-ineligible"));
    const chosen = keys.filter((k) => k.checked).length;
    const count = this.el.querySelector("[data-pick-count]");
    if (count) {
      const verb = { rotate: "rotated", revoke: "revoked", delete: "deleted" }[this.el.dataset.pickAction ?? ""] ?? "chosen";
      count.textContent = keys.length === 0
        ? `No key here can be ${verb}; each says why.`
        : chosen === 0
        ? "No key chosen yet."
        : `${chosen} of ${keys.length} key${keys.length === 1 ? "" : "s"} chosen${all ? ", and any added later" : ""}.`;
    }
    count?.classList.toggle("has-choice", chosen > 0);
    const submit = this.el.closest("form")?.querySelector('[type="submit"]');
    if (submit instanceof HTMLButtonElement) {
      submit.disabled = chosen === 0;
      // "Review 3 keys to rotate": the button says what it will act on.
      const label = submit.querySelector("[data-pick-submit-label]");
      const verb = submit.dataset.pickSubmit;
      if (label && verb) {
        label.textContent = chosen === 0
          ? "Review"
          : all
          ? `Review every key to ${verb}`
          : `Review ${chosen} key${chosen === 1 ? "" : "s"} to ${verb}`;
      }
    }
    return this;
  }
}

/**
 * MovedPanels sends an older link to a slide-in that has moved to a page of
 * its own there: /keys#rotate-keys opens the rotation's review, where its keys
 * are now chosen, and /keys?pick=all#rotate-keys opens it with every key
 * chosen. Markup: `<span data-moved-panels="rotate-keys revoke-keys">`.
 *
 * @extends {Component<{}>}
 */
export class MovedPanels extends Component {
  static attribute = "moved-panels";

  init() {
    const id = window.location.hash.slice(1);
    const moved = (this.el.dataset.movedPanels ?? "").split(" ").filter(Boolean);
    if (id && moved.includes(id)) {
      const all = id === "rotate-keys" && new URLSearchParams(window.location.search).get("pick") === "all";
      window.location.replace(Ui.path(`/jobs/review?action=${encodeURIComponent(id)}${all ? "&all=1" : ""}`));
    }
    return this;
  }
}
