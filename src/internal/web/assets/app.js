// @ts-check
// authama web UI: the entry module. Every page works without it; these
// components only add conveniences on top, and never build HTML from strings.
// Each is a class in js/components, marked on the page by a data attribute and
// configured by more of them (see each class).
import { App } from "./js/core/app.js";
import { Ui } from "./js/core/ui.js";
import { DataTable } from "./js/components/data-table.js";
import { AutoSubmit, ConfirmAction, CopyText, JobStream, ThemeChoice, Tip } from "./js/components/behaviours.js";
import { CommandPalette, PanelFromHash } from "./js/components/palette.js";
import { Shortcuts } from "./js/components/shortcuts.js";
import { PanelOpen, ShowWhen } from "./js/components/forms.js";
import { Tour } from "./js/components/guide.js";
import { Inline, SidePanel } from "./js/components/side-panel.js";
import { CsvExport } from "./js/components/csv-export.js";
import { JsonField } from "./js/components/json-field.js";
import { ExportPage } from "./js/components/export-page.js";
import { LiveBuild, RestartServer, Restarted } from "./js/components/restart.js";

App.create()
  .use(SidePanel)
  .use(Inline)
  .use(CsvExport)
  .use(JsonField)
  .use(ExportPage)
  .use(RestartServer)
  .use(Restarted)
  .use(LiveBuild)
  .use(ThemeChoice)
  .use(Tip)
  .use(DataTable)
  .use(ConfirmAction)
  .use(AutoSubmit)
  .use(JobStream)
  .use(ShowWhen)
  .use(CopyText)
  .use(PanelOpen)
  .use(PanelFromHash)
  .use(CommandPalette)
  .use(Shortcuts)
  .use(Tour)
  .start();

// The tooltip rules cover controls a script adds later too, so they start
// with the page rather than with the first marked control.
Ui.startTooltips();
