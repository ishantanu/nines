package systemone

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
)

func visualPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 280, 280))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	if e := png.Encode(&b, img); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestVisualImageNormalizationAndBounds(t *testing.T) {
	original := visualPNG(t)
	input := append(append([]byte{}, original...), []byte("private EXIF-like trailing payload")...)
	pixels, digest, e := PrepareVisualImage(base64.StdEncoding.EncodeToString(input))
	if e != nil || len(digest) != 64 || bytes.Contains(pixels, []byte("private")) {
		t.Fatal("normalization failed", e)
	}
	for _, bad := range []string{"not base64", base64.StdEncoding.EncodeToString([]byte("not an image")), strings.Repeat("A", base64.StdEncoding.EncodedLen(MaxVisualImageBytes)+4)} {
		if _, _, e := PrepareVisualImage(bad); e == nil {
			t.Fatal("invalid input accepted")
		}
	}
	var small bytes.Buffer
	_ = png.Encode(&small, image.NewRGBA(image.Rect(0, 0, 279, 280)))
	if _, _, e := PrepareVisualImage(base64.StdEncoding.EncodeToString(small.Bytes())); e == nil {
		t.Fatal("undersized image accepted")
	}
}
func TestVisualPNGChunkDetection(t *testing.T) {
	original := visualPNG(t)
	chunk := func(kind string, data []byte) []byte {
		b := make([]byte, len(data)+12)
		binary.BigEndian.PutUint32(b[:4], uint32(len(data)))
		copy(b[4:8], kind)
		copy(b[8:], data)
		binary.BigEndian.PutUint32(b[8+len(data):], crc32.ChecksumIEEE(b[4:8+len(data)]))
		return b
	}
	// Valid still-image metadata may contain the characters acTL.
	still := append(append(append([]byte{}, original[:len(original)-12]...), chunk("tEXt", []byte("note\x00acTL"))...), original[len(original)-12:]...)
	if _, _, err := PrepareVisualImage(base64.StdEncoding.EncodeToString(still)); err != nil {
		t.Fatal("still image rejected", err)
	}
	// An animation control chunk is rejected even when image.Decode would return a first frame.
	animated := append(append(append([]byte{}, original[:33]...), chunk("acTL", make([]byte, 8))...), original[33:]...)
	if _, _, err := PrepareVisualImage(base64.StdEncoding.EncodeToString(animated)); err == nil {
		t.Fatal("animated image accepted")
	}
}
func TestVisualAdapterOptInAndStrictResponses(t *testing.T) {
	good := `{"model":"jev-1.14.0","answers":{"success":{"type":"noul","noul":0.9},"error":{"type":"noul","noul":0.1},"sufficient":{"type":"noul","noul":0.8}},"usage":{"input_tokens":500,"output_tokens":20}}`
	for _, tc := range []struct {
		name, body     string
		status         int
		enabled, valid bool
	}{{"valid", good, 200, true, true}, {"disabled", good, 200, false, false}, {"wrong model", strings.Replace(good, "jev-1.14.0", "jev-latest", 1), 200, true, false}, {"invalid probability", strings.Replace(good, "0.9", "1.9", 1), 200, true, false}, {"missing", strings.Replace(good, `"success":{"type":"noul","noul":0.9},`, "", 1), 200, true, false}, {"null", strings.Replace(good, "0.9", "null", 1), 200, true, false}, {"bad type", strings.Replace(good, `"type":"noul"`, `"type":"box"`, 1), 200, true, false}, {"duplicate", strings.Replace(good, `"model":`, `"model":"bad","model":`, 1), 200, true, false}, {"provider error", good, 429, true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			c, e := New(&Config{ImagesEnabled: tc.enabled}, func(string) string { return "private-fixture-key" })
			if e != nil {
				t.Fatal(e)
			}
			calls := 0
			c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != endpoint || r.Header.Get("TypeSafe-Danger-Zone-Images") != "true" {
					t.Fatal("unsafe image route")
				}
				raw, _ := io.ReadAll(r.Body)
				var in struct {
					State  map[string]string `json:"state"`
					Assets map[string]struct {
						Format, DataBase64 string `json:"-"`
					} `json:"assets"`
					Questions map[string]question `json:"questions"`
				}
				if json.Unmarshal(raw, &in) != nil || len(in.Assets) != 1 || len(in.Questions) != 3 || in.State["screenshot"] == "" {
					t.Fatal("invalid request")
				}
				if strings.Contains(string(raw), c.key) {
					t.Fatal("key in payload")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}}, nil
			})
			out, e := c.AssessVisual(context.Background(), VisualInput{Criterion: "Order confirmation is visible", Image: visualPNG(t)})
			if (e == nil) != tc.valid {
				t.Fatal(out, e)
			}
			if !tc.enabled && calls != 0 {
				t.Fatal("disabled adapter made an external request")
			}
			if tc.valid && (out.Judgments == nil || out.Judgments.SuccessVisible != .9) {
				t.Fatal(out)
			}
		})
	}
}
