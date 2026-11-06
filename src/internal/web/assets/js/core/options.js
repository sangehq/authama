// @ts-check
/**
 * Options reads a component's configuration from data attributes on its
 * element, typed by the defaults it is given: `data-table-page-size="25"`
 * becomes `pageSize: 25` for a component named `table` whose default is a
 * number. A missing or unparsable attribute keeps the default.
 *
 *   const opts = Options.read(el, "table", { pageSize: 10, searchable: true });
 */
export class Options {
  /**
   * @template {Record<string, string | number | boolean>} D
   * @param {HTMLElement} el
   * @param {string} prefix the component's data-attribute name, e.g. "table"
   * @param {D} defaults
   * @returns {D}
   */
  static read(el, prefix, defaults) {
    /** @type {Record<string, string | number | boolean>} */
    const out = { ...defaults };
    for (const [key, fallback] of Object.entries(defaults)) {
      const raw = el.dataset[Options.datasetKey(prefix, key)];
      if (raw === undefined) {
        continue;
      }
      out[key] = Options.parse(raw, fallback);
    }
    return /** @type {D} */ (out);
  }

  /**
   * The dataset key for an option: prefix "table" and key "pageSize" read
   * data-table-page-size, which the browser exposes as dataset.tablePageSize.
   * @param {string} prefix
   * @param {string} key
   */
  static datasetKey(prefix, key) {
    const camel = prefix.replace(/-([a-z])/g, (_, c) => c.toUpperCase());
    return camel + key.charAt(0).toUpperCase() + key.slice(1);
  }

  /**
   * Parses raw text as the default's type.
   * @param {string} raw
   * @param {string | number | boolean} fallback
   * @returns {string | number | boolean}
   */
  static parse(raw, fallback) {
    switch (typeof fallback) {
      case "number": {
        const n = Number.parseInt(raw, 10);
        return Number.isNaN(n) ? fallback : n;
      }
      case "boolean":
        return raw === "" || raw === "true" || raw === "1";
      default:
        return raw;
    }
  }
}
