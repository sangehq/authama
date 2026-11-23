// @ts-check
import { Component } from "../core/component.js";
import { El } from "../core/el.js";
import { Ui } from "../core/ui.js";

/**
 * LogFollow adds the records written since the page loaded to the top of the
 * Logs table while its switch is on, asking /logs/tail every two seconds with
 * the page's filters. Rows are built with DOM calls only.
 *
 *   <input type="checkbox" data-log-follow data-log-follow-offset="N" data-log-follow-target="log-rows">
 *
 * @extends {Component<{ offset: number, target: string, every: number }>}
 */
export class LogFollow extends Component {
  static attribute = "log-follow";
  static defaults = { offset: 0, target: "log-rows", every: 2000 };

  /** @type {number | undefined} */
  #timer;

  /** @type {number} */
  #offset = 0;

  init() {
    const box = /** @type {HTMLInputElement} */ (this.el);
    this.#offset = Number(this.options.offset) || 0;
    box.addEventListener("change", () => (box.checked ? this.start() : this.stop()));
    return this;
  }

  start() {
    this.stop();
    this.#timer = window.setInterval(() => void this.poll(), Number(this.options.every));
    void this.poll();
  }

  stop() {
    if (this.#timer !== undefined) {
      window.clearInterval(this.#timer);
      this.#timer = undefined;
    }
  }

  async poll() {
    const q = new URLSearchParams(window.location.search);
    q.set("offset", String(this.#offset));
    try {
      const res = await fetch(`${Ui.path("/logs/tail")}?${q.toString()}`, { cache: "no-store", credentials: "same-origin" });
      if (!res.ok) {
        return;
      }
      /** @type {{ offset: number, rows: { time: string, level: string, msg: string, attrs: string }[] }} */
      const data = await res.json();
      this.#offset = data.offset;
      const body = document.getElementById(String(this.options.target));
      if (!body) {
        return;
      }
      for (const row of data.rows) {
        const badge = { error: "badge-bad", warn: "badge-warn", debug: "badge-muted" }[row.level] ?? "badge-info";
        const tr = El.create("tr").append(
          El.create("td").cls("text-nowrap text-secondary").text(row.time).node,
          El.create("td").append(El.create("span").cls(`badge ${badge}`).text(row.level).node).node,
          El.create("td").text(row.msg).node,
          El.create("td").cls("text-break small").append(El.create("code").text(row.attrs).node).node,
        ).node;
        body.prepend(tr);
      }
    } catch {
      // The server is restarting or gone; the next tick tries again.
    }
  }
}

/**
 * ZonePreview shows the time now in the zone typed into the Settings page's
 * time zone field, or says the name is not a zone.
 *
 *   <input data-tz-preview="<id of the element to write into>">
 *
 * @extends {Component<{}>}
 */
export class ZonePreview extends Component {
  static attribute = "tz-preview";

  init() {
    const input = /** @type {HTMLInputElement} */ (this.el);
    const out = document.getElementById(input.dataset.tzPreview ?? "");
    if (!out) {
      return this;
    }
    const show = () => {
      const zone = input.value.trim();
      try {
        const fmt = new Intl.DateTimeFormat(undefined, {
          timeZone: zone || undefined,
          year: "numeric",
          month: "short",
          day: "numeric",
          hour: "2-digit",
          minute: "2-digit",
          timeZoneName: "short",
        });
        out.textContent = `${fmt.format(new Date())}${zone ? "" : " (this system's zone)"}`;
      } catch {
        out.textContent = "not a time zone name";
      }
    };
    input.addEventListener("input", show);
    show();
    window.setInterval(show, 30000);
    return this;
  }
}
