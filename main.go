package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "strconv"
    "syscall"
    "time"
)

type sseWriter struct {
    responseWriter http.ResponseWriter
    flusher        http.Flusher
}

func newSSEWriter(w http.ResponseWriter) (*sseWriter, bool) {
    f, ok := w.(http.Flusher)
    if !ok {
        return nil, false
    }
    return &sseWriter{responseWriter: w, flusher: f}, true
}

func (w *sseWriter) writeRetry(ms int) error {
    _, err := fmt.Fprintf(w.responseWriter, "retry: %d\n\n", ms)
    if err != nil {
        return err
    }
    w.flusher.Flush()
    return nil
}

func (w *sseWriter) writeEvent(eventName string, data string, id string) error {
    if id != "" {
        if _, err := fmt.Fprintf(w.responseWriter, "id: %s\n", id); err != nil {
            return err
        }
    }
    if eventName != "" {
        if _, err := fmt.Fprintf(w.responseWriter, "event: %s\n", eventName); err != nil {
            return err
        }
    }
    if _, err := fmt.Fprintf(w.responseWriter, "data: %s\n\n", data); err != nil {
        return err
    }
    w.flusher.Flush()
    return nil
}

func parseInterval(r *http.Request, defaultMs int) time.Duration {
    q := r.URL.Query().Get("intervalMs")
    if q == "" {
        return time.Duration(defaultMs) * time.Millisecond
    }
    v, err := strconv.Atoi(q)
    if err != nil || v <= 0 {
        return time.Duration(defaultMs) * time.Millisecond
    }
    return time.Duration(v) * time.Millisecond
}

func parseStart(r *http.Request) int {
    q := r.URL.Query().Get("start")
    if q == "" {
        return 0
    }
    v, err := strconv.Atoi(q)
    if err != nil || v < 0 {
        return 0
    }
    return v
}

func parseLimit(r *http.Request) int {
    q := r.URL.Query().Get("limit")
    if q == "" {
        return 0
    }
    v, err := strconv.Atoi(q)
    if err != nil || v < 0 {
        return 0
    }
    return v
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("Access-Control-Allow-Origin", getEnv("CORS_ALLOW_ORIGIN", "*"))

    sw, ok := newSSEWriter(w)
    if !ok {
        http.Error(w, "streaming unsupported", http.StatusInternalServerError)
        return
    }

    _ = sw.writeRetry(1000)

    ctx := r.Context()
    defaultInterval, _ := strconv.Atoi(getEnv("STREAM_INTERVAL_MS", "100"))
    interval := parseInterval(r, defaultInterval)
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    sequence := 0
    if last := r.Header.Get("Last-Event-ID"); last != "" {
        if n, err := strconv.Atoi(last); err == nil && n >= 0 {
            sequence = n + 1
        }
    }
    if start := parseStart(r); start > 0 {
        sequence = start
    }
    max := parseLimit(r)
    sent := 0
    for {
        select {
        case <-ctx.Done():
            return
        case t := <-ticker.C:
            id := strconv.Itoa(sequence)
            data := strconv.Itoa(sequence)
            if err := sw.writeEvent("number", data, id); err != nil {
                return
            }
            sequence++
            if max > 0 {
                sent++
                if sent >= max {
                    return
                }
            }
        }
    }
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    _, _ = w.Write([]byte("ok"))
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    _, _ = w.Write([]byte("/stream streams numbers via SSE. params: intervalMs,start,limit"))
}

func corsPreflight(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        allowOrigin := getEnv("CORS_ALLOW_ORIGIN", "*")
        w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
        w.Header().Set("Vary", "Origin")
        if r.Method == http.MethodOptions {
            w.Header().Set("Access-Control-Allow-Methods", "GET,OPTIONS")
            w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Last-Event-ID")
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}

func withServer(addr string, handler http.Handler) *http.Server {
    return &http.Server{Addr: addr, Handler: handler, ReadTimeout: 0, WriteTimeout: 0}
}

func gracefulServe(srv *http.Server) error {
    errCh := make(chan error, 1)
    go func() { errCh <- srv.ListenAndServe() }()
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    select {
    case err := <-errCh:
        return err
    case <-sigCh:
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        _ = srv.Shutdown(ctx)
        return http.ErrServerClosed
    }
}

func getEnv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/", rootHandler)
    mux.HandleFunc("/health", healthHandler)
    mux.HandleFunc("/stream", streamHandler)

    port := getEnv("PORT", "8080")
    srv := withServer(":"+port, corsPreflight(mux))

    if err := gracefulServe(srv); err != nil && err != http.ErrServerClosed {
        log.Fatalf("server error: %v", err)
    }

    _ = srv.Shutdown(context.Background())
}


