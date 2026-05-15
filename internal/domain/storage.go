package domain

func SecretStorageKey(publicID string) string {
	return "secrets/" + publicID
}
