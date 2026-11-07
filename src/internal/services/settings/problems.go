package settings

import (
	"errors"
	"os"
	"path/filepath"
)

// Problems is `settings validate` with no FILE: .env's syntax and permissions,
// then authama.json against the schema. An absent authama.json is not a
// problem. The CLI and the web UI's Validate page both read it.
func Problems(envFile, settingsFile string) []string {
	return append(DotenvProblems(envFile), FileProblems(settingsFile, false)...)
}

// FileProblems checks one settings file against the schema. named is true
// when the file was given on the command line, where a missing file is an
// error rather than "every setting at its default".
func FileProblems(target string, named bool) []string {
	var problems []string
	if b, err := os.ReadFile(target); err == nil {
		if _, err := Parse(target, b); err != nil {
			var ve *ValidationError
			if errors.As(err, &ve) {
				for _, pr := range ve.Problems {
					problems = append(problems, filepath.Base(target)+": "+pr)
				}
			} else {
				problems = append(problems, err.Error())
			}
		}
	} else if named || !errors.Is(err, os.ErrNotExist) {
		problems = append(problems, err.Error())
	}
	return problems
}

// DotenvProblems checks a .env without loading it.
func DotenvProblems(path string) []string {
	var out []string
	r := LoadDotenv(path, Env{
		Lookup: func(string) (string, bool) { return "", true }, // everything "set": load nothing
		Setenv: func(string, string) error { return nil },
	})
	if r.Refused != "" {
		out = append(out, r.Refused)
	}
	out = append(out, r.Warnings...)
	return out
}
