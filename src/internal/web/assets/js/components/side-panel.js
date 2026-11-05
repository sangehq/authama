// @ts-check
import { Component } from "../core/component.js";
import { Ui } from "../core/ui.js";

/** The request header that asks the server for a page's content alone. */
const PANEL_HEADER = "X-Authama-Panel";

/** Paths whose POST makes a real change: the page behind is reloaded when the panel closes. */
const CHANGING = ["/changes/apply", "/jobs"];

/**
 * @typedef {{ url: string, title: string, node: Element }} PanelPage
 */

/**
 * SidePanel is the one slide-in panel every in-place page opens in: a change
 * preview, its confirmation and result, a job's review and output, an
 * operation in the history. The server renders the page's content alone when
 * the request carries X-Authama-Panel, so nothing is parsed into the page but
 * what the server's templates escaped.
 *
 * Pages stack: following a form or a marked link inside the panel pushes a
 * page, and Back pops one. When a change was made inside the panel, the page
 * underneath is reloaded as the panel closes, so it shows the new state.
 *
 *   SidePanel.current?.open("/history/224f5bf1").then(...)
 *
 * The layout renders the panel's shell (data-side-panel); without this
 * script, every form and link it would have taken goes to its own page.
 *
 * @extends {Component<{}>}
 */
export class SidePanel extends Component {
  static attribute = "side-panel";

  /** @type {SidePanel | null} */
  static current = null;

  /** @type {PanelPage[]} */
  #stack = [];

  #changed = false;

  /** @type {AbortController | null} */
  #inflight = null;

