// @ts-check
import { Component } from "../core/component.js";

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
    const submit = this.el.closest("form")?.querySelector('[type="submit"]');
    if (submit instanceof HTMLButtonElement) {
      submit.disabled = chosen === 0;
    }
    return this;
  }
}
