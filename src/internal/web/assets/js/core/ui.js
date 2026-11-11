// @ts-check
/**
 * Ui is the one place the page reaches Tabler's bundled Bootstrap, so a
 * component asks for "the offcanvas for this element" instead of each one
 * looking up window.tabler and guarding against its absence.
 *
 *   Ui.offcanvas(panel)?.show();
 *   Ui.modal(dialog)?.hide();
 *   Ui.tooltips(root);   // popovers that content loaded later brings
 *   Ui.startTooltips();  // the tooltip rules, once per page
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

  /**
   * An icon from the page's sprite, as the server draws one with {{icon}}:
   * decorative, so hidden from screen readers.
   * @param {string} name a Tabler icon the sprite holds
   */
  static icon(name) {
    const ns = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(ns, "svg");
    svg.setAttribute("class", "icon");
    svg.setAttribute("aria-hidden", "true");
    svg.setAttribute("focusable", "false");
    const use = document.createElementNS(ns, "use");
    use.setAttribute("href", `#tabler-${name}`);
    svg.appendChild(use);
    return svg;
  }

  /**
   * The address this page should be reloaded at. A page that answers a form
   * (a preview, a job's review) names the page the form came from: asking for
   * its own address again would be a GET, which has nothing to show.
   */
  static reloadAddress() {
    const to = document.querySelector('meta[name="authama-reload-to"]')?.getAttribute("content");
    return to ? new URL(to, location.href).toString() : location.href;
  }

  /** Reloads the page, or goes to the page a form answer came from. */
  static reload() {
    const to = Ui.reloadAddress();
    if (to === location.href) {
      location.reload();
    } else {
      location.replace(to);
    }
  }

  /** The Offcanvas instance for el, or null without Tabler. @param {Element} el */
  static offcanvas(el) {
    return Ui.#lib?.Offcanvas?.getOrCreateInstance(el) ?? null;
  }

  /** The Modal instance for el, or null without Tabler. @param {Element} el */
  static modal(el) {
    return Ui.#lib?.Modal?.getOrCreateInstance(el) ?? null;
  }

  /** Starts the popovers under root that content loaded later brings. @param {ParentNode} root */
  static tooltips(root) {
    const lib = Ui.#lib;
    if (!lib) {
      return;
    }
    for (const el of root.querySelectorAll('[data-bs-toggle="popover"]')) {
      lib.Popover?.getOrCreateInstance(el);
    }
  }

  /**
   * Hides the popovers open under root, and the tooltip if one is showing.
   * Popovers and tooltips are drawn on <body>, so content swapped out from
   * under an open one would leave it on screen.
   * @param {ParentNode} root
   */
  static hideTips(root) {
    Ui.hideTooltip();
    const lib = Ui.#lib;
    if (!lib) {
      return;
    }
    const els = root instanceof Element ? [root, ...root.querySelectorAll("*")] : [...root.querySelectorAll("*")];
    for (const el of els) {
      lib.Popover?.getInstance(el)?.hide();
    }
  }

  // ---- tooltips ------------------------------------------------------------
  //
  // Tooltips are for icon-only controls, and this is the one place that makes
  // them. A control that shows its name (a labelled button, a nav link, a
  // menu entry, a dropdown toggle) never gets one: it would cover the thing
  // pointed at and repeat its label, so its explainer is aria-description, or
  // a line of text in its menu. The rules:
  //
  //  - made on hover or keyboard focus of a [data-tip] element that shows no
  //    text, after half a second, and disposed as soon as it hides, so no
  //    instance outlives its element when content is swapped;
  //  - one at a time;
  //  - hidden on any click, Escape or scroll, and when a dropdown, slide-in
  //    panel or dialog opens; never shown while a dropdown is open;
  //  - hidden the moment the pointer leaves its control or the window, the
  //    window loses focus or the tab is hidden; keyboard focus shows one only
  //    when it came from the keyboard (:focus-visible), so a clicked control
  //    does not keep one;
  //  - a watchdog checks, while one shows, that its control is still on the
  //    page and still hovered or keyboard-focused, so no missed event (a
  //    redraw under the pointer, a removed control) can leave one behind;
  //  - never on a touch screen, which has no hover to end one.

  /** @type {{ el: Element, tip: any } | null} */
  static #tip = null;
  /** @type {number | undefined} */
  static #timer = undefined;
  /** @type {number | undefined} */
  static #watch = undefined;
  static #started = false;

  /** Starts the tooltip rules once for the whole page. */
  static startTooltips() {
    if (Ui.#started || !Ui.#lib?.Tooltip) {
      return;
    }
    Ui.#started = true;
    if (window.matchMedia?.("(hover: none)").matches) {
      return;
    }
    const begin = (/** @type {Event} */ e) => {
      const el = e.target instanceof Element ? e.target.closest("[data-tip]") : null;
      if (!el || el === Ui.#tip?.el || !Ui.tipAllowed(el)) {
        return;
      }
      // Focus from a click is not a reason to explain the control.
      if (e.type === "focusin" && !el.matches(":focus-visible")) {
        return;
      }
      Ui.hideTooltip();
      Ui.#timer = window.setTimeout(() => Ui.#show(el), 500);
    };
    const end = (/** @type {Event} */ e) => {
      const el = e.target instanceof Element ? e.target.closest("[data-tip]") : null;
      const to = /** @type {MouseEvent | FocusEvent} */ (e).relatedTarget;
      if (el && !(to instanceof Node && el.contains(to))) {
        Ui.hideTooltip();
      }
    };
    document.addEventListener("mouseover", begin);
    document.addEventListener("focusin", begin);
    document.addEventListener("mouseout", end);
    document.addEventListener("focusout", end);
    const hide = () => Ui.hideTooltip();
    document.addEventListener("click", hide, true);
    document.addEventListener("scroll", hide, { capture: true, passive: true });
    document.addEventListener("pointerdown", hide, true);
    document.documentElement.addEventListener("mouseleave", hide);
    document.addEventListener("visibilitychange", hide);
    window.addEventListener("blur", hide);
    window.addEventListener("resize", hide, { passive: true });
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape") {
        hide();
      }
    });
    for (const ev of ["show.bs.dropdown", "show.bs.offcanvas", "show.bs.modal"]) {
      document.addEventListener(ev, hide);
    }
  }

  /**
   * Whether el may have a tooltip: it carries data-tip, shows no text of its
   * own, is not a dropdown toggle or inside a menu, and no dropdown is open.
   * @param {Element} el
   */
  static tipAllowed(el) {
    if (!el.getAttribute("data-tip") || document.querySelector(".dropdown-menu.show")) {
      return false;
    }
    if (el.matches('[data-bs-toggle="dropdown"]') || el.closest(".dropdown-menu")) {
      return false;
    }
    return !Ui.showsText(el);
  }

  /** Whether el shows any text (screen-reader-only and hidden text aside). @param {Element} el */
  static showsText(el) {
    const walk = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
    for (let n = walk.nextNode(); n; n = walk.nextNode()) {
      const parent = n.parentElement;
      if (!n.textContent?.trim() || !parent || parent.closest(".visually-hidden, [hidden], kbd")) {
        continue;
      }
      if (parent.getClientRects().length > 0) {
        return true;
      }
    }
    return false;
  }

  /** @param {Element} el @param {string} [text] */
  static #show(el, text) {
    const lib = Ui.#lib;
    if (!lib?.Tooltip || !el.isConnected) {
      return null;
    }
    Ui.hideTooltip();
    const order = Ui.tipPlacements(el);
    const tip = new lib.Tooltip(el, {
      title: text ?? el.getAttribute("data-tip") ?? "",
      placement: order[0],
      // If the page moves under it, Popper flips through the rest in order.
      fallbackPlacements: order.slice(1),
      trigger: "manual",
      container: "body",
      // Clear of the control, and never touching the edge of the window.
      offset: [0, 6],
      popperConfig: (/** @type {any} */ cfg) => ({
        ...cfg,
        modifiers: [
          ...(cfg.modifiers ?? []).filter((/** @type {any} */ m) => m.name !== "preventOverflow"),
          { name: "preventOverflow", options: { boundary: "viewport", padding: 8, altAxis: true } },
        ],
      }),
    });
    Ui.#tip = { el, tip };
    el.addEventListener("hidden.bs.tooltip", () => {
      tip.dispose();
      if (Ui.#tip?.tip === tip) {
        Ui.#tip = null;
        window.clearInterval(Ui.#watch);
      }
    }, { once: true });
    tip.show();
    // The watchdog: a flash (text given) holds for its own timeout; any other
    // tooltip lasts only while its control is on the page and hovered or
    // keyboard-focused.
    window.clearInterval(Ui.#watch);
    if (text === undefined) {
      Ui.#watch = window.setInterval(() => {
        if (!el.isConnected || !(el.matches(":hover") || el.matches(":focus-visible"))) {
          Ui.hideTooltip();
        }
      }, 250);
    }
    return tip;
  }

  /**
   * The sides a tooltip may open on, best first: the one with the most room,
   * above or below before left or right, because a line of text is wide. A
   * side that cannot fit a tooltip (about 48px tall, 160px wide) goes last,
   * so a control at the top of the window opens downward, one at the right
   * edge opens to the left, and the tooltip never sits over its control. A
   * data-tip-placement on the control is the first choice when it has room.
   * @param {Element} el
   * @returns {string[]}
   */
  static tipPlacements(el) {
    const r = el.getBoundingClientRect();
    const room = {
      top: r.top,
      bottom: window.innerHeight - r.bottom,
      left: r.left,
      right: window.innerWidth - r.right,
    };
    const need = { top: 48, bottom: 48, left: 160, right: 160 };
    /** @param {"top" | "bottom" | "left" | "right"} side */
    const fits = (side) => room[side] >= need[side];
    const vertical = /** @type {const} */ (["top", "bottom"]).slice().sort((a, b) => room[b] - room[a]);
    const horizontal = /** @type {const} */ (["right", "left"]).slice().sort((a, b) => room[b] - room[a]);
    const ranked = [...vertical, ...horizontal].sort((a, b) => Number(fits(b)) - Number(fits(a)));
    const asked = el.getAttribute("data-tip-placement");
    if (asked && ranked.includes(/** @type {any} */ (asked)) && fits(/** @type {any} */ (asked))) {
      return [asked, ...ranked.filter((s) => s !== asked)];
    }
    return ranked;
  }

  /** Hides (and so disposes) the tooltip that is showing, and any about to. */
  static hideTooltip() {
    window.clearTimeout(Ui.#timer);
    window.clearInterval(Ui.#watch);
    const t = Ui.#tip;
    if (!t) {
      return;
    }
    // A control already gone from the page cannot run Bootstrap's hide, so
    // its tooltip is removed directly.
    if (!t.el.isConnected) {
      Ui.#tip = null;
      t.tip.tip?.remove();
      t.tip.dispose();
      return;
    }
    t.tip.hide();
  }

  /**
   * Says message in a tooltip on el for a moment ("Copied" on a copy
   * button). Returns false when there is no tooltip to say it in (a touch
   * screen, a labelled control, a menu entry), so the caller uses a toast.
   * @param {Element} el @param {string} message
   */
  static flashTip(el, message) {
    if (window.matchMedia?.("(hover: none)").matches || el.closest(".dropdown-menu") || Ui.showsText(el)) {
      return false;
    }
    const tip = Ui.#show(el, message);
    if (!tip) {
      return false;
    }
    window.setTimeout(() => {
      if (Ui.#tip?.tip === tip) {
        Ui.hideTooltip();
      }
    }, 1500);
    return true;
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
