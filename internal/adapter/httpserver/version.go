package httpserver

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type versionResponse struct {
	Version string `json:"version"`
}

// VersionHandler reports the commit the running binary was built from, so the
// page can show which build is live.
func VersionHandler(version string) echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, versionResponse{Version: version})
	}
}
