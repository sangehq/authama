// @ts-check
// authama web UI: the entry module. Every page works without it; these
// components only add conveniences on top, and never build HTML from strings.
// Each is a class in js/components, marked on the page by a data attribute and
// configured by more of them (see each class).
import { App } from "./js/core/app.js";
import { DataTable } from "./js/components/data-table.js";
import { AutoSubmit, ConfirmAction, CopyText, JobStream } from "./js/components/behaviours.js";
import { CommandPalette, PanelFromHash } from "./js/components/palette.js";
import { PanelOpen, ShowWhen } from "./js/components/forms.js";
import { Tour } from "./js/components/guide.js";
import { Inline, SidePanel } from "./js/components/side-panel.js";
import { CsvExport } from "./js/components/csv-export.js";
import { JsonField } from "./js/components/json-field.js";

App.create()
  .use(SidePanel)
  .use(Inline)
  .use(CsvExport)
  .use(JsonField)
  .use(DataTable)
  .use(ConfirmAction)
  .use(AutoSubmit)
  .use(JobStream)
  .use(ShowWhen)
  .use(CopyText)
  .use(PanelOpen)
  .use(PanelFromHash)
  .use(CommandPalette)
  .use(Tour)
  .start();
