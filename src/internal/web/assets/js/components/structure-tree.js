// @ts-check
import { Component } from "../core/component.js";

/**
 * StructureTree adds drag and drop to the structure page's tree. Every move
 * is a select (name="move") whose options carry data-target, the category
 * or profile they move the item to; dragging an item (data-drag names its
 * select) onto a drop target (data-drop="category:<name>" or
 * "profile:<name>") picks the option for that target, exactly as choosing it
 * from the menu would. The selects stay the keyboard way to move anything,
 * and the page works without the script. It also lists the moves chosen so
 * far in [data-structure-pending], built from text nodes only.
 *
 *   <div data-structure-tree>…<li draggable="true" data-drag="move-x">…<li data-drop="profile:work">…</div>
 *
 * @extends {Component<{}>}
 */
export class StructureTree extends Component {
  static attribute = "structure-tree";

  /** @type {HTMLSelectElement | null} the select of the item being dragged */
  #dragging = null;

  init() {
    for (const item of /** @type {HTMLElement[]} */ ([...this.el.querySelectorAll("[data-drag]")])) {
      item.addEventListener("dragstart", (e) => this.#start(e, item));
      item.addEventListener("dragend", () => this.#end());
    }
    for (const target of /** @type {HTMLElement[]} */ ([...this.el.querySelectorAll("[data-drop]")])) {
      target.addEventListener("dragover", (e) => this.#over(e, target));
      target.addEventListener("dragleave", () => target.classList.remove("is-drop-target"));
      target.addEventListener("drop", (e) => this.#drop(e, target));
    }
    this.el.addEventListener("change", (e) => {
      if (e.target instanceof HTMLSelectElement && e.target.name === "move") {
        this.render();
      }
    });
    this.render();
    return this;
  }

  /**
   * The option of select that moves its item to target ("" for none).
   * @param {HTMLSelectElement} select @param {string} target
   */
  static optionFor(select, target) {
    for (const opt of [...select.options]) {
      if (opt.dataset.target === target) {
        return opt;
      }
    }
    return null;
  }

  /** @param {DragEvent} e @param {HTMLElement} item */
  #start(e, item) {
    if (e.target !== item) {
      return; // a nested draggable starts its own drag
    }
    e.stopPropagation();
    const select = document.getElementById(item.dataset.drag ?? "");
    if (!(select instanceof HTMLSelectElement)) {
      return;
    }
    this.#dragging = select;
    item.classList.add("is-dragging");
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", select.id);
    }
  }

  #end() {
    this.#dragging = null;
    for (const el of this.el.querySelectorAll(".is-dragging, .is-drop-target")) {
      el.classList.remove("is-dragging", "is-drop-target");
    }
  }

  /** @param {DragEvent} e @param {HTMLElement} target */
  #over(e, target) {
    if (!this.#dragging || !StructureTree.optionFor(this.#dragging, target.dataset.drop ?? "")) {
      return; // not a place this item can go: let an outer target answer
    }
    e.preventDefault();
    e.stopPropagation();
    target.classList.add("is-drop-target");
    if (e.dataTransfer) {
      e.dataTransfer.dropEffect = "move";
    }
  }

  /** @param {DragEvent} e @param {HTMLElement} target */
  #drop(e, target) {
    const select = this.#dragging;
    const opt = select ? StructureTree.optionFor(select, target.dataset.drop ?? "") : null;
    if (!select || !opt) {
      return;
    }
    e.preventDefault();
    e.stopPropagation();
    select.value = opt.value;
    select.dispatchEvent(new Event("change", { bubbles: true }));
    this.#end();
    select.focus();
  }

  /** Lists the moves chosen so far. */
  render() {
    const list = this.el.querySelector("[data-structure-pending]") ?? document.querySelector("[data-structure-pending]");
    if (!(list instanceof HTMLElement)) {
      return this;
    }
    /** @type {HTMLLIElement[]} */
    const rows = [];
    for (const select of /** @type {HTMLSelectElement[]} */ ([...this.el.querySelectorAll("select[name=move]")])) {
      if (!select.value) {
        continue;
      }
      const li = document.createElement("li");
      li.textContent = `${select.dataset.structureWhat ?? ""}: ${select.selectedOptions[0]?.textContent ?? ""}`;
      rows.push(li);
    }
    if (rows.length === 0) {
      const li = document.createElement("li");
      li.className = "text-secondary";
      li.textContent = "Nothing yet: choose a move from an item's menu, or drag the item.";
      rows.push(li);
    }
    list.replaceChildren(...rows);
    return this;
  }
}
