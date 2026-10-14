package settings

import "time"

// SetWatcherPoll shortens the poll of a stopped watcher so tests do not wait
// for the production interval.
func SetWatcherPoll(w *Watcher, d time.Duration) { w.poll = d }
