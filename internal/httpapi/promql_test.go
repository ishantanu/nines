package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func TestAPIRejectsMalformedPromQLWithoutSavingSLO(t *testing.T) {
	memory := store.NewMemory()
	handler := New(service.New(memory, nil), "secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	definition, err := model.ParseSLODefinition([]byte(examples.APIAvailability))
	if err != nil {
		t.Fatal(err)
	}
	slo := definition.SLO()
	slo.TotalQuery = "sum("
	body, err := json.Marshal(slo)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/api/slos", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "totalQuery") {
		t.Fatalf("invalid SLO accepted: %d %s", response.Code, response.Body.String())
	}
	yaml := strings.Replace(examples.APIAvailability, "sum(increase(http_requests_total[1m]))", "sum(", 1)
	for _, path := range []string{"/api/resources/validate", "/api/resources/plan", "/api/resources/apply"} {
		request := httptest.NewRequest("POST", path, strings.NewReader(yaml))
		request.Header.Set("Authorization", "Bearer secret")
		request.Header.Set("Content-Type", "application/yaml")
		request.Header.Set("If-Match", `"0"`)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 400 || !strings.Contains(response.Body.String(), "totalQuery") {
			t.Fatalf("%s accepted invalid SLO: %d %s", path, response.Code, response.Body.String())
		}
	}
	slos, err := memory.List(context.Background())
	if err != nil || len(slos) != 0 {
		t.Fatal("invalid SLO persisted", slos, err)
	}
}
