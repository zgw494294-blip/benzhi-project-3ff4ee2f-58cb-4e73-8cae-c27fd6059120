package httpui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strata-proof/internal/application"
	"strata-proof/internal/evidence"
	"strata-proof/internal/store"
)

func TestWorkbenchAndJSONBoundary(t *testing.T) {
	repo, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	service := application.NewService(repo, evidence.NewAnalyzer(), evidence.NewCredentialIssuer("secret"))
	handler := NewHandler(service).Routes()
	request := httptest.NewRequest("GET", "/workbench", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "<body>") {
		t.Fatalf("工作台页面无效: %d", response.Code)
	}
	request = httptest.NewRequest("POST", "/api/v1/dossiers", strings.NewReader(`{"trenchCode":"T","unknown":true}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_json") {
		t.Fatalf("未知 JSON 字段应被拒绝: %d %s", response.Code, response.Body.String())
	}
}
