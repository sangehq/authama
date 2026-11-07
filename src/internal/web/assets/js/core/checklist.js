// @ts-check

/**
 * A choice of values from a list, where `null` means every value: the model
 * behind the CSV dialog's lists and the Export page's facets. Each picker
 * draws its own markup; this decides what is ticked, what a tick or an "all"
 * box makes of the choice, and how the count badge reads.
 *
 * A choice that names every value is stored as `null`, so a list that gains
 * a value later still has it chosen.
 */
export class Checklist {
  /**
   * @param {string[]} values every value, in order
   * @param {string[] | null | undefined} chosen the values chosen; null or undefined for all
   */
  constructor(values, chosen) {
    this.values = values;
    this.chosen = chosen ?? null;
  }

  /** Whether value is chosen. @param {string} value */
  on(value) {
    return this.chosen === null || this.chosen.includes(value);
  }

  /** How many values are chosen. */
  get ticked() {
    return this.values.filter((v) => this.on(v)).length;
  }

  /** The "all" box: checked when every value is, mixed when some are. */
  get all() {
    const n = this.ticked;
    return { checked: n === this.values.length, indeterminate: n > 0 && n < this.values.length };
  }

  /**
   * The choice after value is ticked or unticked, kept in list order.
   * @param {string} value @param {boolean} checked
   */
  toggle(value, checked) {
    const rest = this.values.filter((v) => this.on(v) && v !== value);
    return this.normal(checked ? [...rest, value] : rest);
  }

  /**
   * The choice after the "all" box, applied to shown (every value when
   * omitted, or only those a search leaves visible): ticking it adds them,
   * unticking it drops them, and the hidden values keep their state.
   * @param {boolean} checked @param {string[]} [shown]
   */
  setAll(checked, shown = this.values) {
    const rest = this.values.filter((v) => this.on(v) && !shown.includes(v));
    return this.normal(checked ? [...rest, ...shown] : rest);
  }

  /**
   * next as a choice: null when it names every value.
   * @param {string[]} next @returns {string[] | null}
   */
  normal(next) {
    return this.values.every((v) => next.includes(v)) ? null : next;
  }

  /** The count badge: "n" when all are chosen, else "ticked/n", coloured by state. */
  get badge() {
    const n = this.ticked;
    const total = this.values.length;
    return {
      text: n === total ? String(total) : `${n}/${total}`,
      state: n === 0 ? "badge-bad" : n === total ? "badge-muted" : "badge-info",
    };
  }
}
