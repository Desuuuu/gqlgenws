package util

import (
	"net/http"
	"net/textproto"
	"strings"
)

func HasHeader(h http.Header, name string) bool {
	name = textproto.CanonicalMIMEHeaderKey(name)
	return len(h[name]) > 0
}

func HeaderContains(h http.Header, name string, value string) bool {
	for _, t := range HeaderValues(h, name) {
		if strings.EqualFold(t, value) {
			return true
		}
	}

	return false
}

func HeaderValues(h http.Header, name string) []string {
	name = textproto.CanonicalMIMEHeaderKey(name)

	var values []string
	for _, l := range h[name] {
		for _, v := range strings.Split(l, ",") {
			values = append(values, strings.TrimSpace(v))
		}
	}

	return values
}
