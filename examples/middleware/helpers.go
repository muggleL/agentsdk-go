package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	requestIDKey     = "request_id"
	startedAtKey     = "started_at"
	promptKey        = "prompt"
	securityFlagsKey = "security.flags"
)

func genRequestID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err == nil {
		return fmt.Sprintf("req-%s", hex.EncodeToString(buf))
	}
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}

func clampPreview(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func readString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if val, ok := m[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func nowOr(stored any, fallback time.Time) time.Time {
	if t, ok := stored.(time.Time); ok {
		return t
	}
	return fallback
}
