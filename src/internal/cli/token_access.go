package cli

import (
	"github.com/sangehq/authama/src/internal/core/tokens"
	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/services/tokensvc"
	"github.com/sangehq/authama/src/internal/util/log"
)

// Access events: a secret that left its file, to the terminal or the
// clipboard. Reading one changes nothing, so no guard records it; these put
// each one in the history, under the command that read it.
const (
	eventTokenRevealed = "token.revealed"
	eventTokenCopied   = "token.copied"
)

// recordTokenAccess files one access event per secret about to be shown or
// copied, naming the tool, host and file it came from, never the value. An
// entry with no secret was not read and is not recorded.
func recordTokenAccess(env tokensvc.Env, event string, items ...tokens.Installed) {
	for _, it := range items {
		if it.Secret == "" {
			continue
		}
		mutation.RecordAccess(env.Paths, event,
			log.Field{Key: "token", Value: it.Target + "/" + it.Host},
			log.Field{Key: "file", Value: it.File})
	}
}
