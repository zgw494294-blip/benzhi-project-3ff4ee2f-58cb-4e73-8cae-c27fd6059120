package httpui

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

func operationalMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			var raw [8]byte
			if _, err := rand.Read(raw[:]); err == nil {
				requestID = hex.EncodeToString(raw[:])
			} else {
				requestID = "unavailable"
			}
		}
		w.Header().Set("X-Request-ID", requestID)
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("HTTP panic request_id=%s path=%s error=%v\n%s", requestID, r.URL.Path, recovered, debug.Stack())
				if !wrapped.wroteHeader {
					problem(wrapped, http.StatusInternalServerError, "internal", "服务处理请求时发生内部错误", nil)
				}
			}
			log.Printf("HTTP request_id=%s method=%s path=%s status=%d duration=%s", requestID, r.Method, r.URL.Path, wrapped.status, time.Since(started).Round(time.Millisecond))
		}()
		next.ServeHTTP(wrapped, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}
