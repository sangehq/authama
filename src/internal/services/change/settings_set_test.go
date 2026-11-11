package change

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/settings"
)

// A saved setting reaches this process at once when the variable is unset or
// came from authama.json, and never over one the environment set otherwise.
func TestASavedSettingIsUsedAtOnceUnlessTheEnvironmentSetsIt(t *testing.T) {
	t.Setenv("AUTHAMA_GIT_MODE", "")
	_ = os.Unsetenv("AUTHAMA_GIT_MODE")
	useNow("git.mode", "ssh", "")
	if os.Getenv("AUTHAMA_GIT_MODE") != "ssh" {
		t.Fatal("an unset variable did not take the saved value")
	}
	useNow("git.mode", "https", "ssh")
	if os.Getenv("AUTHAMA_GIT_MODE") != "https" {
		t.Fatal("a value that came from the file was not replaced")
	}
	useNow("git.mode", "profile", "ssh")
	if os.Getenv("AUTHAMA_GIT_MODE") != "https" {
		t.Fatal("a value set some other way was overwritten")
	}
	useNow("git.mode", "", "https")
	if _, set := os.LookupEnv("AUTHAMA_GIT_MODE"); set {
		t.Fatal("going back to the default left the variable set")
	}
}

// backup-key makes an age key, keeps its identity owner-only in the config
// home, and points backup.age_recipient and paths.age_identity_file at it.
func TestBackupKeySetsUpEncryptedBackups(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in age-keygen is a shell script")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = -o ]; then printf 'AGE-SECRET-KEY-TEST\\n' > \"$2\"; exit 0; fi\nprintf 'age1standin\\n'\n"
	if err := os.WriteFile(filepath.Join(bin, "age-keygen"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("AUTHAMA_AGE_RECIPIENT", "")
	t.Setenv("AUTHAMA_AGE_IDENTITY_FILE", "")
	_ = os.Unsetenv("AUTHAMA_AGE_RECIPIENT")
	_ = os.Unsetenv("AUTHAMA_AGE_IDENTITY_FILE")
	p := home(t)
	ch := BackupKey()
	if _, err := ch.Effects(p); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := ch.OnApply(p, &out); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(BackupKeyFile(p))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("identity file: %v, mode %v", err, fi)
	}
	if v, _ := settings.FileValue(p.SettingsFile(), "backup.age_recipient"); v != "age1standin" {
		t.Errorf("backup.age_recipient = %q", v)
	}
	if os.Getenv("AUTHAMA_AGE_RECIPIENT") != "age1standin" {
		t.Error("this process does not use the new recipient")
	}
	if strings.Contains(out.String(), "AGE-SECRET-KEY") {
		t.Error("the output shows the private key")
	}
	if _, err := ch.Effects(p); err == nil {
		t.Error("a second backup-key would replace the identity already there")
	}
}

// A key is made where paths.age_identity_file already points, so a restore
// finds it; with nothing configured, in the config home.
func TestBackupKeyGoesWhereTheIdentityIsConfigured(t *testing.T) {
	p := home(t)
	t.Setenv("AUTHAMA_AGE_IDENTITY_FILE", "")
	if got, want := BackupKeyFile(p), filepath.Join(p.ConfigDir, "backup", "age-identity.txt"); got != want {
		t.Errorf("with nothing configured: %s, want %s", got, want)
	}
	want := filepath.Join(t.TempDir(), "mine.txt")
	t.Setenv("AUTHAMA_AGE_IDENTITY_FILE", want)
	if got := BackupKeyFile(p); got != want {
		t.Errorf("with the identity configured: %s, want %s", got, want)
	}
}
