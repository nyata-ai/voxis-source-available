package domain

import "fmt"

// MediaStorageKey returns the encrypted media object key. OSS has one storage namespace.
func MediaStorageKey(orgID, mediaID string, _ RecordingMode) string {
	return fmt.Sprintf("orgs/%s/media/%s/encrypted.bin", orgID, mediaID)
}

// IsPrivilegeStorageKey always returns false because OSS has no special storage namespace.
func IsPrivilegeStorageKey(string) bool { return false }

// applyPrivilegePrefix preserves common recording call signatures without changing an OSS key.
func applyPrivilegePrefix(key string, _ RecordingMode) string { return key }

// OrganizationStoragePrefix returns the prefix under which every stored
// object of an organization (media and recording chunks) lives.
func OrganizationStoragePrefix(orgID string) string {
	return fmt.Sprintf("orgs/%s/", orgID)
}
