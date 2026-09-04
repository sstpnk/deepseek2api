package history

import (
	"net/http"

	dsclient "ds2api/internal/deepseek/client"
)

func MapError(err error) (int, string) {
	switch {
	case dsclient.IsManagedUnauthorizedError(err):
		return http.StatusUnauthorized, "Account token is invalid. Refresh the configured DeepSeek account credentials."
	case dsclient.IsDirectUnauthorizedError(err):
		return http.StatusUnauthorized, "Invalid token. If this should be a DS2API key, add it to config.keys first."
	default:
		return http.StatusInternalServerError, err.Error()
	}
}
