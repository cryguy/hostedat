package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func errorJSON(c echo.Context, code int, msg string) error {
	return c.JSON(code, ErrorResponse{Error: msg})
}

func CustomErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	var he *echo.HTTPError
	ok := errors.As(err, &he)
	if ok {
		msg, _ := he.Message.(string)
		if msg == "" {
			msg = http.StatusText(he.Code)
		}
		_ = errorJSON(c, he.Code, msg)
		return
	}

	_ = errorJSON(c, http.StatusInternalServerError, "internal server error")
}
