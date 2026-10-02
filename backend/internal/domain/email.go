package domain

import "strings"

// NormalizeEmailAlias derives the stable duplicate-detection key stored with a
// provisioned user. Only Gmail aliases fold dots and plus tags because other
// providers can deliver those forms to different mailboxes.
func NormalizeEmailAlias(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndexByte(normalized, '@')
	if at < 0 {
		return normalized
	}
	local, host := normalized[:at], normalized[at+1:]
	if host == "googlemail.com" {
		host = "gmail.com"
	}
	if host != "gmail.com" {
		return local + "@" + host
	}
	if plus := strings.IndexByte(local, '+'); plus >= 0 {
		local = local[:plus]
	}
	local = strings.ReplaceAll(local, ".", "")
	if local == "" {
		local = normalized[:at]
	}
	return local + "@" + host
}
