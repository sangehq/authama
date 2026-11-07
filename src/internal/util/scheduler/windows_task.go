package scheduler

// The Windows task definition, kept out of the build-tagged file that runs it,
// so the argv can be checked on any machine rather than only on Windows.

// notifyHour is when the daily expiry check runs. All three platforms read it
// from here: a user with a Mac and a Linux box should get the same behaviour,
// and the hour was previously a literal repeated behind three build tags, where
// no test could see more than one of them at a time.
const notifyHour = "09:00"

// vmSyncHour is when the daily VM host sync runs: after the notifier, so
// the two never queue on the lock.
const vmSyncHour = "09:30"

// exportsHour is when the daily scheduled exports run: after the VM sync,
// so a synced host is in them.
const exportsHour = "10:00"

// timerUnit is the systemd timer that drives the notifier, built from
// notifyHour so it cannot drift from the other two platforms.
func timerUnit() string { return timerUnitFor(Label) }

// timerUnitFor is the systemd timer for label's job.
func timerUnitFor(label string) string {
	j := jobFor(label)
	return `[Unit]
Description=` + j.description + ` (daily)

[Timer]
OnCalendar=*-*-* ` + j.at + `:00
Persistent=true

[Install]
WantedBy=timers.target
`
}

// schtasksArgs is the argv that registers the daily notifier task.
//
// /F is load-bearing: without it schtasks prompts for confirmation when a task
// of that name already exists, and a scheduled install would block forever on a
// prompt nobody is there to answer. Re-registering is how the command upgrades
// an existing task, so overwriting is the intended behaviour, not a shortcut.
func schtasksArgs(command, label string) []string {
	return []string{"schtasks", "/Create", "/TN", label, "/TR", command,
		"/SC", "DAILY", "/ST", jobFor(label).at, "/F"}
}

// schtasksDeleteArgs is the argv that removes a task; /F skips the prompt for
// the same reason as in schtasksArgs.
func schtasksDeleteArgs(label string) []string {
	return []string{"schtasks", "/Delete", "/TN", label, "/F"}
}
