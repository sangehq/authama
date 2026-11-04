// @ts-check
import { Component } from "../core/component.js";

/**
 * ConfirmAction opens a confirmation modal instead of submitting: a submit
 * button with data-confirm="<modal id>" shows that modal, whose own button
 * submits the form by its id. Without the script the button submits straight
 * to the preview, which asks again of its own.
 *
 * @extends {Component<{}>}
 */
export class ConfirmAction extends Component {
  static attribute = "confirm";

  init() {
    const Modal = /** @type {any} */ (window).tabler?.Modal;
    const modal = document.getElementById(this.el.dataset.confirm ?? "");
    if (!Modal || !modal) {
      return this;
    }
    this.el.addEventListener("click", (e) => {
      e.preventDefault();
      Modal.getOrCreateInstance(modal).show();
    });
    return this;
  }
}

/**
 * AutoSubmit submits a control's form as soon as it changes: a filter that
 * applies itself. Markup: `<select data-autosubmit>`.
 *
 * @extends {Component<{}>}
 */
export class AutoSubmit extends Component {
  static attribute = "autosubmit";

  init() {
    const control = /** @type {HTMLInputElement | HTMLSelectElement} */ (this.el);
    control.addEventListener("change", () => control.form?.requestSubmit());
    return this;
  }
}

/**
 * JobStream appends a running job's output to its log as it comes, from the
 * job's event stream, as text nodes only; when the job changes state the page
 * reloads to show the question, the result and the right buttons.
 *
 * Markup: `<pre data-job-stream data-job-stream-events="/jobs/<id>/events"
 * data-job-stream-state="#job-state">`, where the state element carries the
 * job's data-state.
 *
 * @extends {Component<{events: string, state: string}>}
 */
export class JobStream extends Component {
  static attribute = "job-stream";
  static defaults = { events: "", state: "#job-state" };

  init() {
    const box = document.querySelector(this.options.state);
    if (!(box instanceof HTMLElement) || !this.options.events || !window.EventSource) {
      return this;
    }
    const state = box.dataset.state;
    if (state !== "running" && state !== "waiting") {
      return this;
    }
    // Lines already on the page are skipped when the stream replays them.
    const seen = (this.el.textContent ?? "").split("\n").length - 1;
    let count = 0;
    const events = new EventSource(this.options.events);
    events.addEventListener("line", (e) => {
      count += 1;
      if (count > seen) {
        this.el.appendChild(document.createTextNode(`${/** @type {MessageEvent} */ (e).data}\n`));
      }
    });
    events.addEventListener("state", (e) => {
      if (/** @type {MessageEvent} */ (e).data !== state) {
        events.close();
        window.location.reload();
      }
    });
    return this;
  }
}

/**
 * CopyText puts a public value on the clipboard: a fingerprint, a public key,
 * a path, an ssh command. Markup: `<button data-copy="the value">`; options:
 *
 *   data-copy-done="Copied"   what the control says for a moment after
 *
 * The page announces the result to a screen reader through a polite live
 * region, and a tooltip on the control, if it has one, says it too. It never
 * reads a value from anywhere but its own attribute, so nothing secret can
 * reach it that the page did not already show.
 *
 * @extends {Component<{done: string}>}
 */
export class CopyText extends Component {
  static attribute = "copy";
  static defaults = { done: "Copied" };

  /** @type {HTMLElement | null} */
  static #live = null;

  init() {
    this.el.addEventListener("click", (e) => {
      e.preventDefault();
      const value = this.el.dataset.copy ?? "";
      CopyText.write(value).then(
        () => this.say(this.options.done),
        () => this.say("Could not copy: select the text and copy it"),
      );
    });
    return this;
  }

  /**
   * Writes text to the clipboard: the async clipboard API where the page may
   * use it (127.0.0.1 counts as secure), else a selected textarea.
   * @param {string} text
   * @returns {Promise<void>}
   */
  static write(text) {
    if (navigator.clipboard?.writeText) {
      return navigator.clipboard.writeText(text);
    }
    return new Promise((resolve, reject) => {
      const area = document.createElement("textarea");
      area.value = text;
      area.setAttribute("readonly", "");
      area.className = "visually-hidden";
      document.body.appendChild(area);
      area.select();
      const ok = document.execCommand("copy");
      area.remove();
      ok ? resolve() : reject(new Error("copy refused"));
    });
  }

  /** Tells a screen reader, and the control's tooltip, what happened. @param {string} message */
  say(message) {
    if (!CopyText.#live) {
      const live = document.createElement("div");
      live.className = "visually-hidden";
      live.setAttribute("role", "status");
      live.setAttribute("aria-live", "polite");
      document.body.appendChild(live);
      CopyText.#live = live;
    }
    CopyText.#live.textContent = message;
    const Tooltip = /** @type {any} */ (window).tabler?.Tooltip;
    const tip = Tooltip?.getInstance(this.el);
    if (tip) {
      const was = this.el.getAttribute("data-bs-original-title") ?? this.el.title;
      tip.setContent({ ".tooltip-inner": message });
      tip.show();
      setTimeout(() => tip.setContent({ ".tooltip-inner": was }), 1500);
    } else {
      CopyText.toast(message);
    }
    return this;
  }

  /** A short Tabler toast in the corner, for a control with no tooltip. @param {string} message */
  static toast(message) {
    const Toast = /** @type {any} */ (window).tabler?.Toast;
    if (!Toast) {
      return;
    }
    let stack = document.getElementById("toast-stack");
    if (!stack) {
      stack = document.createElement("div");
      stack.id = "toast-stack";
      stack.className = "toast-container position-fixed bottom-0 end-0 p-3";
      document.body.appendChild(stack);
    }
    const toast = document.createElement("div");
    toast.className = "toast align-items-center";
    toast.setAttribute("role", "status");
    const body = document.createElement("div");
    body.className = "toast-body";
    body.textContent = message;
    toast.appendChild(body);
    stack.appendChild(toast);
    toast.addEventListener("hidden.bs.toast", () => toast.remove());
    new Toast(toast, { delay: 1800 }).show();
  }
}
