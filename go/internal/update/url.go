package update

import (
	"net/url"
	"strings"
)

// AllowedURL reports whether u may be fetched in a release build: https, no
// userinfo, host github.com or a subdomain of githubusercontent.com.
func AllowedURL(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "github.com" || strings.HasSuffix(h, ".githubusercontent.com")
}
