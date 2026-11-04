// @ts-check
import { Options } from "./options.js";

/**
 * A component class, whatever its options: what App.use takes.
 * @typedef {{ name: string, attribute: string, mount(root?: ParentNode): Component<any>[] }} ComponentClass
 */

/**
 * Component is the base of every behaviour the UI adds. A subclass names the
 * data attribute that marks its elements and its options' defaults; `mount`
 * finds every marked element and builds one instance each, reading the
 * options from the element's data attributes. An element is mounted once.
 *
 *   class Foo extends Component {
 *     static attribute = "foo";              // data-foo marks it
 *     static defaults = { delay: 0 };        // data-foo-delay="200"
 *     init() { … }
 *   }
 *
 * @template {Record<string, string | number | boolean>} [O=Record<string, string | number | boolean>]
 */
export class Component {
  /** The data attribute, without "data-", that marks this component's elements. */
  static attribute = "";

  /** @type {Record<string, string | number | boolean>} The options' defaults; data attributes override them. */
  static defaults = {};

  /** @type {WeakMap<HTMLElement, Set<string>>} */
  static #mounted = new WeakMap();

  /** @type {HTMLElement} */
  #el;

  /** @type {O} */
  #options;

  /**
   * @param {HTMLElement} el
   * @param {Partial<O>} [overrides] options set in code, over the data attributes
   */
  constructor(el, overrides = {}) {
    const ctor = /** @type {typeof Component} */ (this.constructor);
    this.#el = el;
    this.#options = /** @type {O} */ ({ ...Options.read(el, ctor.attribute, ctor.defaults), ...overrides });
  }

  /** The element this instance drives. */
  get el() {
    return this.#el;
  }

  /** The options, after data attributes and overrides. */
  get options() {
    return this.#options;
  }

  /** Sets up the behaviour; a subclass overrides it. Returns this, for chaining. */
  init() {
    return this;
  }

  /**
   * Builds and initialises an instance on every element marked with the
   * subclass's attribute under root, skipping any already mounted.
   * @template {typeof Component} C
   * @this {C}
   * @param {ParentNode} [root]
   * @returns {InstanceType<C>[]}
   */
  static mount(root = document) {
    if (!this.attribute) {
      throw new Error(`${this.name} names no data attribute`);
    }
    /** @type {InstanceType<C>[]} */
    const out = [];
    for (const el of root.querySelectorAll(`[data-${this.attribute}]`)) {
      if (!(el instanceof HTMLElement) || Component.#isMounted(el, this.attribute)) {
        continue;
      }
      const c = /** @type {InstanceType<C>} */ (new this(el));
      c.init();
      out.push(c);
    }
    return out;
  }

  /** @param {HTMLElement} el @param {string} attribute */
  static #isMounted(el, attribute) {
    const set = Component.#mounted.get(el) ?? new Set();
    if (set.has(attribute)) {
      return true;
    }
    set.add(attribute);
    Component.#mounted.set(el, set);
    return false;
  }
}
