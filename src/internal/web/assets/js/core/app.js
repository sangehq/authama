// @ts-check
/**
 * App wires the UI's components onto the page, fluently:
 *
 *   App.create().use(DataTable).use(ConfirmAction).start();
 *
 * `use` registers a component class; `start` mounts every registered one on
 * the root once the document is ready, and returns the instances by class.
 */
export class App {
  /** @type {import("./component.js").ComponentClass[]} */
  #components = [];

  /** @type {ParentNode} */
  #root;

  /** @type {Map<string, import("./component.js").Component<any>[]>} */
  #instances = new Map();

  /** @param {ParentNode} root */
  constructor(root) {
    this.#root = root;
  }

  /** An app over the whole document, or over root. @param {ParentNode} [root] */
  static create(root = document) {
    return new App(root);
  }

  /** Registers a component class. @param {import("./component.js").ComponentClass} component */
  use(component) {
    if (!this.#components.includes(component)) {
      this.#components.push(component);
    }
    return this;
  }

  /** Mounts every registered component once the document is ready. */
  start() {
    const run = () => {
      for (const c of this.#components) {
        this.#instances.set(c.name, c.mount(this.#root));
      }
    };
    // Content loaded into the page later (the side panel) brings elements of
    // its own; mount the same components on it.
    document.addEventListener("authama:mount", (e) => {
      const root = /** @type {CustomEvent<{ root: ParentNode }>} */ (e).detail?.root;
      if (root) {
        for (const c of this.#components) {
          c.mount(root);
        }
      }
    });
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", run, { once: true });
    } else {
      run();
    }
    return this;
  }

  /** The instances of one component, after start. @param {string} name */
  instances(name) {
    return this.#instances.get(name) ?? [];
  }
}
