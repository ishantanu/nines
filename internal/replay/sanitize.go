package replay

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const MaxBytes = 2 << 20
const MaxEvents = 2000
const MaxDurationMillis = 5 * 60 * 1000

var safeGeometry = regexp.MustCompile(`^[a-zA-Z0-9#.,%() +\-]*$`)
var unsafeGeometry = regexp.MustCompile(`(?i)url|https|data|javascript`)
var geometry = map[string]bool{}
var presentation = map[string]string{
	"data-orientation": " horizontal vertical ",
	"data-variant":     " default line outline secondary destructive ghost link ",
	"data-slot":        " tabs tabs-list tabs-trigger tabs-content card card-header card-title card-description card-action card-content card-footer badge button input progress progress-indicator select select-trigger select-value select-content select-item table table-header table-body table-row table-head table-cell ",
}
var flags = map[string]bool{"open": true, "hidden": true, "disabled": true, "checked": true, "selected": true}

var cssURL = regexp.MustCompile(`(?is)url\s*\([^)]*\)|@import[^;]*;|https?://[^\s"'<>;)]+`)
var cssContent = regexp.MustCompile(`(?is)(^|[;{])\s*(?:--tw-)?content\s*:(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^;}"'])*`)

func Mask(s string) string {
	if len(s) > 20000 {
		s = s[:20000]
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return r
		}
		return '*'
	}, s)
}
func cleanCSS(s string) string {
	if len(s) > 256<<10 {
		s = ""
	}
	return cssContent.ReplaceAllString(cssURL.ReplaceAllString(s, ""), "$1")
}
func number(v any) float64 { n, _ := v.(float64); return n }
func pick(m map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if n, ok := v.(float64); ok && n >= -100000 && n <= 10000000 {
				out[key] = n
			} else if b, ok := v.(bool); ok {
				out[key] = b
			}
		}
	}
	return out
}
func attributes(value any) map[string]any {
	in, _ := value.(map[string]any)
	out := map[string]any{}
	for key, v := range in {
		if v == nil && (key == "class" || key == "style" || flags[key]) {
			out[key] = nil
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		if values := presentation[key]; values != "" && s != "" && !strings.ContainsAny(s, " \t\r\n") && strings.Contains(values, " "+s+" ") {
			out[key] = s
			continue
		}
		if flags[key] {
			out[key] = ""
			continue
		}
		if geometry[strings.ToLower(key)] && len(s) <= 20000 && safeGeometry.MatchString(s) && !unsafeGeometry.MatchString(s) {
			out[key] = s
			continue
		}
		if key == "data-state" && (s == "open" || s == "closed" || s == "active" || s == "inactive" || s == "checked" || s == "unchecked") {
			out[key] = s
			continue
		}
		switch key {
		case "class":
			if len(s) <= 1024 {
				out[key] = s
			}
		case "style", "_cssText":
			out[key] = cleanCSS(s)
		case "width", "height", "rr_width", "rr_height":
			if regexp.MustCompile(`^[0-9.]{1,6}(px|%)?$`).MatchString(s) {
				out[key] = s
			}
		case "type":
			if s == "checkbox" || s == "radio" || s == "button" {
				out[key] = s
			}
		}
	}
	return out
}

//go:embed public-labels.json
var publicLabelFiles embed.FS
var publicLabels = map[string]bool{}
var tags = map[string]bool{}

func publicText(s, label string) string {
	if publicLabels[label] && strings.TrimSpace(s) == label {
		return s
	}
	return Mask(s)
}

func init() {
	raw, err := publicLabelFiles.ReadFile("public-labels.json")
	if err != nil {
		panic(err)
	}
	var labels []string
	if err := json.Unmarshal(raw, &labels); err != nil {
		panic(err)
	}
	for _, label := range labels {
		publicLabels[label] = true
	}
	for _, key := range strings.Fields("viewbox d x y x1 y1 x2 y2 cx cy r rx ry width height points transform stroke-width stroke-linecap stroke-linejoin fill stroke fill-opacity stroke-opacity opacity offset stop-color") {
		geometry[key] = true
	}
	for _, tag := range strings.Fields("html head body style div span p a button input textarea select option label form main section article header footer nav aside h1 h2 h3 h4 h5 h6 ul ol li table thead tbody tfoot tr th td caption colgroup col dl dt dd details summary pre code br hr strong em small b i u s img figure figcaption fieldset legend progress meter dialog svg g path rect circle ellipse line polyline polygon defs clippath lineargradient radialgradient stop text tspan") {
		tags[tag] = true
	}
}
func node(value any, depth int, count *int, style bool, labels map[float64]string, publicLabel string) (map[string]any, error) {
	*count++
	if depth > 60 || *count > 12000 {
		return nil, fmt.Errorf("snapshot exceeds node bounds")
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid snapshot node")
	}
	typ := int(number(m["type"]))
	delete(labels, number(m["id"]))
	out := map[string]any{"type": typ, "id": number(m["id"])}
	childLabel := ""
	switch typ {
	case 0, 2:
		if typ == 2 {
			attrs, _ := m["attributes"].(map[string]any)
			_, private := attrs["data-private"]
			_, blocked := attrs["data-replay-block"]
			if private || blocked {
				return map[string]any{"type": 2, "id": number(m["id"]), "tagName": "div", "attributes": map[string]any{}, "childNodes": []any{}}, nil
			}
			tag, _ := m["tagName"].(string)
			tag = strings.ToLower(tag)
			if tag == "link" {
				tag = "style"
			}
			if !tags[tag] {
				tag = "div"
				m = map[string]any{"childNodes": []any{}}
			}
			canonical := map[string]string{"clippath": "clipPath", "lineargradient": "linearGradient", "radialgradient": "radialGradient"}
			if c := canonical[tag]; c != "" {
				tag = c
			}
			out["tagName"] = tag
			out["attributes"] = attributes(m["attributes"])
			candidate, _ := attrs["data-replay-label"].(string)
			if labels != nil && tag == "span" && publicLabels[candidate] {
				childLabel = candidate
				labels[number(m["id"])] = candidate
				out["attributes"].(map[string]any)["data-replay-label"] = candidate
			}
			style = tag == "style"
			if m["isSVG"] == true {
				out["isSVG"] = true
			}
			if b, ok := m["isShadow"].(bool); ok {
				out["isShadow"] = b
			}
		}
		children, _ := m["childNodes"].([]any)
		clean := []any{}
		for _, child := range children {
			c, err := node(child, depth+1, count, style, labels, childLabel)
			if err != nil {
				return nil, err
			}
			clean = append(clean, c)
		}
		out["childNodes"] = clean
	case 1:
		out["name"] = "html"
		out["publicId"] = ""
		out["systemId"] = ""
	case 3, 4, 5:
		s, _ := m["textContent"].(string)
		if labels != nil && publicLabel != "" {
			labels[number(m["id"])] = publicLabel
		}
		if style {
			out["textContent"] = cleanCSS(s)
		} else {
			out["textContent"] = publicText(s, publicLabel)
		}
		if typ != 3 {
			out["type"] = 3
		}
	default:
		return nil, fmt.Errorf("unsupported snapshot node")
	}
	return out, nil
}

// Sanitize drops unsupported event/plugin payloads, scripts, media, URLs and identifying
// attributes. Recorders must also mask before transport. Playback has an independent CSP/sandbox.
func Sanitize(events []json.RawMessage) ([]json.RawMessage, error) {
	return sanitize(events, true)
}
func sanitize(events []json.RawMessage, selective bool) ([]json.RawMessage, error) {
	var labels map[float64]string
	if selective {
		labels = map[float64]string{}
	}
	if len(events) < 2 || len(events) > MaxEvents {
		return nil, fmt.Errorf("recording requires 2–2000 events")
	}
	out := []json.RawMessage{}
	var first, last float64
	full := false
	nodes := 0
	total := 0
	for i, raw := range events {
		total += len(raw)
		if total > MaxBytes {
			return nil, fmt.Errorf("recording exceeds 2 MiB")
		}
		var e struct {
			Type      int            `json:"type"`
			Timestamp float64        `json:"timestamp"`
			Data      map[string]any `json:"data"`
		}
		if json.Unmarshal(raw, &e) != nil || e.Timestamp < 1 || e.Data == nil {
			return nil, fmt.Errorf("invalid replay event")
		}
		if i == 0 {
			first = e.Timestamp
		}
		if e.Timestamp < last || e.Timestamp-first > MaxDurationMillis {
			return nil, fmt.Errorf("timestamps must be ordered within five minutes")
		}
		last = e.Timestamp
		data := map[string]any{}
		switch e.Type {
		case 4:
			data = pick(e.Data, "width", "height")
			data["href"] = "/"
		case 2:
			if number(asMap(e.Data["node"])["type"]) != 0 {
				return nil, fmt.Errorf("full snapshot requires a document root")
			}
			clear(labels)
			n, err := node(e.Data["node"], 0, &nodes, false, labels, "")
			if err != nil {
				return nil, err
			}
			data["node"] = n
			data["initialOffset"] = pick(asMap(e.Data["initialOffset"]), "left", "top")
			full = true
		case 3:
			source := int(number(e.Data["source"]))
			data["source"] = source
			switch source {
			case 0:
				for _, key := range []string{"texts", "attributes", "removes", "adds"} {
					items, _ := e.Data[key].([]any)
					clean := []any{}
					if len(items) > 12000 {
						return nil, fmt.Errorf("mutation exceeds bounds")
					}
					for _, value := range items {
						m, ok := value.(map[string]any)
						if !ok {
							return nil, fmt.Errorf("invalid mutation")
						}
						v := pick(m, "id", "parentId", "nextId", "previousId", "isShadow")
						switch key {
						case "texts":
							s, _ := m["value"].(string)
							v["value"] = publicText(s, labels[number(m["id"])])
						case "attributes":
							v["attributes"] = attributes(m["attributes"])
						case "adds":
							n, err := node(m["node"], 0, &nodes, false, labels, labels[number(m["parentId"])])
							if err != nil {
								return nil, err
							}
							v["node"] = n
						}
						clean = append(clean, v)
					}
					data[key] = clean
				}
			case 1, 6, 12:
				positions, _ := e.Data["positions"].([]any)
				if len(positions) > 1000 {
					return nil, fmt.Errorf("movement exceeds bounds")
				}
				clean := []any{}
				for _, v := range positions {
					clean = append(clean, pick(asMap(v), "id", "x", "y", "timeOffset"))
				}
				data["positions"] = clean
			case 2:
				data = pick(e.Data, "type", "id", "x", "y")
				data["source"] = source
			case 3:
				data = pick(e.Data, "id", "x", "y")
				data["source"] = source
			case 4:
				data = pick(e.Data, "width", "height")
				data["source"] = source
			case 5:
				data = pick(e.Data, "id", "isChecked")
				data["source"] = source
				s, _ := e.Data["text"].(string)
				data["text"] = Mask(s)
			default:
				continue
			}
		default:
			continue
		}
		b, err := json.Marshal(map[string]any{"type": e.Type, "timestamp": e.Timestamp, "data": data})
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if !full || len(out) < 2 {
		return nil, fmt.Errorf("recording needs a full snapshot and timeline")
	}
	return out, nil
}
func asMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
