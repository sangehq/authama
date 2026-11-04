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

/**
 * PanelOpen opens a slide-in panel from anywhere - a row's menu, a card - with
 * its form already filled in, so "replace this token" needs no retyping.
 * Markup: `<button data-panel-open="<panel id>" data-panel-fill-<field>="value">`;
 * each data-panel-fill-* sets the panel's control named <field> and tells it
 * it changed, so fields that depend on it (ShowWhen) follow. Without the
 * script the control does nothing, and the page's own button still opens the
 * panel empty.
 *
 * @extends {Component<{}>}
 */
export class PanelOpen extends Component {
  static attribute = "panel-open";

  init() {
    this.el.addEventListener("click", (e) => {
      e.preventDefault();
      const panel = document.getElementById(this.el.dataset.panelOpen ?? "");
      const Offcanvas = /** @type {any} */ (window).tabler?.Offcanvas;
      if (!panel || !Offcanvas) {
        return;
      }
      this.fill(panel);
      Offcanvas.getOrCreateInstance(panel).show();
    });
    return this;
  }

  /** Sets each named control in panel from this element's data-panel-fill-*. @param {HTMLElement} panel */
  fill(panel) {
    for (const [key, value] of Object.entries(this.el.dataset)) {
      if (!key.startsWith("panelFill") || value === undefined) {
        continue;
      }
      const name = key.slice("panelFill".length).replace(/^./, (c) => c.toLowerCase());
      const control = panel.querySelector(`[name="${CSS.escape(name)}"]`);
      if (control instanceof HTMLInputElement || control instanceof HTMLSelectElement || control instanceof HTMLTextAreaElement) {
        control.value = value;
        control.dispatchEvent(new Event("change", { bubbles: true }));
      }
    }
    return this;
  }
}
