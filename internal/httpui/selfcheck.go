package httpui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"strata-proof/internal/domain"
)

func SelfCheck(ctx context.Context, addr string, handler http.Handler) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("selfcheck 未执行：上下文已取消：%w", err)
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("selfcheck 监听 %s: %w", addr, err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second}
	done := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err == http.ErrServerClosed {
			err = nil
		}
		done <- err
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		select {
		case <-done:
		case <-shutdownCtx.Done():
		}
	}()
	base := "http://" + listener.Addr().String()
	client := &http.Client{Timeout: 3 * time.Second}
	pageRequest, err := http.NewRequestWithContext(ctx, "GET", base+"/workbench", nil)
	if err != nil {
		return err
	}
	pageResponse, err := client.Do(pageRequest)
	if err != nil {
		return fmt.Errorf("读取工作台页面: %w", err)
	}
	page, readErr := io.ReadAll(io.LimitReader(pageResponse.Body, 1<<20))
	pageResponse.Body.Close()
	if readErr != nil || pageResponse.StatusCode != http.StatusOK || !bytes.Contains(page, []byte("<body>")) {
		return fmt.Errorf("工作台页面自检失败")
	}
	var snapshot domain.Snapshot
	if err := call(ctx, client, "POST", base+"/api/v1/dossiers", map[string]any{"trenchCode": "SC-T01", "excavationArea": "自检区", "leadRecorder": "记录员甲", "chronologyHypothesis": "汉代", "actor": "记录员甲", "idempotencyKey": "self-create-001"}, &snapshot); err != nil {
		return err
	}
	if snapshot.Dossier.Status != domain.StatusDraft || snapshot.Dossier.Version != 1 {
		return fmt.Errorf("建档状态断言失败")
	}
	batch := map[string]any{"expectedVersion": snapshot.Dossier.Version, "rows": []map[string]any{{"row": 1, "unitCode": "SU-01", "unitType": "堆积", "description": "上层灰褐土", "topElevation": 101.2, "bottomElevation": 100.8, "soilTraits": "灰褐色粉砂土", "photoRefs": []string{"photo://SC-001"}}, {"row": 2, "unitCode": "SU-02", "unitType": "地面", "description": "下层硬化面", "topElevation": 100.8, "bottomElevation": 100.7, "soilTraits": "黄褐色夯土", "photoRefs": []string{"photo://SC-002"}}}, "actor": "记录员甲", "idempotencyKey": "self-unit-batch-001"}
	var batchResult struct {
		Snapshot domain.Snapshot `json:"snapshot"`
	}
	if err := call(ctx, client, "POST", base+"/api/v1/dossiers/"+snapshot.Dossier.ID+"/units/batch", batch, &batchResult); err != nil {
		return err
	}
	snapshot = batchResult.Snapshot
	firstID := snapshot.Units[0].ID
	secondID := snapshot.Units[1].ID
	relation := map[string]any{"expectedVersion": snapshot.Dossier.Version, "sourceUnitId": firstID, "targetUnitId": secondID, "relationType": "overlies", "evidenceNote": "剖面照片与标高共同表明 SU-01 叠压 SU-02", "actor": "记录员甲", "idempotencyKey": "self-rel-001"}
	if err := call(ctx, client, "POST", base+"/api/v1/dossiers/"+snapshot.Dossier.ID+"/relations", relation, &snapshot); err != nil {
		return err
	}
	var path domain.RelationPathResult
	if err := call(ctx, client, "GET", base+"/api/v1/dossiers/"+snapshot.Dossier.ID+"/relation-path?sourceUnitId="+firstID+"&targetUnitId="+secondID, nil, &path); err != nil {
		return err
	}
	if path.Classification != "direct" || len(path.Path) != 1 {
		return fmt.Errorf("关系路径追溯断言失败")
	}
	if err := call(ctx, client, "POST", base+"/api/v1/dossiers/"+snapshot.Dossier.ID+"/checks", versioned(snapshot, "记录员甲", "self-check-001"), &snapshot); err != nil {
		return err
	}
	if len(snapshot.Findings) != 0 || snapshot.LastCheckRunID == "" {
		return fmt.Errorf("一致性检查应无问题")
	}
	var batches []domain.CheckBatch
	if err := call(ctx, client, "GET", base+"/api/v1/dossiers/"+snapshot.Dossier.ID+"/checks", nil, &batches); err != nil {
		return err
	}
	if len(batches) != 1 || batches[0].ID != snapshot.LastCheckRunID {
		return fmt.Errorf("检查批次查询断言失败")
	}
	if err := call(ctx, client, "POST", base+"/api/v1/dossiers/"+snapshot.Dossier.ID+"/submit", versioned(snapshot, "记录员甲", "self-submit-001"), &snapshot); err != nil {
		return err
	}
	review := map[string]any{"expectedVersion": snapshot.Dossier.Version, "actor": "复核员乙", "idempotencyKey": "self-review-001", "approved": true, "reviewer": "复核员乙", "note": "记录完整，关系证据充分", "checklist": map[string]bool{"unitCompleteness": true, "relationEvidence": true, "findingClosure": true, "manifestPreview": true}}
	if err := call(ctx, client, "POST", base+"/api/v1/dossiers/"+snapshot.Dossier.ID+"/review", review, &snapshot); err != nil {
		return err
	}
	if snapshot.Dossier.Status != domain.StatusFrozen || snapshot.Manifest == nil {
		return fmt.Errorf("复核冻结断言失败")
	}
	issue := map[string]any{"expectedVersion": snapshot.Dossier.Version, "actor": "负责人丙", "idempotencyKey": "self-issue-001", "issuedBy": "负责人丙"}
	if err := call(ctx, client, "POST", base+"/api/v1/dossiers/"+snapshot.Dossier.ID+"/credentials", issue, &snapshot); err != nil {
		return err
	}
	if len(snapshot.Credentials) != 1 || snapshot.Dossier.Status != domain.StatusIssued {
		return fmt.Errorf("凭据签发断言失败")
	}
	var verification struct {
		Valid bool                `json:"valid"`
		Audit []domain.AuditEntry `json:"audit"`
	}
	if err := call(ctx, client, "GET", base+"/api/v1/credentials/"+snapshot.Credentials[0].CredentialID, nil, &verification); err != nil {
		return err
	}
	if !verification.Valid || len(verification.Audit) != 7 {
		return fmt.Errorf("凭据验真或审计时间线断言失败")
	}
	oldID := snapshot.Credentials[0].CredentialID
	revoke := map[string]any{"expectedVersion": snapshot.Dossier.Version, "actor": "负责人丙", "idempotencyKey": "self-revoke-001", "reason": "自检撤销"}
	if err := call(ctx, client, "POST", base+"/api/v1/dossiers/"+snapshot.Dossier.ID+"/credentials/"+oldID+"/revoke", revoke, &snapshot); err != nil {
		return err
	}
	if err := call(ctx, client, "GET", base+"/api/v1/credentials/"+oldID, nil, &verification); err != nil {
		return err
	}
	if verification.Valid {
		return fmt.Errorf("已撤销凭据不应有效")
	}
	if err := call(ctx, client, "POST", base+"/api/v1/dossiers/"+snapshot.Dossier.ID+"/credentials/"+oldID+"/reissue", versioned(snapshot, "负责人丙", "self-reissue-001"), &snapshot); err != nil {
		return err
	}
	newID := snapshot.Credentials[1].CredentialID
	if err := call(ctx, client, "GET", base+"/api/v1/credentials/"+newID, nil, &verification); err != nil {
		return err
	}
	if !verification.Valid || len(verification.Audit) != 9 {
		return fmt.Errorf("补发凭据验真断言失败")
	}
	fmt.Printf("selfcheck 通过：案卷 %s，替代凭据 %s，审计 %d 条\n", snapshot.Dossier.ID, newID, len(verification.Audit))
	return nil
}

func versioned(snapshot domain.Snapshot, actor, key string) map[string]any {
	return map[string]any{"expectedVersion": snapshot.Dossier.Version, "actor": actor, "idempotencyKey": key}
}

func call(ctx context.Context, client *http.Client, method, url string, body any, target any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s %s 返回 %d: %s", method, url, response.StatusCode, string(data))
	}
	if target != nil {
		return json.Unmarshal(data, target)
	}
	return nil
}
