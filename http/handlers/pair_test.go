package handlers

import (
	"errors"
	"main/whatsapp"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPairHandlerReturnsPairingCode(t *testing.T) {
	var gotPhone string
	handler := pairHandler(func(phone string) (string, error) {
		gotPhone = phone
		return "ABCD-EFGH", nil
	})

	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"phone":" 5491123456789 "}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if gotPhone != "5491123456789" {
		t.Fatalf("expected trimmed phone, got %q", gotPhone)
	}
	if !strings.Contains(rec.Body.String(), `"code":"ABCD-EFGH"`) {
		t.Fatalf("expected pairing code in response, got %s", rec.Body.String())
	}
}

func TestPairHandlerRejectsNonPost(t *testing.T) {
	handler := pairHandler(func(string) (string, error) {
		t.Fatal("pair function should not be called")
		return "", nil
	})

	req := httptest.NewRequest(http.MethodGet, "/pair", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestPairHandlerRejectsInvalidJSON(t *testing.T) {
	handler := pairHandler(func(string) (string, error) {
		t.Fatal("pair function should not be called")
		return "", nil
	})

	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"phone":`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPairHandlerRequiresPhone(t *testing.T) {
	handler := pairHandler(func(string) (string, error) {
		t.Fatal("pair function should not be called")
		return "", nil
	})

	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"phone":"   "}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPairHandlerRejectsAlreadyLinkedClient(t *testing.T) {
	handler := pairHandler(func(string) (string, error) {
		return "", whatsapp.ErrAlreadyLinked
	})

	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"phone":"5491123456789"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestPairHandlerRejectsInvalidPhone(t *testing.T) {
	handler := pairHandler(func(string) (string, error) {
		return "", whatsapp.ErrInvalidPhone
	})

	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"phone":"123"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPairHandlerReturnsServiceUnavailableWhenLoginIsNotReady(t *testing.T) {
	handler := pairHandler(func(string) (string, error) {
		return "", whatsapp.ErrLoginNotReady
	})

	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"phone":"5491123456789"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestPairHandlerReturnsInternalErrorForUnexpectedFailure(t *testing.T) {
	handler := pairHandler(func(string) (string, error) {
		return "", errors.New("unexpected")
	})

	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"phone":"5491123456789"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}
