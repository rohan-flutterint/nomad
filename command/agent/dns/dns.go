package dns

import "regexp"

// invalidNameRe is a regex that matches characters which can not be included in
// a DNS name.
var invalidNameRe = regexp.MustCompile(`[^A-Za-z0-9\\-]+`)
