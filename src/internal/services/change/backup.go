package change

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/homeperms"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// SettingsSet is `authama settings set <key> <value>`: one setting written
// into authama.json, every other kept. On apply the running process takes the
// new value at once, so a page that needed it (a job's backup check) passes
// without a restart; a variable already set in the environment still wins,
// as it does at start-up.
func SettingsSet(key, value string) Change {
	return Change{
		Command: "settings set", Args: []string{key, value}, NoSnapshot: true,
		Effects: func(paths.Paths) ([]string, error) {
			spec, ok := settings.Find(key)
			if !ok {
				return nil, fmt.Errorf("%s: unknown setting", key)
			}
			return []string{fmt.Sprintf("this server uses the new %s at once, unless %s is already set in its environment", key, spec.Env)}, nil
		},
		// The running process takes the value on apply only; Run, which a
		// preview runs against a copy, writes the file and nothing else.
		OnApply: func(p paths.Paths, _ io.Writer) error {
			old, _ := settings.FileValue(p.SettingsFile(), key)
			useNow(key, value, old)
			return nil
		},
		Run: func(p paths.Paths, out io.Writer) error {
			if err := settings.Set(p.SettingsFile(), key, value); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "set %s in %s\n", key, p.SettingsFile())
			return nil
		},
	}
}

// useNow gives the running process a setting just written to authama.json:
// it replaces the variable when it is unset or holds the file's old value
// (so it came from the file), and leaves one set some other way - the shell,
// .env - which wins, as at start-up. value "" unsets it. Only on apply: a
// preview's copy must not change what this process uses.
func useNow(key, value, old string) {
	spec, ok := settings.Find(key)
	if !ok {
		return
	}
	if cur, set := os.LookupEnv(spec.Env); set && cur != old {
		return
	}
	if value == "" {
		_ = os.Unsetenv(spec.Env)
		return
	}
	_ = os.Setenv(spec.Env, value)
}

// SettingsUnset is `authama settings unset <key>`: the setting is taken out of
// authama.json and goes back to its default; this server follows at once,
// unless the environment sets it some other way.
func SettingsUnset(key string) Change {
	return Change{
		Command: "settings unset", Args: []string{key}, NoSnapshot: true,
		Effects: func(paths.Paths) ([]string, error) {
			if _, ok := settings.Find(key); !ok {
				return nil, fmt.Errorf("%s: unknown setting", key)
			}
			return []string{"this server goes back to the default " + key + " at once, unless its environment sets it"}, nil
		},
		OnApply: func(p paths.Paths, _ io.Writer) error {
			old, _ := settings.FileValue(p.SettingsFile(), key)
			useNow(key, "", old)
			return nil
		},
		Run: func(p paths.Paths, out io.Writer) error {
			if err := settings.Unset(p.SettingsFile(), key); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "%s goes back to its default\n", key)
			return nil
		},
	}
}

// BackupKeyFile is where `settings backup-key` keeps the age identity it
// makes, owner-only: where paths.age_identity_file (AUTHAMA_AGE_IDENTITY_FILE)
// already points, when it does, so a restore finds the key it expects;
// otherwise in the config home.
func BackupKeyFile(p paths.Paths) string {
	if f := envnames.Get(nil, envnames.AgeIdentityFile); f != "" {
		return f
	}
	return filepath.Join(p.ConfigDir, "backup", "age-identity.txt")
}

// BackupKey is `authama settings backup-key`: makes an age key for encrypted
// backups with age-keygen, keeps its identity owner-only in the config home,
// and sets backup.age_recipient and paths.age_identity_file to it. The key is
// made only on apply, never in a preview, so no page ever shows it.
func BackupKey() Change {
	return Change{
		Command: "settings backup-key", NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			if _, err := exec.LookPath("age-keygen"); err != nil {
				return nil, errors.New("age-keygen is not installed: install age (brew install age, apt install age), then try again")
			}
			if fs.Exists(BackupKeyFile(p)) {
				return []string{
					"uses the age key already in " + BackupKeyFile(p) + ": no new key is made",
					"sets backup.age_recipient to its recipient and paths.age_identity_file to that file, in authama.json",
					"this server uses them at once; encrypted backups are written to the recipient from then on",
				}, nil
			}
			return []string{
				"makes a new age key with age-keygen, and keeps its identity, owner-only, in " + BackupKeyFile(p),
				"sets backup.age_recipient to its recipient and paths.age_identity_file to that file, in authama.json",
				"this server uses them at once; encrypted backups are written to the recipient from then on",
				"keep a copy of the identity file somewhere safe (a password manager): without it no backup can be decrypted",
			}, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			file := BackupKeyFile(p)
			// A key already there is used, not replaced: a second press, or a
			// key made before, sets the settings up from it.
			made := !fs.Exists(file)
			if made {
				if err := fs.EnsureDir(filepath.Dir(file), homeperms.DirMode); err != nil {
					return err
				}
				if err := exec.Command("age-keygen", "-o", file).Run(); err != nil {
					return fmt.Errorf("age-keygen: %w", err)
				}
			}
			if err := os.Chmod(file, homeperms.FileMode); err != nil {
				return err
			}
			pub, err := exec.Command("age-keygen", "-y", file).Output()
			if err != nil {
				return fmt.Errorf("age-keygen -y: %w", err)
			}
			recipient := strings.TrimSpace(string(pub))
			for _, kv := range [][2]string{{"backup.age_recipient", recipient}, {"paths.age_identity_file", file}} {
				old, _ := settings.FileValue(p.SettingsFile(), kv[0])
				if err := settings.Set(p.SettingsFile(), kv[0], kv[1]); err != nil {
					return err
				}
				useNow(kv[0], kv[1], old)
			}
			verb := "made a backup key"
			if !made {
				verb = "set up the backup key already there"
			}
			_, _ = fmt.Fprintf(out, "%s: recipient %s, identity %s\nkeep a copy of %s somewhere safe\n", verb, recipient, file, file)
			return nil
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}
