// @ts-check
import { Component } from "../core/component.js";
import { Ui } from "../core/ui.js";
import { SidePanel } from "./side-panel.js";
import { consoleLineElement } from "./console.js";

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
      // Ui.startModals shows it in front of any slide-in panel.
      Modal.getOrCreateInstance(modal).show();
    });
    // Confirming closes the dialog as the form is sent.
    modal.querySelector('[type="submit"]')?.addEventListener("click", () => Modal.getInstance(modal)?.hide());
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
    const seen = this.el.querySelectorAll(".console-line").length;
    let count = 0;
    const events = new EventSource(this.options.events);
    events.addEventListener("line", (e) => {
      count += 1;
      if (count > seen) {
        this.el.querySelector("[data-console-empty]")?.remove();
        // Follow the newest line, unless the reader has scrolled up to read.
        const atEnd = this.el.scrollHeight - this.el.scrollTop - this.el.clientHeight < 40;
        this.el.appendChild(consoleLineElement(/** @type {MessageEvent} */ (e).data));
        if (atEnd) {
          this.el.scrollTop = this.el.scrollHeight;
        }
      }
    });
    const progress = new JobProgress(document.querySelector("[data-job-progress]"));
    events.addEventListener("progress", (e) => progress.update(/** @type {MessageEvent} */ (e).data));
    events.addEventListener("items", (e) => progress.items(/** @type {MessageEvent} */ (e).data));
    events.addEventListener("state", (e) => {
      if (/** @type {MessageEvent} */ (e).data !== state) {
        events.close();
        progress.stop();
        // In the side panel, the panel's page is the one to refresh, not the
        // page under it.
        if (this.el.closest("[data-side-panel]") && SidePanel.current) {
          SidePanel.current.reload();
        } else {
          Ui.reload();
        }
      }
    });
    return this;
  }
}

/** @param {number} ms @returns {string} "45s", "3m 05s", "1h 20m" */
export function humanDuration(ms) {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) {
    return `${s}s`;
  }
  if (s < 3600) {
    return `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, "0")}s`;
  }
  return `${Math.floor(s / 3600)}h ${String(Math.floor(s / 60) % 60).padStart(2, "0")}m`;
}

/**
 * JobProgress keeps a job page's progress card current: the bar, "3 of 10 ·
 * 30% · about 1m left" and the elapsed time, ticking once a second. The time
 * left is the pace of the finished steps applied to the rest, so it says
 * nothing until one step has finished.
 */
class JobProgress {
  /** @param {Element | null} card */
  constructor(card) {
    this.card = card instanceof HTMLElement ? card : null;
    this.started = Number(this.card?.dataset.jobStarted ?? 0);
    this.since = Number(this.card?.dataset.jobProgressSince ?? 0);
    this.done = 0;
    this.total = 0;
    const bar = this.card?.querySelector("[data-job-progress-bar]");
    if (bar instanceof HTMLProgressElement && bar.hasAttribute("value")) {
      this.done = bar.value;
      this.total = bar.max;
    }
    this.at = Date.now();
    this.timer = this.card ? window.setInterval(() => this.tick(), 1000) : 0;
    this.tick();
  }

  /** @param {string} data "done total label" */
  update(data) {
    if (!this.card) {
      return;
    }
    const [d, t, ...rest] = data.split(" ");
    const done = Number(d);
    const total = Number(t);
    if (!this.since) {
      this.since = Date.now();
    }
    this.done = done;
    this.total = total;
    this.at = Date.now();
    const bar = this.card.querySelector("[data-job-progress-bar]");
    if (bar instanceof HTMLProgressElement) {
      bar.max = Math.max(total, 1);
      bar.value = done;
      bar.setAttribute("aria-label", "Job progress");
      bar.textContent = `${this.percent()}%`;
    }
    this.card.querySelector("[data-job-progress-uncounted]")?.remove();
    const label = this.card.querySelector("[data-job-progress-label]");
    if (label) {
      label.textContent = rest.join(" ") || "Finishing";
    }
    this.tick();
  }

