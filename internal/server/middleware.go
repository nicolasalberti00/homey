package server

// Transport hardening for the API surface (roadmap Steps 3.6 and 7.2):
//
//   - securityHeaders adds defensive headers to every response;
//   - cors answers preflights and tags responses for configured origins,
//     staying silent (same-origin only) by default;
//   - rateLimit caps mutating requests per client IP, over the API and the
//     MCP endpoint alike;
//   - bodyLimit keeps a request body from filling memory before validation
//     ever runs;
//   - authFailureLimit blocks client IPs that keep failing authentication.
//
// All limits are in-memory fixed windows: homey is a single-node, self-hosted
// service. Client IP is the direct peer address; when running behind a
// reverse proxy, rate limits apply to the proxy address, so keep that in mind
// when tuning them.

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// securityHeaders adds defensive headers to every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// cors answers preflights and tags responses for allowed origins. With no
// configured origins (the default) nothing is tagged: same-origin only.
func cors(allowedOrigins []string, next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" || !allowed[origin] {
			next.ServeHTTP(w, r)
			return
		}
		header := w.Header()
		header.Set("Access-Control-Allow-Origin", origin)
		header.Add("Vary", "Origin")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			header.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE")
			header.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			header.Set("Access-Control-Max-Age", "600")
			header.Add("Vary", "Access-Control-Request-Method")
			header.Add("Vary", "Access-Control-Request-Headers")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimit caps mutating requests per client IP with 429 and a
// Retry-After header. It counts the API writes and the MCP endpoint alike:
// /mcp is where an agent drives mutations through, and the endpoint cannot
// tell a read from a write without parsing the stream, so every POST to it
// counts. A limit of 0 disables it.
func rateLimit(perMinute int, next http.Handler) http.Handler {
	if perMinute <= 0 {
		return next
	}
	writes := newFixedWindow(time.Minute, perMinute)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isWriteMethod(r.Method) || !countsTowardWriteLimit(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if retryAfter, ok := writes.allow(clientIP(r)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeProblem(w, http.StatusTooManyRequests, "rate limit exceeded for mutating requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// countsTowardWriteLimit reports whether a request on path mutates: the API
// operations all live under /api/v1/, and the MCP endpoint is mounted at /mcp.
func countsTowardWriteLimit(path string) bool {
	return strings.HasPrefix(path, "/api/v1/") || path == "/mcp"
}

// authFailureLimit blocks client IPs whose failed authentications exceeded
// the limit. Failed attempts are detected from the 401 responses the auth
// middleware produces. Like rateLimit, the 429 carries Retry-After so clients
// can tell the user how long to wait. A limit of 0 disables it.
func authFailureLimit(perMinute int, next http.Handler) http.Handler {
	if perMinute <= 0 {
		return next
	}
	failures := newFixedWindow(time.Minute, perMinute)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if retryAfter := failures.retryAfter(ip); retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeProblem(w, http.StatusTooManyRequests, "too many failed authentication attempts, try again later")
			return
		}
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		if recorder.status == http.StatusUnauthorized {
			failures.allow(ip)
		}
	})
}

// writeProblem writes an RFC 9457 problem document for middleware failures.
// The middleware layer runs outside Huma, so the problem is written directly;
// the shape is the same ErrorModel the API uses.
func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
}

// maxRequestBodyBytes caps how much of a request body is read. An item is a
// few kilobytes and the largest operation today is a page of search results,
// so 4 MiB is room to grow while keeping a client from filling memory before
// validation ever sees its input (Step 7.2 security review).
const maxRequestBodyBytes int64 = 4 << 20

// bodyLimit refuses bodies over the cap: the announced length is rejected
// outright, and a body that grows past the cap while streaming is cut by
// MaxBytesReader wherever the reader happens to notice. Reading is bounded
// either way, so a flood cannot cost more than the cap.
func bodyLimit(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxBytes {
			writeProblem(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("request body exceeds %d bytes", maxBytes))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}

// clientIP returns the direct peer address of the request.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// fixedWindow counts events per key within a fixed time window.
type fixedWindow struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	keys   map[string]*windowCounter
}

type windowCounter struct {
	count int
	reset time.Time
}

func newFixedWindow(window time.Duration, limit int) *fixedWindow {
	return &fixedWindow{window: window, limit: limit, keys: make(map[string]*windowCounter)}
}

// allow counts one event for key. The first result is the seconds to wait
// when the key is over the limit, the second whether it may proceed.
func (f *fixedWindow) allow(key string) (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	counter := f.keys[key]
	if counter == nil || now.After(counter.reset) {
		f.cleanup(now)
		counter = &windowCounter{reset: now.Add(f.window)}
		f.keys[key] = counter
	}
	counter.count++
	if counter.count > f.limit {
		return int(counter.reset.Sub(now).Seconds()) + 1, false
	}
	return 0, true
}

// retryAfter returns the seconds until key's window resets when key has
// reached its limit, or 0 when it may proceed.
func (f *fixedWindow) retryAfter(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	counter := f.keys[key]
	now := time.Now()
	if counter == nil || !now.Before(counter.reset) || counter.count < f.limit {
		return 0
	}
	return int(counter.reset.Sub(now).Seconds()) + 1
}

// cleanup drops expired entries; called on window rollover with the lock
// held, and only when the map grew beyond a threshold to keep it cheap.
func (f *fixedWindow) cleanup(now time.Time) {
	if len(f.keys) < 4096 {
		return
	}
	for key, counter := range f.keys {
		if now.After(counter.reset) {
			delete(f.keys, key)
		}
	}
}
