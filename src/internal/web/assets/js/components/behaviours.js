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
