package client

import (
	"net/http"

	"github.com/ishantanu/nines/internal/deprecation"
)

// warningTransport reports each Warning header a Nines server sends, such as
// a deprecation, to the configured handler.
type warningTransport struct {
	base   http.RoundTripper
	notify func(string)
}

func (t warningTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.base.RoundTrip(req)
	if res != nil {
		for _, value := range res.Header.Values("Warning") {
			if text, ok := deprecation.ParseWarning(value); ok {
				t.notify(text)
			}
		}
	}
	return res, err
}

// WarningTransport wraps base (nil means http.DefaultTransport) so that
// notify receives the text of each Warning header in responses.
func WarningTransport(base http.RoundTripper, notify func(string)) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return warningTransport{base: base, notify: notify}
}
