// @ts-check
import { Component } from "../core/component.js";
import { humanDuration } from "./behaviours.js";

/** How long an action runs before the indicator shows. */
const SHOW_AFTER = 2000;
/** Where past durations are kept, per action. */
const STORE = "authama.durations";
/** Where a full-page submit leaves its start, for the next page to time. */
const PENDING = "authama.pending";

/**
 * SlowProgress shows that a request is still running once it has taken more
 * than two seconds: a spinner, the time so far, and - when this browser has
 * timed the same action before - a percentage and the time left, estimated
 * from the last runs and never shown past 95% until the answer arrives. An
 * action never timed shows a moving bar instead of a made-up number.
 *
 * The page carries one indicator, `<div data-slow-progress hidden>`. The side
 * panel calls SlowProgress.begin() around each load and submit; a form that
 * reloads the whole page is timed across the reload.
 *
 * @extends {Component<{}>}
 */
export class SlowProgress extends Component {
  static attribute = "slow-progress";

  /** @type {SlowProgress | null} */
  static current = null;

  /** @type {{key: string, start: number, label: string} | null} */
  #run = null;
  #show = 0;
  #tick = 0;

  init() {
    SlowProgress.current = this;
    // A full-page submit started on the previous page: record how long it took.
    try {
      const raw = window.sessionStorage.getItem(PENDING);
      if (raw) {
        window.sessionStorage.removeItem(PENDING);
        const p = JSON.parse(raw);
        if (typeof p?.key === "string" && typeof p?.start === "number") {
          SlowProgress.record(p.key, Date.now() - p.start);
        }
      }
    } catch {
      // Storage is a convenience: without it nothing is timed.
    }
    document.addEventListener("submit", (e) => {
      const form = e.target;
      // The side panel takes over its own forms and times them itself.
      if (e.defaultPrevented || !(form instanceof HTMLFormElement) || form.method.toLowerCase() !== "post") {
        return;
      }
      const key = SlowProgress.key(form);
      try {
        window.sessionStorage.setItem(PENDING, JSON.stringify({ key, start: Date.now() }));
      } catch {
        // Not timed.
      }
      this.begin(key, SlowProgress.label(/** @type {SubmitEvent} */ (e).submitter));
    });
    return this;
  }

  /**
   * The name a request's duration is kept under: the method, the path and the
   * form's action, so "preview a host add" is timed apart from "preview a
   * key delete".
   * @param {HTMLFormElement} form
   */
  static key(form) {
    const path = new URL(form.getAttribute("action") ?? window.location.href, window.location.href).pathname;
    const action = /** @type {HTMLInputElement | null} */ (form.querySelector('input[name="action"]'))?.value ?? "";
    return `${form.method.toUpperCase()} ${path} ${action}`.trim();
  }

  /** What the indicator calls a request: the button that sent it. @param {HTMLElement | null | undefined} submitter */
  static label(submitter) {
    const text = (submitter?.textContent ?? "").replace(/\s+/g, " ").trim();
    return text ? `${text}…` : "Working…";
  }

  /** @param {string} key @param {number} ms */
  static record(key, ms) {
    if (!(ms > 0) || ms > 30 * 60 * 1000) {
      return;
    }
    try {
      const all = JSON.parse(window.localStorage.getItem(STORE) ?? "{}");
      const was = Number(all[key]);
      // A moving average, so one slow run does not set the estimate alone.
      all[key] = was > 0 ? Math.round(was * 0.6 + ms * 0.4) : ms;
      window.localStorage.setItem(STORE, JSON.stringify(all));
    } catch {
      // Not kept.
    }
  }

  /** @param {string} key @returns {number} 0 when never timed */
  static typical(key) {
    try {
      return Number(JSON.parse(window.localStorage.getItem(STORE) ?? "{}")[key]) || 0;
    } catch {
      return 0;
    }
  }

  /**
   * Starts timing a request. The returned function ends it: call it when the
   * answer has arrived, with ok false for a request that failed or was
   * aborted, which is not timed.
   * @param {string} key @param {string} [label]
   * @returns {(ok?: boolean) => void}
   */
  static begin(key, label = "Working…") {
    const me = SlowProgress.current;
    if (!me) {
      return () => {};
    }
    return me.begin(key, label);
  }

  /** @param {string} key @param {string} label @returns {(ok?: boolean) => void} */
  begin(key, label) {
    this.end();
    const run = { key, start: Date.now(), label };
    this.#run = run;
    this.#show = window.setTimeout(() => {
      this.el.hidden = false;
      this.#draw();
      this.#tick = window.setInterval(() => this.#draw(), 250);
    }, SHOW_AFTER);
    return (ok = true) => {
      if (this.#run !== run) {
        return;
      }
      if (ok) {
        SlowProgress.record(key, Date.now() - run.start);
      }
      this.end();
    };
  }

  /** Hides the indicator. */
  end() {
    window.clearTimeout(this.#show);
    window.clearInterval(this.#tick);
    this.#run = null;
    this.el.hidden = true;
    return this;
  }

  #draw() {
    const run = this.#run;
    if (!run) {
      return;
    }
    const elapsed = Date.now() - run.start;
    const typical = SlowProgress.typical(run.key);
    const label = this.el.querySelector("[data-slow-progress-label]");
    const meta = this.el.querySelector("[data-slow-progress-meta]");
    const bar = this.el.querySelector("[data-slow-progress-bar]");
    if (label) {
      label.textContent = run.label;
    }
    let text = humanDuration(elapsed);
    if (typical > 0 && bar instanceof HTMLProgressElement) {
      const pct = Math.min(95, Math.floor((elapsed * 100) / typical));
      bar.max = 100;
      bar.value = pct;
      const left = typical - elapsed;
      text += ` · ${pct}% · ${left > 1000 ? `about ${humanDuration(left)} left` : "taking longer than last time"}`;
    } else if (bar instanceof HTMLProgressElement) {
      bar.removeAttribute("value");
      text += " · first time timed here, so no estimate yet";
    }
    if (meta) {
      meta.textContent = text;
    }
  }
}
