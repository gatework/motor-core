# Settings publication and resource ownership

`Setup` loads a nonempty `settings` object, validates it, and publishes built-in
settings and registered extensions together. Missing, null, or empty settings
are rejected. Invalid reloads leave `Current()` and extension accessors at their
last successful publication.

Preparation callbacks run before publication. Each receives its own candidate
copy and must clean up resources it creates if it rejects that candidate.
`OnApplied` runs after publication and cannot reject it. Close the returned
watcher outside these callbacks; `Close` stops source watchers and waits for a
callback already running.

Settings publication does not open caches, queues, databases, or loggers. The
host constructs and injects resources explicitly. In particular, `Logger.Open`
returns a caller-owned root and may report an output I/O error; `Setup` only
validates the logger settings and never changes `logger.Default()`.

Keep logger roots alive while request field views borrow them. Replacing a
published logger does not transfer ownership or make the previous root safe to
close. The host closes it after its users have drained. Closing a settings
watcher does not close application resources.
