// authama web UI: the whole of its own JavaScript. Every page works without it;
// this only adds conveniences on top, and never builds HTML from strings.
(function () {
  "use strict";

  // The history filter's source list applies as soon as it changes.
  var source = document.getElementById("f-source");
  if (source && source.form) {
    source.addEventListener("change", function () { source.form.submit(); });
  }
})();

// A job's page streams its output. Lines are added as text nodes, never as
// HTML; when the job changes state the page reloads to show the question,
// the result and the right buttons.
(function () {
  "use strict";
  var log = document.getElementById("job-log");
  var box = document.getElementById("job-state");
  if (!log || !box || !window.EventSource) {
    return;
  }
  var state = box.getAttribute("data-state");
  if (state !== "running" && state !== "waiting") {
    return;
  }
  var seen = log.textContent.split("\n").length - 1;
  var count = 0;
  var events = new EventSource(log.getAttribute("data-events"));
  events.addEventListener("line", function (e) {
    count += 1;
    if (count <= seen) {
      return; // already on the page
    }
    log.appendChild(document.createTextNode(e.data + "\n"));
  });
  events.addEventListener("state", function (e) {
    if (e.data !== state) {
      events.close();
      window.location.reload();
    }
  });
})();

// Tables marked data-table get Tabler's list features: a search box, a page
// size, sortable column headers and pagination. Every row is in the page
// already, so a table without this script is the whole list, unpaginated.
// Elements are built with DOM calls, never from HTML strings.
(function () {
  "use strict";

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) { e.className = cls; }
    if (text !== undefined) { e.textContent = text; }
    return e;
  }

  function cellValue(row, i) {
    var c = row.cells[i];
    if (!c) { return ""; }
    return (c.getAttribute("data-sort") || c.textContent || "").trim();
  }

  function compare(a, b) {
    var na = parseFloat(a), nb = parseFloat(b);
    if (!isNaN(na) && !isNaN(nb) && String(na) === a.replace(/[^0-9.\-].*$/, "") && String(nb) === b.replace(/[^0-9.\-].*$/, "")) {
      return na - nb;
    }
    return a.localeCompare(b, undefined, { numeric: true, sensitivity: "base" });
  }

  function enhance(table, n) {
    var body = table.tBodies[0];
    if (!body) { return; }
    var rows = Array.prototype.slice.call(body.rows);
    var wrap = table.closest(".table-responsive") || table;
    var card = table.closest(".card");
    var label = table.getAttribute("data-table") || "entries";
    var pageSize = parseInt(table.getAttribute("data-page-size") || "10", 10);
    var page = 1, query = "", sortCol = -1, sortDir = 1;

    // Toolbar: page size on the left, search on the right, as Tabler's tables.
    var bar = el("div", "card-body border-bottom py-3 d-print-none");
    var flex = el("div", "d-flex flex-wrap gap-2 align-items-center");
    var showWrap = el("div", "text-secondary d-flex align-items-center gap-2");
    var sizeId = "dt-size-" + n, searchId = "dt-search-" + n;
    var sizeLabel = el("label", "mb-0", "Show");
    sizeLabel.htmlFor = sizeId;
    var size = el("select", "form-select form-select-sm w-auto");
    size.id = sizeId;
    [10, 25, 50, 100, 0].forEach(function (v) {
      var o = el("option", "", v === 0 ? "all" : String(v));
      o.value = String(v);
      if (v === pageSize) { o.selected = true; }
      size.appendChild(o);
    });
    showWrap.appendChild(sizeLabel);
    showWrap.appendChild(size);
    showWrap.appendChild(el("span", "", label));
    var searchWrap = el("div", "ms-auto text-secondary d-flex align-items-center gap-2");
    var searchLabel = el("label", "mb-0", "Search:");
    searchLabel.htmlFor = searchId;
    var search = el("input", "form-control form-control-sm");
    search.type = "search";
    search.id = searchId;
    search.setAttribute("aria-controls", table.id || "");
    searchWrap.appendChild(searchLabel);
    searchWrap.appendChild(search);
    flex.appendChild(showWrap);
    flex.appendChild(searchWrap);
    bar.appendChild(flex);
    wrap.parentNode.insertBefore(bar, wrap);

    // Footer: what is shown, and the pages.
    var foot = el("div", "card-footer d-flex flex-wrap align-items-center gap-2 d-print-none");
    var info = el("p", "m-0 text-secondary");
    info.setAttribute("aria-live", "polite");
    var nav = el("nav", "ms-auto");
    nav.setAttribute("aria-label", label + " pages");
    var pager = el("ul", "pagination m-0");
    nav.appendChild(pager);
    foot.appendChild(info);
    foot.appendChild(nav);
    if (wrap.nextSibling) { wrap.parentNode.insertBefore(foot, wrap.nextSibling); } else { wrap.parentNode.appendChild(foot); }

    // Sortable headers: a button in each header cell, aria-sort on the cell.
    var heads = table.tHead ? Array.prototype.slice.call(table.tHead.rows[0].cells) : [];
    heads.forEach(function (th, i) {
      if (th.hasAttribute("data-nosort") || !th.textContent.trim()) { return; }
      var b = el("button", "table-sort", th.textContent.trim());
      b.type = "button";
      th.textContent = "";
      th.appendChild(b);
      b.addEventListener("click", function () {
        sortDir = sortCol === i ? -sortDir : 1;
        sortCol = i;
        heads.forEach(function (h) { h.removeAttribute("aria-sort"); var x = h.querySelector(".table-sort"); if (x) { x.classList.remove("asc", "desc"); } });
        th.setAttribute("aria-sort", sortDir > 0 ? "ascending" : "descending");
        b.classList.add(sortDir > 0 ? "asc" : "desc");
        rows.sort(function (r1, r2) { return sortDir * compare(cellValue(r1, i), cellValue(r2, i)); });
        rows.forEach(function (r) { body.appendChild(r); });
        page = 1;
        draw();
      });
    });

    function matching() {
      if (!query) { return rows; }
      return rows.filter(function (r) { return r.textContent.toLowerCase().indexOf(query) !== -1; });
    }

    function pageItem(text, target, opts) {
      var li = el("li", "page-item" + (opts.active ? " active" : "") + (opts.disabled ? " disabled" : ""));
      var a = el("button", "page-link", text);
      a.type = "button";
      if (opts.label) { a.setAttribute("aria-label", opts.label); }
      if (opts.active) { a.setAttribute("aria-current", "page"); }
      if (opts.disabled) { a.disabled = true; } else { a.addEventListener("click", function () { page = target; draw(); }); }
      li.appendChild(a);
      pager.appendChild(li);
    }

    function draw() {
      var shown = matching();
      var per = pageSize > 0 ? pageSize : shown.length || 1;
      var pages = Math.max(1, Math.ceil(shown.length / per));
      if (page > pages) { page = pages; }
      var from = (page - 1) * per, to = Math.min(shown.length, from + per);
      rows.forEach(function (r) { r.hidden = true; });
      shown.slice(from, to).forEach(function (r) { r.hidden = false; });
      info.textContent = shown.length === 0 ? "No matching " + label :
        "Showing " + (from + 1) + " to " + to + " of " + shown.length + " " + label + (query ? " (filtered from " + rows.length + ")" : "");
      while (pager.firstChild) { pager.removeChild(pager.firstChild); }
      foot.classList.toggle("d-none", rows.length <= 10);
      if (pages > 1) {
        pageItem("‹", page - 1, { disabled: page === 1, label: "Previous page" });
        for (var i = 1; i <= pages; i++) {
          if (pages > 7 && i > 2 && i < pages - 1 && Math.abs(i - page) > 1) {
            if (i === 3 || i === pages - 2) { pageItem("…", i, { disabled: true }); }
            continue;
          }
          pageItem(String(i), i, { active: i === page, label: "Page " + i });
        }
        pageItem("›", page + 1, { disabled: page === pages, label: "Next page" });
      }
    }

    size.addEventListener("change", function () { pageSize = parseInt(size.value, 10); page = 1; draw(); });
    search.addEventListener("input", function () { query = search.value.trim().toLowerCase(); page = 1; draw(); });
    if (!card) { bar.classList.remove("card-body"); }
    // A short list keeps its sortable headers but needs no search or pages.
    // (d-none, not the hidden attribute: Tabler's d-flex outranks [hidden].)
    if (rows.length <= 10) { bar.classList.add("d-none"); }
    draw();
  }

  Array.prototype.slice.call(document.querySelectorAll("table[data-table]")).forEach(enhance);
})();

// Destructive actions confirm in a modal first: a button with
// data-confirm="<modal id>" opens it, and the modal's own button submits the
// form it belongs to. Without the script the button submits straight to the
// preview, which is a second confirmation of its own.
(function () {
  "use strict";
  var Modal = window.tabler && window.tabler.Modal;
  if (!Modal) { return; }
  Array.prototype.slice.call(document.querySelectorAll("[data-confirm]")).forEach(function (b) {
    var modal = document.getElementById(b.getAttribute("data-confirm"));
    if (!modal) { return; }
    b.addEventListener("click", function (e) {
      e.preventDefault();
      Modal.getOrCreateInstance(modal).show();
    });
  });
})();
