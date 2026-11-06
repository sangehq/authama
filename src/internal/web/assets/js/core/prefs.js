// @ts-check
/**
 * Prefs keeps a viewer's small choices in this browser - a CSV's columns, the
 * tour seen - under one "authama." prefix. Storage can be missing or refuse
 * (a private window, blocked site data), so every call is guarded and a
 * failure means the default, never an error.
 *
 *   Prefs.get("csv.hosts.csv", { drop: [] });
 *   Prefs.set("csv.hosts.csv", { drop: ["user"] });
 */
export class Prefs {
  /** @param {string} key */
  static #key(key) {
    return `authama.${key}`;
  }

  /**
   * The stored value for key, or fallback.
   * @template T
   * @param {string} key
   * @param {T} fallback
   * @returns {T}
   */
  static get(key, fallback) {
    try {
      const raw = window.localStorage.getItem(Prefs.#key(key));
      return raw === null ? fallback : /** @type {T} */ (JSON.parse(raw));
    } catch {
      return fallback;
    }
  }

  /** Stores value under key. @param {string} key @param {unknown} value */
  static set(key, value) {
    try {
      window.localStorage.setItem(Prefs.#key(key), JSON.stringify(value));
    } catch {
      // Nothing to do: the choice lasts this page only.
    }
  }

  /** Forgets key. @param {string} key */
  static remove(key) {
    try {
      window.localStorage.removeItem(Prefs.#key(key));
    } catch {
      // As above.
    }
  }
}
