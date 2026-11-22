package web

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/services/bundler"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/setupxfer"
	"github.com/sangehq/authama/src/internal/services/transfer"
	"github.com/sangehq/authama/src/internal/util/agepass"
	"github.com/sangehq/authama/src/internal/util/encode"
	"github.com/sangehq/authama/src/internal/util/log"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/version"
)

// transferPage is /transfer, the Export / Import hub the header's button
// opens. Export: tick the parts, preview the files, download them as one zip
// (GET, no secret in any of them: `export pack`). The encrypted full export
// (private keys and token values) is a POST behind the owner PIN that sends
// the age file straight to the download; nothing of it is shown. Import: a
// file is uploaded, told apart (setup document, export pack, encrypted full
// export, or something to bring in elsewhere), summarised by name, and then
// goes through the change preview like every other change. An encrypted
// export and its passphrase wait in the secret stash, never in the page.
type transferPage struct{ s *Server }

// TransferPart is one part the export offers.
type TransferPart struct {
	transfer.Part
	Checked bool
}

// TransferFile is one file of the export preview.
type TransferFile struct {
	Name  string
	Size  int
	Head  string // its first lines
	Lines int    // how many lines it has
	More  bool   // Head is cut short
}

// TransferImport is an uploaded file, told apart and summarised by name.
type TransferImport struct {
	File, Kind, Explain string
	Doc                 *setupxfer.Doc
	KeyFiles, Tokens    []setupxfer.Item // full export: names and states only
	Body                string           // a setup document, carried to the preview
	FileRef, PassRef    string           // a full export, held in the stash
	Policy              string
}

// TransferView is the /transfer page.
type TransferView struct {
	Parts         []TransferPart
	Groups        []string
	Group         string
	IncludeGit    bool
	Preview       []TransferFile
	Previewed     bool
	Download      string
	Problem       string
	ImportProblem string
	Import        *TransferImport
	MinPass       int
	MaxUpload     string
	PackCmd       string
	EncryptCmd    string
	ImportCmd     string
}

// transferPreviewLines is how much of each file the preview shows.
const transferPreviewLines = 12

func (tp transferPage) view(q url.Values) (TransferView, transfer.Options) {
	o := transfer.Options{Include: q["part"], TermiusGroups: encode.TermiusGroupsProfile, IncludeGit: q.Get("include_git") == "1",
		Version: version.Version, Now: tp.s.now()}
	// Only a scheme of the list goes on, as the list's own string: nothing of
	// the query is echoed into the zip.
	problem := ""
	if raw := q.Get("termius_groups"); raw != "" {
		if scheme, ok := encode.TermiusGroupsScheme(raw); ok {
			o.TermiusGroups = scheme
		} else {
			problem = "No Termius group scheme " + strconv.Quote(raw) + ": choose one of " + strings.Join(encode.TermiusGroupSchemes, ", ") + "."
		}
	}
	chosen := map[string]bool{}
	want := o.Include
	if len(want) == 0 {
		want = transfer.Default
	}
	for _, id := range want {
		chosen[id] = true
	}
	v := TransferView{Groups: encode.TermiusGroupSchemes, Group: o.TermiusGroups, IncludeGit: o.IncludeGit,
		MinPass: agepass.MinPassphrase, MaxUpload: fmt.Sprintf("%d KiB", maxTransferUpload>>10),
		PackCmd:    brand.Command + " export pack --include " + strings.Join(want, ",") + " --termius-groups " + o.TermiusGroups,
		EncryptCmd: brand.Command + " setup export --encrypt --out setup.age",
		ImportCmd:  brand.Command + " setup import FILE --dry-run", Problem: problem}
	for _, p := range transfer.Parts {
		v.Parts = append(v.Parts, TransferPart{Part: p, Checked: chosen[p.ID]})
	}
	dq := url.Values{}
	for _, id := range want {
		dq.Add("part", id)
	}
	dq.Set("termius_groups", o.TermiusGroups)
	if o.IncludeGit {
		dq.Set("include_git", "1")
	}
	v.Download = "/transfer/pack.zip?" + dq.Encode()
	return v, o
}

func (tp transferPage) handle(w http.ResponseWriter, r *http.Request) {
	v, o := tp.view(r.URL.Query())
	if r.URL.Query().Get("preview") == "1" && v.Problem == "" {
		files, err := transfer.Build(tp.s.opts.Engine.Home(), o)
		if err != nil {
			v.Problem = err.Error()
		} else {
			v.Previewed = true
			for _, f := range files {
				v.Preview = append(v.Preview, previewFile(f))
			}
		}
	}
	tp.render(w, r, http.StatusOK, v)
}

