package httpserver

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
)

type healthResponse struct {
	Status string `json:"status"`
}

type Pinger interface {
	Ping(ctx context.Context) error
}

func Liveness(c echo.Context) error {
	return c.JSON(http.StatusOK, healthResponse{Status: "ok"})
}

func ReadinessWithDB(pinger Pinger) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		if err := pinger.Ping(ctx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, healthResponse{Status: "unavailable"})
		}
		return c.JSON(http.StatusOK, healthResponse{Status: "ok"})
	}
}
