// @ts-check
import { Component } from "../core/component.js";

/**
 * Submenu keeps a menu open while one of its submenus (Tabler's multi-level
 * .dropend or .dropstart) is used. Bootstrap closes a dropdown on any click inside it unless
 * its toggle says data-bs-auto-close="outside", which would also keep it open
 * after Take the tour or About; this vetoes the close only for a click inside
 * the submenu itself. Markup: `<div class="dropstart" data-submenu>` inside the
 * parent's .dropdown-menu. Without this script the submenu still opens from
 * the keyboard, and a click on it closes both.
 *
 * @extends {Component<{}>}
 */
export class Submenu extends Component {
  static attribute = "submenu";

  init() {
    const parent = this.el.parentElement?.closest(".dropdown")?.querySelector(":scope > [data-bs-toggle='dropdown']");
    parent?.addEventListener("hide.bs.dropdown", (e) => {
      const click = /** @type {any} */ (e).clickEvent;
      if (click && click.target instanceof Node && this.el.contains(click.target)) {
        e.preventDefault();
      }
    });
    // Closing the parent closes the submenu, so it is not open next time.
    parent?.addEventListener("hidden.bs.dropdown", () => {
      const toggle = this.el.querySelector(":scope > [data-bs-toggle='dropdown']");
      const Dropdown = /** @type {any} */ (window).tabler?.Dropdown ?? /** @type {any} */ (window).bootstrap?.Dropdown;
      Dropdown?.getInstance(toggle)?.hide();
    });
    return this;
  }
}
