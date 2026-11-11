// @ts-check
import { Component } from "../core/component.js";
import { Ui } from "../core/ui.js";

/** The flag a restart leaves for the reloaded page, which then says it is current. */
const RESTARTED = "authama.restarted";

/**
 * RestartServer is the stale-build banner's "Restart now": it asks the
 * server to restart from the binary now installed, waits until a new server
 * answers on the same address, signs back in with the same token (the old
 * process's sessions end with it) and reloads the page it was on.
 *
 *   <button data-restart-server>Restart now</button>
 *
 * @extends {Component<{ wait: number }>}
 */
export class RestartServer extends Component {
  static attribute = "restart-server";
  static defaults = { wait: 30000 };

  init() {
    this.el.addEventListener("click", () => this.restart());
    return this;
  }

  /** @param {string} text */
  #say(text) {
    const status = document.querySelector("[data-restart-status]");
    if (status) {
      status.textContent = text;
    }
  }

  async restart() {
    const button = /** @type {HTMLButtonElement} */ (this.el);
    button.disabled = true;
    this.#say("Restarting…");
    try {
      const csrf = document.querySelector('meta[name="csrf-token"]')?.getAttribute("content") ?? "";
      const res = await fetch(Ui.path("/server/restart"), {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-CSRF-Token": csrf },
      });
      /** @type {{ instance?: string, sign_in?: string, error?: string }} */
      const reply = await res.json();
      if (!res.ok || !reply.instance || !reply.sign_in) {
        throw new Error(reply.error ?? `the server answered ${res.status}`);
      }
      await this.#waitForNewServer(reply.instance);
      try {
        sessionStorage.setItem(RESTARTED, "1");
      } catch {
        // no storage: the page still reloads, without the confirmation
      }
      const token = new URLSearchParams(reply.sign_in.replace(/^\?/, "")).get("token") ?? "";
      const url = new URL(Ui.reloadAddress());
      url.searchParams.set("token", token);
      location.replace(url.toString());
    } catch (err) {
      button.disabled = false;
      this.#say(`Could not restart: ${err instanceof Error ? err.message : err}`);
    }
  }

  /** Waits until /healthz answers as another instance than old. @param {string} old */
  async #waitForNewServer(old) {
    const deadline = Date.now() + Number(this.options.wait);
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 400));
      try {
        const res = await fetch(Ui.path("/healthz"), { cache: "no-store" });
        if (res.ok) {
          /** @type {{ app?: string, instance?: string }} */
          const h = await res.json();
          if (h.app === "authama" && h.instance && h.instance !== old) {
            return;
          }
        }
      } catch {
        // between the old server stopping and the new one listening
      }
    }
    throw new Error("no new server answered; run `authama serve --restart` in a terminal");
  }
}

/**
 * Restarted shows the "Restarted, and current" banner once, on the page a
 * restart reloaded.
 *
 *   <div data-restarted hidden>…</div>
 *
 * @extends {Component<{}>}
 */
export class Restarted extends Component {
  static attribute = "restarted";
  static defaults = {};

  init() {
    try {
      if (sessionStorage.getItem(RESTARTED)) {
        sessionStorage.removeItem(RESTARTED);
        this.el.hidden = false;
      }
    } catch {
      // no storage: no banner
    }
    return this;
  }
}

/**
 * LiveBuild keeps a page on the server's current build: it asks /healthz
 * every few seconds while the page is visible, and when a new server answers
 * (the old one restarted into a new build) it reloads, and the reloaded page
 * says it is current. A page with unsaved input in a form is not reloaded
 * under the user: it shows a "Reload" prompt instead.
 *
 *   <meta name="authama-instance" content="…">
 *   <div data-live-build hidden>… <button data-live-build-reload>Reload</button></div>
 *
 * @extends {Component<{ every: number }>}
 */
export class LiveBuild extends Component {
  static attribute = "live-build";
  static defaults = { every: 3000 };

  #dirty = false;

  #timer = 0;

  init() {
    const instance = document.querySelector('meta[name="authama-instance"]')?.getAttribute("content");
    if (!instance) {
      return this;
    }
    // Typing in a form makes the page dirty; sending it, clean again.
    document.addEventListener("input", (e) => {
      if (e.target instanceof Element && e.target.closest("form")) {
        this.#dirty = true;
      }
    });
    document.addEventListener("submit", () => {
      this.#dirty = false;
    });
    this.el.querySelector("[data-live-build-reload]")?.addEventListener("click", () => this.#reload());
    this.#timer = window.setInterval(() => this.#check(instance), Number(this.options.every));
    return this;
  }

  /** @param {string} instance */
  async #check(instance) {
    if (document.visibilityState !== "visible") {
      return;
    }
    try {
      const res = await fetch(Ui.path("/healthz"), { cache: "no-store" });
      if (!res.ok) {
        return;
      }
      /** @type {{ app?: string, instance?: string }} */
      const h = await res.json();
      if (h.app !== "authama" || !h.instance || h.instance === instance) {
        return;
      }
      window.clearInterval(this.#timer);
      if (this.#dirty) {
        this.el.hidden = false;
      } else {
        this.#reload();
      }
    } catch {
      // the server is between builds; ask again next time
    }
  }

  #reload() {
    try {
      sessionStorage.setItem(RESTARTED, "1");
    } catch {
      // no storage: the page reloads without the confirmation
    }
    Ui.reload();
  }
}
