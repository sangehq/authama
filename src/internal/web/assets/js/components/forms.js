// @ts-check
import { Component } from "../core/component.js";

/**
 * ShowWhen shows a field only while another control holds one of the values
 * it applies to, and disables what it holds while hidden, so a hidden field
 * is never submitted. Without the script every field shows and the server
 * still checks the combination.
 *
 * Markup: the field's wrapper carries data-show-when="<selector of the
 * control>" and data-show-when-in="<value> <value> …". An empty value in the
 * list ("-") matches the control having no value.
 *
 *   <div data-show-when="#ts-target" data-show-when-in="npm">…</div>
 *
 * @extends {Component<{in: string}>}
 */
export class ShowWhen extends Component {
  static attribute = "show-when";
  static defaults = { in: "" };

  init() {
    const control = document.querySelector(this.el.dataset.showWhen ?? "");
    if (!(control instanceof HTMLSelectElement || control instanceof HTMLInputElement)) {
      return this;
    }
    const values = new Set(this.options.in.split(/\s+/).filter(Boolean).map((v) => (v === "-" ? "" : v)));
    const sync = () => this.toggle(values.has(control.value));
    control.addEventListener("change", sync);
    sync();
    return this;
  }

  /** Shows or hides the field, enabling its controls only while shown. @param {boolean} on */
  toggle(on) {
    this.el.classList.toggle("d-none", !on);
    for (const c of this.el.querySelectorAll("input, select, textarea")) {
      /** @type {HTMLInputElement} */ (c).disabled = !on;
    }
    return this;
  }
}
