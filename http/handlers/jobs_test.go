package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"main/whatsapp"
)

func setupHandlerTest(t *testing.T) {
	t.Helper()
	t.Setenv("DB_TYPE", "sqlite")
	t.Setenv("DATABASE_URL", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "test.db"))+"?_foreign_keys=on")
	t.Setenv("SEND_RATE_PER_MINUTE", "20")
	t.Setenv("SEND_TIMEOUT_SECONDS", "15")
	t.Chdir("../../whatsapp")
	whatsapp.InitDB()
	t.Cleanup(whatsapp.CloseDB)
	SetClient(whatsapp.NewWhatsAppClient())
}

func TestCreateMessageReturns202AndDeduplicates(t *testing.T) {
	setupHandlerTest(t)
	body := `{"chat_id":"120363012345678901@g.us","type":"text","text":"hola","idempotency_key":"delivery-1"}`
	first := httptest.NewRecorder()
	CreateMessageHandler(first, httptest.NewRequest(http.MethodPost, "/messages", bytes.NewBufferString(body)))
	if first.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", first.Code, first.Body.String())
	}
	var resp struct {
		Success      bool   `json:"success"`
		JobID        string `json:"job_id"`
		Status       string `json:"status"`
		Deduplicated bool   `json:"deduplicated"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success || resp.JobID == "" || resp.Status != "queued" || resp.Deduplicated {
		t.Fatalf("resp = %+v", resp)
	}

	second := httptest.NewRecorder()
	CreateMessageHandler(second, httptest.NewRequest(http.MethodPost, "/messages", bytes.NewBufferString(body)))
	if second.Code != http.StatusAccepted {
		t.Fatalf("status = %d", second.Code)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Deduplicated {
		t.Fatalf("second response not deduplicated: %+v", resp)
	}
}

func TestCreateMessageValidation(t *testing.T) {
	setupHandlerTest(t)
	cases := []string{
		`{"type":"text","text":"hola"}`,
		`{"chat_id":"a@s.whatsapp.net","type":"video"}`,
		`{"chat_id":"a@s.whatsapp.net","type":"text"}`,
		`{"chat_id":"a@s.whatsapp.net","type":"forward"}`,
	}
	for _, body := range cases {
		rr := httptest.NewRecorder()
		CreateMessageHandler(rr, httptest.NewRequest(http.MethodPost, "/messages", bytes.NewBufferString(body)))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, rr.Code)
		}
	}
}

func TestCreateMessageDeduplicatesTerminalJobs(t *testing.T) {
	setupHandlerTest(t)
	body := `{"chat_id":"a@s.whatsapp.net","type":"text","text":"hello","idempotency_key":"terminal-1"}`
	first := httptest.NewRecorder()
	CreateMessageHandler(first, httptest.NewRequest(http.MethodPost, "/messages", bytes.NewBufferString(body)))
	var created struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.JobID == "" {
		t.Fatalf("missing job id: %s", first.Body.String())
	}
	if err := whatsapp.UpdateJob(t.Context(), created.JobID, whatsapp.JobUpdate{
		Status:   whatsapp.JobStatusFailed,
		Attempts: 1,
	}); err != nil {
		t.Fatal(err)
	}

	second := httptest.NewRecorder()
	CreateMessageHandler(second, httptest.NewRequest(http.MethodPost, "/messages", bytes.NewBufferString(body)))
	var duplicate struct {
		JobID        string `json:"job_id"`
		Status       string `json:"status"`
		Deduplicated bool   `json:"deduplicated"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &duplicate); err != nil {
		t.Fatal(err)
	}
	if duplicate.JobID != created.JobID || duplicate.Status != whatsapp.JobStatusFailed || !duplicate.Deduplicated {
		t.Fatalf("duplicate response = %+v", duplicate)
	}
}

func TestListJobsOmitsMissingIDsAndEnforcesLimit(t *testing.T) {
	setupHandlerTest(t)
	job, _, err := whatsapp.CreateJob(t.Context(), whatsapp.Job{
		ChatID:  "a@s.whatsapp.net",
		Payload: whatsapp.JobPayload{Type: "text", Text: "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/jobs?ids="+job.ID+",missing", nil)
	rec := httptest.NewRecorder()
	ListJobsHandler(rec, req)
	var response struct {
		Success bool           `json:"success"`
		Jobs    []whatsapp.Job `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !response.Success || len(response.Jobs) != 1 || response.Jobs[0].ID != job.ID {
		t.Fatalf("response status=%d body=%s", rec.Code, rec.Body.String())
	}

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "id"
	}
	req = httptest.NewRequest(http.MethodGet, "/jobs?ids="+strings.Join(ids, ","), nil)
	rec = httptest.NewRecorder()
	ListJobsHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRetryJobHandlerReportsMissingJob(t *testing.T) {
	setupHandlerTest(t)
	req := httptest.NewRequest(http.MethodPost, "/jobs/missing/retry", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	RetryJobHandler(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}
