package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func requireQuery(r *http.Request, key string) (string, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return "", fmt.Errorf("missing %s", key)
	}
	return value, nil
}

func optionalQuery(r *http.Request, key string) *string {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil
	}
	return &value
}

func optionalUint64Query(r *http.Request, key string) (*uint64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s", key)
	}
	return &value, nil
}

func optionalBoolQuery(r *http.Request, key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid %s", key)
	}
	return value, nil
}

func positiveUint32Query(r *http.Request, key string, fallback uint32) (uint32, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return uint32(value), nil
}
