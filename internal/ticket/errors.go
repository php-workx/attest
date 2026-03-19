package ticket

import "errors"

// Sentinel errors for the ticket store.
var (
	ErrTicketNotFound = errors.New("ticket not found")
	ErrAmbiguousID    = errors.New("ambiguous ticket ID")
	ErrIDCollision    = errors.New("ticket ID collision after retries")
	ErrCorruptYAML    = errors.New("corrupt ticket YAML")
	ErrCycleDetected  = errors.New("dependency cycle detected")
	ErrPartialRead    = errors.New("partial read: some ticket files were skipped")
)
