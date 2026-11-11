// @ts-check

/**
 * The job output console's reading of one line, the same rules as the
 * server's (web/console.go): its indent, the status word it starts with and
 * how that reads, the host or key it is about, and the rest. Used for the
 * lines that arrive while a job runs; the server draws the ones already there.
 */

/** @type {Record<string, string>} */
const TAGS = {
  ok: "ok", done: "ok", revoked: "ok", deleted: "ok", pinned: "ok", deployed: "ok",
  FAIL: "bad", FAILED: "bad", "error:": "bad",
  paste: "warn", "WARNING:": "warn", "warning:": "warn",
  skip: "muted", rotate: "info", move: "info",
};

const NAME = /^([A-Za-z0-9_.@/-]+):\s+(.*)$/;

/**
 * @param {string} raw
 * @returns {{indent: number, tag: string, kind: string, name: string, text: string}}
 */
export function readConsoleLine(raw) {
  const trimmed = raw.replace(/^ +/, "");
  const indent = Math.floor((raw.length - trimmed.length + 1) / 2);
  let tag = "";
  let kind = "";
  let rest = trimmed;
  const space = trimmed.indexOf(" ");
  const word = space < 0 ? trimmed : trimmed.slice(0, space);
  if (Object.hasOwn(TAGS, word)) {
    tag = word.replace(/:$/, "");
    kind = TAGS[word];
    rest = space < 0 ? "" : trimmed.slice(space + 1).trim();
  }
  if (!kind) {
    if (rest.includes("FAILED") || rest.includes("NOT ") || rest.startsWith("cannot ")) {
      kind = "bad";
    } else if (rest.includes("WARNING")) {
      kind = "warn";
    }
  }
  let name = "";
  let text = "";
  const m = NAME.exec(rest);
  if (m) {
    name = m[1];
    text = m[2];
  } else if (tag && !rest.includes(" ")) {
    name = rest;
  } else {
    text = rest;
  }
  return { indent, tag, kind, name, text };
}

/**
 * One console line as the page draws it, built from DOM nodes only.
 * @param {string} raw
 */
export function consoleLineElement(raw) {
  const l = readConsoleLine(raw);
  const row = document.createElement("div");
  row.className = `console-line lvl-${Math.min(l.indent, 2)}${l.kind ? ` is-${l.kind}` : ""}`;
  const tag = document.createElement("span");
  tag.className = "console-tag";
  tag.textContent = l.tag;
  const body = document.createElement("span");
  body.className = "console-body";
  // The colon and spaces are real text, so a copied line reads as it was
  // printed.
  if (l.name) {
    const n = document.createElement("span");
    n.className = "console-name";
    n.textContent = l.text ? `${l.name}:` : l.name;
    body.appendChild(n);
  }
  if (l.name && l.text) {
    body.appendChild(document.createTextNode(" "));
  }
  if (l.text) {
    const t = document.createElement("span");
    t.className = "console-text";
    t.textContent = l.text;
    body.appendChild(t);
  }
  row.append(tag, document.createTextNode(" "), body);
  return row;
}
