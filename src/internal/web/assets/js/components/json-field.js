// @ts-check
import { Component } from "../core/component.js";
import { El } from "../core/el.js";

/**
 * JsonField checks a textarea that holds JSON as it is typed: an error names
 * its line and column under the field, marks the field invalid for assistive
 * technology, and stops the form from submitting until it is fixed. A Format
 * button re-indents valid JSON. The server checks again: this is a convenience,
 * never the check.
 *
 *   <textarea data-json data-json-indent="2">…</textarea>
 *
 * @extends {Component<{ indent: number }>}
 */
export class JsonField extends Component {
  static attribute = "json";
  static defaults = { indent: 2 };

  /** @type {HTMLElement} */
  #feedback = /** @type {any} */ (null);

  init() {
    const area = this.el;
    if (!(area instanceof HTMLTextAreaElement)) {
      return this;
    }
    const id = `${area.id || "json"}-feedback`;
    this.#feedback = El.create("div").cls("invalid-feedback").attr("id", id).attr("aria-live", "polite").node;
    const describedBy = [area.getAttribute("aria-describedby"), id].filter(Boolean).join(" ");
    area.setAttribute("aria-describedby", describedBy);
    const format = El.create("button")
      .cls("btn btn-sm btn-outline-secondary mt-2")
      .attr("type", "button")
      .text("Format JSON")
      .on("click", () => this.format());
    area.after(this.#feedback, format.node);
    area.addEventListener("input", () => this.check());
    this.check();
    return this;
  }

  /**
   * Checks the text and shows the result. Returns the parse error, or null.
   * @returns {string | null}
   */
  check() {
    const area = /** @type {HTMLTextAreaElement} */ (this.el);
    const problem = JsonField.problem(area.value);
    area.classList.toggle("is-invalid", problem !== null);
    area.classList.toggle("is-valid", problem === null && area.value.trim() !== "");
    area.setAttribute("aria-invalid", String(problem !== null));
    area.setCustomValidity(problem ?? "");
    this.#feedback.textContent = problem ? `Not valid JSON: ${problem}` : "";
    return problem;
  }

  /** Re-indents the text, if it is valid. */
  format() {
    const area = /** @type {HTMLTextAreaElement} */ (this.el);
    if (this.check() === null && area.value.trim() !== "") {
      area.value = JSON.stringify(JSON.parse(area.value), null, Number(this.options.indent)) + "\n";
      area.dispatchEvent(new Event("input", { bubbles: true }));
    }
    return this;
  }

  /**
   * Why text is not JSON, with the line and column where the engine says,
   * or null when it is.
   * @param {string} text
   */
  static problem(text) {
    if (text.trim() === "") {
      return null; // "required" says it is empty; this says whether it parses
    }
    try {
      JSON.parse(text);
      return null;
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      // V8 and SpiderMonkey report "position N"; JavaScriptCore does not.
      const m = /position (\d+)/.exec(message);
      if (!m) {
        return message;
      }
      const before = text.slice(0, Number(m[1]));
      const line = before.split("\n").length;
      const column = before.length - before.lastIndexOf("\n");
      return `line ${line}, column ${column}: ${message.replace(/ in JSON at position \d+.*/, "")}`;
    }
  }
}
