// @ts-check
/**
 * El builds and changes DOM elements fluently, with DOM calls only: text is
 * always a text node and attributes are set one by one, so nothing is ever
 * parsed as HTML and the page's CSP (no inline script or style) holds.
 *
 *   El.create("button").cls("btn btn-sm").text("Next").attr("type", "button").on("click", next).node
 *
 * @template {HTMLElement} [T=HTMLElement]
 */
export class El {
  /** @type {T} */
  #node;

  /** @param {T} node */
  constructor(node) {
    this.#node = node;
  }

  /**
   * A new element of the given tag.
   * @template {keyof HTMLElementTagNameMap} K
   * @param {K} tag
   * @returns {El<HTMLElementTagNameMap[K]>}
   */
  static create(tag) {
    return new El(document.createElement(tag));
  }

  /**
   * Wraps an element that is already on the page.
   * @template {HTMLElement} E
   * @param {E} node
   * @returns {El<E>}
   */
  static of(node) {
    return new El(node);
  }

  /** The element. */
  get node() {
    return this.#node;
  }

  /** Adds classes, space-separated. @param {string} names */
  cls(names) {
    for (const n of names.split(/\s+/).filter(Boolean)) {
      this.#node.classList.add(n);
    }
    return this;
  }

  /** Adds or removes one class. @param {string} name @param {boolean} on */
  toggle(name, on) {
    this.#node.classList.toggle(name, on);
    return this;
  }

  /** Replaces the element's content with text. @param {string} value */
  text(value) {
    this.#node.textContent = value;
    return this;
  }

  /** Sets an attribute, or removes it when value is null. @param {string} name @param {string | null} value */
  attr(name, value) {
    if (value === null) {
      this.#node.removeAttribute(name);
    } else {
      this.#node.setAttribute(name, value);
    }
    return this;
  }

  /**
   * Sets a property on the element (id, type, value, disabled, htmlFor, …).
   * @template {keyof T} P
   * @param {P} name
   * @param {T[P]} value
   */
  prop(name, value) {
    this.#node[name] = value;
    return this;
  }

  /**
   * Listens for an event.
   * @param {string} type
   * @param {(e: Event) => void} handler
   */
  on(type, handler) {
    this.#node.addEventListener(type, handler);
    return this;
  }

  /** Appends children, elements or builders. @param {...(Node | El<any>)} children */
  append(...children) {
    for (const c of children) {
      this.#node.appendChild(c instanceof El ? c.node : c);
    }
    return this;
  }

  /** Removes every child. */
  empty() {
    this.#node.replaceChildren();
    return this;
  }

  /** Inserts this element before another node. @param {Node} ref */
  before(ref) {
    ref.parentNode?.insertBefore(this.#node, ref);
    return this;
  }

  /** Inserts this element after another node. @param {Node} ref */
  after(ref) {
    ref.parentNode?.insertBefore(this.#node, ref.nextSibling);
    return this;
  }

  /** Hides or shows the element with Tabler's d-none (it outranks [hidden] under d-flex). @param {boolean} hidden */
  hide(hidden = true) {
    return this.toggle("d-none", hidden);
  }
}