  init() {
    SidePanel.current = this;
    this.#part("back")?.addEventListener("click", () => this.back());
    this.el.addEventListener("hidden.bs.offcanvas", () => this.#closed());
    // Inside the panel, every form continues in it, and so does every link
    // marked data-inline: a preview's "Make this change", a job's answer.
    this.#body().addEventListener("submit", (e) => {
      const form = e.target;
      if (form instanceof HTMLFormElement && !form.hasAttribute("data-panel-skip")) {
        e.preventDefault();
        this.submit(form, /** @type {SubmitEvent} */ (e).submitter);
      }
    });
    this.#body().addEventListener("click", (e) => {
      const a = e.target instanceof Element ? e.target.closest("a[data-inline]") : null;
      if (a instanceof HTMLAnchorElement && SidePanel.local(a)) {
        e.preventDefault();
        this.open(a.href);
      }
    });
    return this;
  }

  /** Whether a link stays on this server and is not a download. @param {HTMLAnchorElement} a */
  static local(a) {
    return a.origin === window.location.origin && !a.hasAttribute("download") && !a.target && !a.pathname.startsWith("/assets/");
  }

  /** Opens url as a new page in the panel. @param {string} url */
  open(url) {
    return this.#load(url, { method: "GET" });
  }

  /**
   * Sends form, as the button that submitted it would, and shows the answer
   * in the panel.
   * @param {HTMLFormElement} form
   * @param {HTMLElement | null} [submitter]
   */
  submit(form, submitter = null) {
    const method = (form.getAttribute("method") ?? "get").toUpperCase();
    const data = new FormData(form, submitter ?? undefined);
    const action = new URL(form.action, window.location.href);
    if (method === "GET") {
      action.search = new URLSearchParams(/** @type {any} */ (data)).toString();
      return this.open(action.href);
    }
    if (CHANGING.includes(action.pathname)) {
      this.#changed = true;
    }
    this.#busy(form, true);
    return this.#load(action.href, { method, body: data }).finally(() => this.#busy(form, false));
  }

  /** Goes back one page, or closes the panel at the first. */
  back() {
    if (this.#stack.length <= 1) {
      Ui.offcanvas(this.el)?.hide();
      return this;
    }
    this.#stack.pop();
    this.#show(/** @type {PanelPage} */ (this.#stack.at(-1)));
    return this;
  }

  /**
   * @param {string} url
   * @param {RequestInit} init
   */
  async #load(url, init) {
    this.#inflight?.abort();
    this.#inflight = new AbortController();
    Ui.offcanvas(this.el)?.show();
    this.el.setAttribute("aria-busy", "true");
    try {
      const res = await fetch(url, {
        ...init,
        credentials: "same-origin",
        headers: { [PANEL_HEADER]: "1" },
        signal: this.#inflight.signal,
      });
      const type = res.headers.get("Content-Type") ?? "";
      if (!type.startsWith("text/html")) {
        // A download or anything else the panel cannot show: let the browser have it.
        window.location.assign(res.url);
        return this;
      }
      const doc = new DOMParser().parseFromString(await res.text(), "text/html");
      const page = doc.querySelector("[data-panel-page]") ?? doc.querySelector("main") ?? doc.body;
      const node = document.importNode(page, true);
      const entry = { url: res.url, title: page.getAttribute("data-title") ?? doc.title, node };
      this.#stack.push(entry);
      this.#show(entry);
    } catch (err) {
      if (!(err instanceof DOMException && err.name === "AbortError")) {
        this.#stack.push({ url, title: "Could not load", node: SidePanel.#problem(String(err)) });
        this.#show(/** @type {PanelPage} */ (this.#stack.at(-1)));
      }
    } finally {
      this.el.removeAttribute("aria-busy");
    }
    return this;
  }

  /** @param {PanelPage} page */
  #show(page) {
    const body = this.#body();
    body.replaceChildren(page.node);
    const title = this.#part("title");
    if (title) {
      title.textContent = page.title;
    }
    this.#part("back")?.toggleAttribute("hidden", this.#stack.length <= 1);
    const full = this.#part("full");
    if (full instanceof HTMLAnchorElement) {
      full.href = page.url;
    }
    body.scrollTop = 0;
    document.dispatchEvent(new CustomEvent("authama:mount", { detail: { root: body } }));
    Ui.tooltips(body);
    // The panel's heading takes focus, so a screen reader announces the page.
    const heading = body.querySelector("h1, h2");
    if (heading instanceof HTMLElement) {
      heading.tabIndex = -1;
      heading.focus({ preventScroll: true });
    }
  }

  #closed() {
    this.#inflight?.abort();
    this.#stack = [];
    this.#body().replaceChildren();
    if (this.#changed) {
      this.#changed = false;
      window.location.reload();
    }
  }

  /** @param {HTMLFormElement} form @param {boolean} on */
  #busy(form, on) {
    for (const b of form.querySelectorAll("button[type=submit], button:not([type])")) {
      if (b instanceof HTMLButtonElement) {
        b.disabled = on;
        b.classList.toggle("btn-loading", on);
      }
    }
  }

  #body() {
    return /** @type {HTMLElement} */ (this.#part("body"));
  }

  /** @param {string} name */
  #part(name) {
    return this.el.querySelector(`[data-side-panel-${name}]`);
  }

  /** @param {string} message */
  static #problem(message) {
    const p = document.createElement("div");
    p.className = "alert alert-danger";
    p.setAttribute("role", "alert");
    p.textContent = `The page could not be loaded: ${message}`;
    return p;
  }
}

/**
 * Inline sends a form or a link marked data-inline on the page itself into the
 * side panel instead of to a new page. A form inside another slide-in panel
 * (a "Set lifetime" form, say) closes that panel first.
 *
 *   <form method="post" action="/changes/preview" data-inline>…</form>
 *   <a href="/history/224f5bf1" data-inline>…</a>
 *
 * @extends {Component<{}>}
 */
export class Inline extends Component {
  static attribute = "inline";

  init() {
    // Inside the side panel, the panel's own listeners handle it.
    if (this.el.closest("[data-side-panel]")) {
      return this;
    }
    if (this.el instanceof HTMLFormElement) {
      const form = this.el;
      form.addEventListener("submit", (e) => {
        const panel = SidePanel.current;
        if (!panel || e.defaultPrevented) {
          return;
        }
        e.preventDefault();
        Ui.closeOverlays();
        panel.submit(form, /** @type {SubmitEvent} */ (e).submitter);
      });
    } else if (this.el instanceof HTMLAnchorElement) {
      const a = this.el;
      a.addEventListener("click", (e) => {
        const panel = SidePanel.current;
        // A modified click (new tab, new window) keeps its meaning.
        if (!panel || !SidePanel.local(a) || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) {
          return;
        }
        e.preventDefault();
        panel.open(a.href);
      });
    }
    return this;
  }
}