func (tp transferPage) render(w http.ResponseWriter, r *http.Request, status int, v TransferView) {
	tp.s.render(w, status, "transfer", tp.s.newView(r, "transfer", "Export / Import", v))
}

// previewFile is a file's first lines. A zip member that is not text shows
// its size alone.
func previewFile(f transfer.File) TransferFile {
	tf := TransferFile{Name: f.Name, Size: len(f.Body)}
	if !utf8.Valid(f.Body) {
		return tf
	}
	lines := strings.Split(strings.TrimRight(string(f.Body), "\n"), "\n")
	tf.Lines = len(lines)
	if len(lines) > transferPreviewLines {
		lines, tf.More = lines[:transferPreviewLines], true
	}
	tf.Head = strings.Join(lines, "\n")
	return tf
}

// handlePack downloads the pack. It holds no secret and writes nothing.
func (tp transferPage) handlePack(w http.ResponseWriter, r *http.Request) {
	v, o := tp.view(r.URL.Query())
	if v.Problem != "" {
		tp.s.renderError(w, r, http.StatusBadRequest, "The export could not be made", v.Problem)
		return
	}
	files, err := transfer.Build(tp.s.opts.Engine.Home(), o)
	if err != nil {
		tp.s.renderError(w, r, http.StatusBadRequest, "The export could not be made", err.Error())
		return
	}
	data, err := transfer.Zip(files, o.Now)
	if err != nil {
		tp.s.renderError(w, r, http.StatusInternalServerError, "The export could not be made", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+brand.Command+`-export-`+o.Now.Format("20060102-150405")+`.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// handleEncrypted makes the encrypted full export and sends it as the
// download. It needs the owner PIN (within the step-up window) and a
// passphrase typed twice. The archive is written to a private folder in the
// state home, read back and removed; nothing of it is rendered.
func (tp transferPage) handleEncrypted(w http.ResponseWriter, r *http.Request) {
	v, _ := tp.view(nil)
	if !ownerGate(tp).stepUp(w, r) {
		return
	}
	pass, again := r.PostForm.Get("passphrase"), r.PostForm.Get("passphrase_confirm")
	r.PostForm.Del("passphrase")
	r.PostForm.Del("passphrase_confirm")
	switch {
	case len(pass) < agepass.MinPassphrase:
		v.Problem = fmt.Sprintf("The passphrase needs at least %d characters.", agepass.MinPassphrase)
	case pass != again:
		v.Problem = "The two passphrases differ."
	}
	if v.Problem != "" {
		tp.render(w, r, http.StatusBadRequest, v)
		return
	}
	p := tp.s.opts.Engine.Home()
	data, res, err := writeFullExport(p, []byte(pass), tp.s.now())
	if err != nil {
		v.Problem = "The encrypted export could not be made: " + err.Error()
		tp.render(w, r, http.StatusInternalServerError, v)
		return
	}
	log.Audit(p.AuditLog(), "setup.export", log.Field{Key: "encrypted", Value: true}, log.Field{Key: "source", Value: "web"},
		log.Field{Key: "members", Value: len(res.Contents)})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+brand.Command+`-setup-`+tp.s.now().Format("20060102-150405")+`.age"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// writeFullExport writes the full export to a private temporary folder in
// the state home and reads it back.
func writeFullExport(p paths.Paths, pass []byte, now time.Time) ([]byte, setupxfer.FullResult, error) {
	if err := os.MkdirAll(p.StateDir(), 0o700); err != nil {
		return nil, setupxfer.FullResult{}, err
	}
	dir, err := os.MkdirTemp(p.StateDir(), "export-")
	if err != nil {
		return nil, setupxfer.FullResult{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	res, err := setupxfer.WriteFull(p, filepath.Join(dir, "setup.age"), pass, version.Version, now, os.LookupEnv)
	if err != nil {
		return nil, res, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "setup.age"))
	return data, res, err
}

// maxTransferUpload is the largest file the import takes: the server reads
// no larger body. A larger one is imported at a terminal.
const maxTransferUpload = MaxBodyBytes - 64<<10

// handleImport tells an uploaded file apart and summarises it, then hands it
// to the change preview: a setup document (or a pack's) as the setup-import
// action, an encrypted full export as transfer-import-full with the file and
// its passphrase in the stash.
func (tp transferPage) handleImport(w http.ResponseWriter, r *http.Request) {
	v, _ := tp.view(nil)
	fail := func(status int, msg string) {
		v.ImportProblem = msg
		tp.render(w, r, status, v)
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		fail(http.StatusBadRequest, "Choose a file to import.")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxTransferUpload+1))
	if err != nil || len(data) > maxTransferUpload {
		fail(http.StatusRequestEntityTooLarge, "The file is larger than "+v.MaxUpload+"; import it at a terminal: "+v.ImportCmd)
		return
	}
	pass := r.PostForm.Get("passphrase")
	r.PostForm.Del("passphrase")
	policy, err := setupxfer.ParsePolicy(r.PostForm.Get("on_conflict"))
	if err != nil {
		fail(http.StatusBadRequest, err.Error())
		return
	}
	imp := &TransferImport{File: filepath.Base(hdr.Filename), Policy: string(policy)}
	k := transfer.Detect(data)
	imp.Kind = string(k)
	p := tp.s.opts.Engine.Home()
	switch k {
	case transfer.KindPack, transfer.KindSetup:
		if k == transfer.KindPack {
			if data, err = transfer.SetupFromPack(data); err != nil {
				fail(http.StatusBadRequest, err.Error())
				return
			}
		}
		d, err := setupxfer.Parse(data)
		if err != nil {
			fail(http.StatusBadRequest, err.Error())
			return
		}
		if _, errs := setupImportChange(url.Values{"body": {string(data)}, "on_conflict": {string(policy)}}, nil); len(errs) > 0 {
			fail(http.StatusBadRequest, errors.Join(errs...).Error())
			return
		}
		imp.Doc, imp.Body = &d, string(data)
	case transfer.KindFull:
		if pass == "" {
			fail(http.StatusBadRequest, "This is an encrypted full export: give its passphrase with the file.")
			return
		}
		full, err := openFullExport(p, data, pass, tp.s.now())
		if err != nil {
			fail(http.StatusBadRequest, err.Error())
			return
		}
		if _, err := change.SetupImportFull(full, policy, "", false); err != nil {
			fail(http.StatusBadRequest, err.Error()+" Import it at a terminal with `"+brand.Command+" setup import FILE --allow-risky-options` if you set them yourself.")
			return
		}
		keys, err := full.KeyItems(policy)
		if err != nil {
			fail(http.StatusBadRequest, err.Error())
			return
		}
		env, err := setupxfer.PlanEnv(p.EnvFile(), full.Env, policy)
		if err != nil {
			fail(http.StatusBadRequest, err.Error())
			return
		}
		sess := sessionFrom(r.Context())
		d := full.Doc
		imp.Doc, imp.KeyFiles, imp.Tokens = &d, keys.Items, env.Items
		imp.FileRef = tp.s.secrets.put(sess, base64.StdEncoding.EncodeToString(data), tp.s.now())
		imp.PassRef = tp.s.secrets.put(sess, pass, tp.s.now())
	default:
		imp.Explain = transfer.Explain(k, brand.Command)
	}
	v.Import = imp
	tp.render(w, r, http.StatusOK, v)
}

// openFullExport decrypts an uploaded full export. OpenFull reads a path, so
// the file is written to a private temporary folder in the state home and
// removed once it is open; risky options are reported by SetupImportFull.
func openFullExport(p paths.Paths, data []byte, pass string, now time.Time) (*setupxfer.Full, error) {
	if err := os.MkdirAll(p.StateDir(), 0o700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(p.StateDir(), "import-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "setup.age")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return setupxfer.OpenFull(p, path, []byte(pass), bundler.ReplacedDir(p, now), true)
}

// transferImportFullChange is the transfer-import-full action: the full
// export and passphrase the upload left in the stash, opened again and merged
// under on_conflict. Risky SSH options are refused, as for a pasted document.
func transferImportFullChange(f url.Values, h home) (change.Change, error) {
	now := time.Now()
	policy, err := setupxfer.ParsePolicy(f.Get("on_conflict"))
	if err != nil {
		return change.Change{}, badForm("%s", err.Error())
	}
	enc, pass := h.stashed(f, "file"), h.stashed(f, "pass")
	if enc == "" || pass == "" {
		return change.Change{}, errors.New("the uploaded export is no longer held (it waits 15 minutes, and is used once); upload it again")
	}
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil || !bytes.HasPrefix(data, []byte("age-encryption.org/")) {
		return change.Change{}, errors.New("the uploaded export could not be read; upload it again")
	}
	full, err := openFullExport(h.p, data, pass, now)
	if err != nil {
		return change.Change{}, err
	}
	return change.SetupImportFull(full, policy, bundler.ReplacedDir(h.p, now), false)
}
