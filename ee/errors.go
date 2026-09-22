package ee

import "errors"

var (
	// errStoreUnavailable is reported when an Enterprise decorator was composed
	// without a metadata database and therefore cannot evaluate its rules.
	errStoreUnavailable = errors.New("enterprise policy store unavailable")
	// errGrantsUnavailable is reported when core did not supply a grant
	// explainer, which binding expiry needs to see the bindings behind a grant.
	errGrantsUnavailable = errors.New("core grant explainer unavailable")
)
