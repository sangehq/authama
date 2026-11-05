// @ts-check
/**
 * Ui is the one place the page reaches Tabler's bundled Bootstrap, so a
 * component asks for "the offcanvas for this element" instead of each one
 * looking up window.tabler and guarding against its absence.
 *
 *   Ui.offcanvas(panel)?.show();
 *   Ui.modal(dialog)?.hide();
 *   Ui.tooltips(root);
 */
export class Ui {
  /** @returns {any} */
  static get #lib() {
    return /** @type {any} */ (window).tabler ?? /** @type {any} */ (window).bootstrap ?? null;
  }

  /**
   * The path the UI is served under (web.base_path), "" at the root, as the
   * layout writes it on <html data-base>. Paths the script names itself go
   * through Ui.path; a link's or form's own URL already carries it.
   * @returns {string}
   */
  static base() {
    return document.documentElement.dataset.base ?? "";
  }

  /** A path on this server under the base path: "/jobs" is "/authama/jobs". @param {string} p */
  static path(p) {
    return Ui.base() + p;
  }

  /** The Offcanvas instance for el, or null without Tabler. @param {Element} el */
  static offcanvas(el) {
    return Ui.#lib?.Offcanvas?.getOrCreateInstance(el) ?? null;
  }

  /** The Modal instance for el, or null without Tabler. @param {Element} el */
  static modal(el) {
    return Ui.#lib?.Modal?.getOrCreateInstance(el) ?? null;
  }

  /** Starts the tooltips and popovers under root that content loaded later brings. @param {ParentNode} root */
  static tooltips(root) {
    const lib = Ui.#lib;
    if (!lib) {
      return;
    }
    for (const el of root.querySelectorAll('[data-bs-toggle="tooltip"]')) {
      lib.Tooltip?.getOrCreateInstance(el);
    }
    for (const el of root.querySelectorAll('[data-bs-toggle="popover"]')) {
      lib.Popover?.getOrCreateInstance(el);
    }
  }

  /**
   * Hides every open modal and slide-in panel but the side panel: a form
   * submitted from one (a confirmation, a "Set lifetime" panel) continues in
   * the side panel, which should not open behind it.
   */
  static closeOverlays() {
    for (const m of document.querySelectorAll(".modal.show")) {
      Ui.modal(m)?.hide();
    }
    for (const o of document.querySelectorAll(".offcanvas.show:not([data-side-panel])")) {
      Ui.offcanvas(o)?.hide();
    }
  }
}
