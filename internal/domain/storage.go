package domain

// UploadStorageKey is the object key an upload session writes to. Session IDs
// are unique, so no two uploads ever share an object: a failed or rejected
// upload can delete its own object without touching anyone else's.
func UploadStorageKey(sessionID string) string {
	return "blobs/" + sessionID
}
