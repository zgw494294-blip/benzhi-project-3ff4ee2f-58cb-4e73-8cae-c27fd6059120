package query_error_chain_collapse_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strata-proof/internal/application"
	"strata-proof/internal/evidence"
	"strata-proof/internal/httpui"
	"strata-proof/internal/store"
)

func TestWrappedQueryErrorsKeepNotFoundHTTPStatus(t *testing.T) {
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	handler := httpui.NewHandler(application.NewService(repository, evidence.NewAnalyzer(), evidence.NewCredentialIssuer("private-error-secret"))).Routes()

	paths := []string{
		"/api/v1/dossiers/missing-dossier",
		"/api/v1/dossiers/missing-dossier/units/missing-unit/revisions",
		"/api/v1/dossiers/missing-dossier/relations/missing-relation/revisions",
		"/api/v1/dossiers/missing-dossier/checks",
		"/api/v1/dossiers/missing-dossier/relation-path?sourceUnitId=a&targetUnitId=b",
		"/api/v1/dossiers/missing-dossier/audit",
		"/api/v1/credentials/missing-credential",
	}
	failures := make([]string, 0, len(paths))
	for _, path := range paths {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"code":"not_found"`) {
			failures = append(failures, fmt.Sprintf("%s => %d %s", path, response.Code, strings.TrimSpace(response.Body.String())))
		}
	}
	if len(failures) != 0 {
		t.Fatalf("包装后的查询错误丢失 not_found 身份:\n%s", strings.Join(failures, "\n"))
	}
}