  /**
   * Marks each listed item with its state as the job moves on: "name=state"
   * pairs, space-separated (names hold no spaces or "=").
   * @param {string} data
   */
  items(data) {
    for (const pair of data.split(" ")) {
      const at = pair.lastIndexOf("=");
      const name = pair.slice(0, at);
      const state = pair.slice(at + 1);
      const row = [...(this.card?.querySelectorAll("[data-job-item]") ?? [])].find((r) => r instanceof HTMLElement && r.dataset.jobItem === name);
      if (!(row instanceof HTMLElement) || row.classList.contains(`is-${state}`)) {
        continue;
      }
      row.className = `job-run-item is-${state}`;
      const label = row.querySelector("[data-job-item-state]");
      if (label) {
        label.textContent = state;
      }
      const mark = row.querySelector(".job-run-mark");
      const icon = { running: null, done: "circle-check", failed: "circle-x", waiting: "clock" }[state];
      if (mark) {
        if (state === "running") {
          const spin = document.createElement("span");
          spin.className = "spinner-border spinner-border-sm";
          mark.replaceChildren(spin);
        } else if (icon) {
          mark.replaceChildren(Ui.icon(icon));
        }
      }
      if (state === "running") {
        row.scrollIntoView({ block: "nearest" });
      }
    }
  }

  percent() {
    return this.total > 0 ? Math.min(100, Math.floor((this.done * 100) / this.total)) : 0;
  }

  tick() {
    if (!this.card) {
      return;
    }
    const now = Date.now();
    const elapsed = this.card.querySelector("[data-job-progress-elapsed]");
    if (elapsed && this.started) {
      elapsed.textContent = humanDuration(now - this.started);
    }
    const meta = this.card.querySelector("[data-job-progress-meta]");
    if (meta && this.total > 0) {
      let text = `${this.done} of ${this.total} · ${this.percent()}%`;
      if (this.done > 0 && this.done < this.total && this.since) {
        const per = (this.at - this.since) / this.done;
        const left = per * (this.total - this.done) - (now - this.at);
        text += left > 1000 ? ` · about ${humanDuration(left)} left` : " · almost done";
      }
      meta.textContent = text;
    }
  }

  stop() {
    window.clearInterval(this.timer);
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
    // An icon-only copy button says it in its tooltip; a menu entry (the menu
    // closes as it is chosen) or a labelled button says it in a toast.
    if (!Ui.flashTip(this.el, message)) {
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

/**
 * ThemeChoice marks the colour theme in effect among the links that choose
 * one, so the Settings page shows which is selected. Tabler's own theme script
 * keeps the choice in localStorage as "tabler-theme" ("auto" when unset).
 *
 *   <a href="?theme=dark" class="btn" data-theme-choice="dark">Dark</a>
 *
 * @extends {Component<{}>}
 */
export class ThemeChoice extends Component {
  static attribute = "theme-choice";

  init() {
    let current = "auto";
    try {
      current = window.localStorage.getItem("tabler-theme") ?? "auto";
    } catch {
      // No storage: the default is in effect.
    }
    const on = this.el.dataset.themeChoice === current;
    // The primary style, not Bootstrap's grey "active" outline, which has too
    // little contrast for its text.
    this.el.classList.toggle("btn-primary", on);
    this.el.classList.toggle("btn-outline-secondary", !on);
    if (on) {
      this.el.setAttribute("aria-current", "true");
    }
    return this;
  }
}

/**
 * Tip marks an icon-only control's tooltip: data-tip holds what it does. The
 * tooltip itself is made by Ui.startTooltips, on hover or focus and only while
 * the control shows no text; this component starts those rules and makes sure
 * the control has an accessible name, which an icon alone does not give it.
 *
 *   <button class="btn btn-icon" data-tip="Copy the fingerprint">{{icon "copy"}}</button>
 *
 * A control with a visible label uses aria-description (the desc template
 * function) instead, and never gets a tooltip.
 *
 * @extends {Component<{}>}
 */
export class Tip extends Component {
  static attribute = "tip";

  init() {
    Ui.startTooltips();
    const text = this.el.getAttribute("data-tip") ?? "";
    if (text && !this.el.hasAttribute("aria-label") && !this.el.hasAttribute("aria-labelledby")) {
      this.el.setAttribute("aria-label", text);
    }
    return this;
  }
}
