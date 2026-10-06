package httpserver

import "mime"

// Go's built-in table knows .json but not .webmanifest, and the container
// image has no /etc/mime.types to fall back on; without this, the web app
// manifest would be served as text.
func init() {
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}
