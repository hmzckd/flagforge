package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func etag(version int64) string {
	return `"` + strconv.FormatInt(version, 10) + `"`
}

func ifMatchVersion(r *http.Request) (int64, error) {
	header := strings.TrimSpace(r.Header.Get("If-Match"))
	if header == "" {
		return 0, errors.New("If-Match header is required; send the ETag from GET")
	}
	unquoted, ok := strings.CutPrefix(header, `"`)
	if ok {
		unquoted, ok = strings.CutSuffix(unquoted, `"`)
	}
	version, err := strconv.ParseInt(unquoted, 10, 64)
	if !ok || err != nil || version < 1 {
		return 0, errors.New(`If-Match must be a single ETag from GET, such as "3"`)
	}
	return version, nil
}
