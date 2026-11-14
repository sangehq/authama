package repair

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// ErrHistoryPermanent is what a history reset says when history.permanent
// (AUTHAMA_HISTORY_PERMANENT) is on.
var ErrHistoryPermanent = errors.New("the history is kept for good (history.permanent is on); turn it off in authama.json to start the history again")

// HistoryPermanent reports whether history.permanent is on, so the history
// may never be set aside.
func HistoryPermanent(get envnames.Getenv) bool {
	switch strings.ToLower(strings.TrimSpace(envnames.Get(get, envnames.HistoryPermanent))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// PlanHistoryReset says what ResetHistory would move aside, and where: the
// history database (and its write-ahead files) and the audit log it rebuilds
// itself from.
func PlanHistoryReset(p paths.Paths, now time.Time) [][2]string {
	stamp := now.UTC().Format("20060102-150405")
	var moves [][2]string
	for _, f := range []string{p.HistoryDB(), p.HistoryDB() + "-wal", p.HistoryDB() + "-shm", p.AuditLog()} {
		if fileExists(f) {
			moves = append(moves, [2]string{f, f + ".reset-" + stamp})
		}
	}
	return moves
}

// ResetHistory starts the history again, and deletes nothing: the history
// database and the audit log are renamed aside, so renaming them back undoes
// it. Both go, because a new history imports the audit log the first time it
// is opened; moving the database alone would bring the old record straight
// back. The next change starts a new history, which first lists what is on
// disk now (keys' dates, snapshots) as backfill. It refuses while
// history.permanent is on.
func ResetHistory(p paths.Paths, now time.Time, get envnames.Getenv) ([][2]string, error) {
	if HistoryPermanent(get) {
		return nil, ErrHistoryPermanent
	}
	moves := PlanHistoryReset(p, now)
	var done [][2]string
	for _, mv := range moves {
		if err := os.Rename(mv[0], mv[1]); err != nil {
			return done, fmt.Errorf("moving %s aside: %w", mv[0], err)
		}
		done = append(done, mv)
	}
	return done, nil
}
