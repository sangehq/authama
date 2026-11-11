// @ts-check
import { Component } from "../core/component.js";

/**
 * SettingsFilter narrows the Settings page to the settings whose name, key,
 * variable or description hold what is typed, hiding a section with none
 * left. Opening the page at #setting-<key> (a job's check links there) opens
 * that setting's edit form and puts the cursor in it.
 *
 *   <input data-settings-filter>  …  <div data-settings-row="<searchable text>">
 *
 * @extends {Component<{}>}
 */
export class SettingsFilter extends Component {
  static attribute = "settings-filter";

  init() {
    const input = /** @type {HTMLInputElement} */ (this.el);
    input.addEventListener("input", () => this.filter(input.value));
    SettingsFilter.openFromHash();
    window.addEventListener("hashchange", () => SettingsFilter.openFromHash());
    return this;
  }

  /** @param {string} text */
  filter(text) {
    const q = text.trim().toLowerCase();
    let shown = 0;
    for (const section of /** @type {HTMLElement[]} */ ([...document.querySelectorAll("[data-settings-section]")])) {
      let any = false;
      for (const row of /** @type {HTMLElement[]} */ ([...section.querySelectorAll("[data-settings-row]")])) {
        const hit = q === "" || (row.dataset.settingsRow ?? "").toLowerCase().includes(q);
        row.hidden = !hit;
        any ||= hit;
      }
      section.hidden = !any;
      shown += any ? 1 : 0;
    }
    const none = document.querySelector("[data-settings-nomatch]");
    if (none instanceof HTMLElement) {
      none.hidden = shown > 0;
    }
    return this;
  }

  /** Opens the edit form of the setting the address names, if any. */
  static openFromHash() {
    const m = /^#setting-(.+)$/.exec(window.location.hash);
    if (!m) {
      return;
    }
    const row = document.getElementById(`setting-${decodeURIComponent(m[1])}`);
    const collapse = row?.querySelector(".collapse");
    const Collapse = /** @type {any} */ (window).tabler?.Collapse;
    if (!(collapse instanceof HTMLElement)) {
      return;
    }
    if (Collapse) {
      Collapse.getOrCreateInstance(collapse, { toggle: false }).show();
    } else {
      collapse.classList.add("show");
    }
    row?.scrollIntoView({ block: "center" });
    /** @type {HTMLElement | null} */ (collapse.querySelector("input:not([type=hidden]), select"))?.focus({ preventScroll: true });
  }
}
