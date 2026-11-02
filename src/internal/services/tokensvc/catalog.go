package tokensvc

import (
	"fmt"
	"os"

	"github.com/simtabi/authama/src/internal/core/tokens"
)

// Catalog override variables. auth-token-manager read its catalog from
// $AUTH_TOKEN_MANAGER_CATALOG when set; authama reads $SSH_MANAGER_TOKENS_CATALOG
// first and still honours the old name.
const (
	CatalogEnv       = "SSH_MANAGER_TOKENS_CATALOG"
	legacyCatalogEnv = "AUTH_TOKEN_MANAGER_CATALOG"
)

// CatalogSource is the catalog a variable selected, if any.
type CatalogSource struct {
	// Path is the file in use; "" means the embedded catalog.
	Path string
	// Var is the variable that named it.
	Var string
	// Legacy is set when Var is auth-token-manager's name for it.
	Legacy bool
	// Warning is set when a variable named a file that is not there; the
	// embedded catalog is used instead, as auth-token-manager did.
	Warning string
	// Err is set when the file is there but is not a usable catalog. The
	// embedded catalog stays in use; the token commands refuse to run.
	Err error
}

// useCatalog installs the catalog the environment names, or restores the
// embedded one when it names none.
func useCatalog(e Env) CatalogSource {
	var src CatalogSource
	for _, v := range []string{CatalogEnv, legacyCatalogEnv} {
		if p := e.getenv(v); p != "" {
			src = CatalogSource{Path: e.abs(p), Var: v, Legacy: v == legacyCatalogEnv}
			break
		}
	}
	if src.Var == "" {
		_ = tokens.UseCatalog("")
		return src
	}
	if !isFile(src.Path) {
		_ = tokens.UseCatalog("")
		src.Warning = fmt.Sprintf("$%s names %s, which is not a file; using the built-in catalog", src.Var, src.Path)
		src.Path = ""
		return src
	}
	b, err := os.ReadFile(src.Path)
	if err == nil {
		err = tokens.UseCatalog(string(b))
	}
	if err != nil {
		_ = tokens.UseCatalog("")
		src.Err = fmt.Errorf("$%s (%s): %w", src.Var, src.Path, err)
	}
	return src
}
