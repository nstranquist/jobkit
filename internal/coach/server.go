package coach

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/privatefs"
)

type Server struct {
	Store     *Store
	Providers *ProviderConfig
	Token     string
}

// NewServer creates a Coach server with an ephemeral access token.
func NewServer(store *Store, providers *ProviderConfig) (*Server, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("create coach access token: %w", err)
	}
	return &Server{
		Store: store, Providers: providers,
		Token: base64.RawURLEncoding.EncodeToString(tokenBytes),
	}, nil
}

// AccessURL returns the one-time bootstrap URL printed to the local terminal.
// The handler exchanges the query token for an HttpOnly same-site cookie and
// redirects to a clean URL.
func (s *Server) AccessURL(addr string) string {
	return "http://" + addr + "/?token=" + url.QueryEscape(s.Token)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /assets/coach.css", serveEmbedded("text/css; charset=utf-8", coachCSS))
	mux.HandleFunc("GET /assets/coach.js", serveEmbedded("text/javascript; charset=utf-8", coachJS))
	mux.HandleFunc("GET /assets/audio-worklet.js", serveEmbedded("text/javascript; charset=utf-8", audioWorkletJS))
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("GET /api/decks", s.handleDecks)
	mux.HandleFunc("GET /api/decks/{id}", s.handleDeck)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("POST /api/sessions", s.handleSession)
	mux.HandleFunc("POST /api/transcribe", s.handleTranscribe)
	return securityHeaders(localRequestBoundary(s.authBoundary(mux)))
}

func (s *Server) Serve(ctx context.Context, addr string) error {
	if err := validateLoopbackAddress(addr); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second,
		IdleTimeout: 60 * time.Second,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-done:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func validateLoopbackAddress(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid coach address %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("coach address must use localhost or a loopback IP")
	}
	return nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; media-src 'self' blob:; worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queryToken := r.URL.Query().Get("token")
		if queryToken != "" {
			if r.Method != http.MethodGet || r.URL.Path != "/" || !sameToken(queryToken, s.Token) {
				writeAPIError(w, http.StatusUnauthorized, fmt.Errorf("invalid coach access token"))
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name: "jobkit_coach", Value: s.Token, Path: "/", HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		token := strings.TrimPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer ")
		if token == "" {
			if cookie, err := r.Cookie("jobkit_coach"); err == nil {
				token = cookie.Value
			}
		}
		if !sameToken(token, s.Token) {
			writeAPIError(w, http.StatusUnauthorized, fmt.Errorf("coach access token is required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameToken(got, want string) bool {
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func serveEmbedded(contentType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = io.WriteString(w, body)
	}
}

// localRequestBoundary rejects DNS-rebinding hosts. Browser writes must also
// come from the exact origin that served the Coach UI. Non-browser clients do
// not send Origin and can continue to use the loopback API directly.
func localRequestBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			writeAPIError(w, http.StatusForbidden, fmt.Errorf("coach request host must be localhost or a loopback IP"))
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Scheme == "" || !strings.EqualFold(parsed.Host, r.Host) {
				writeAPIError(w, http.StatusForbidden, fmt.Errorf("coach browser request must use the same localhost origin"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(hostPort string) bool {
	host := strings.TrimSpace(hostPort)
	if split, _, err := net.SplitHostPort(host); err == nil {
		host = split
	} else if strings.Contains(host, ":") {
		return false
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, coachIndexHTML)
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	providers := []string{}
	if s.Providers != nil {
		for name := range s.Providers.Providers {
			providers = append(providers, name)
		}
		sort.Strings(providers)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version":          SchemaVersion,
		"transcription_available": s.Providers != nil && s.Providers.Transcriber != nil,
		"providers":               providers,
	})
}

func (s *Server) handleDecks(w http.ResponseWriter, _ *http.Request) {
	decks, err := s.Store.ListDecks()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"decks": decks})
}

func (s *Server) handleDeck(w http.ResponseWriter, r *http.Request) {
	deck, err := s.Store.LoadDeck(r.PathValue("id"))
	if err != nil {
		if os.IsNotExist(err) {
			writeAPIError(w, http.StatusNotFound, err)
			return
		}
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, deck)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	report, err := s.Store.Stats(time.Now().UTC(), r.URL.Query().Get("project"))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

type sessionRequest struct {
	DeckID   string   `json:"deck_id"`
	Answers  []Answer `json:"answers"`
	Provider string   `json:"provider,omitempty"`
	Useful   *bool    `json:"useful,omitempty"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
	var request sessionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("decode session request: %w", err))
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("decode session request: trailing JSON data"))
		return
	}
	deck, err := s.Store.LoadDeck(request.DeckID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	bundle, err := s.Store.LoadSource()
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	if err := ValidateSessionInput(deck, bundle, request.Answers); err != nil {
		writeAPIError(w, http.StatusConflict, err)
		return
	}
	now := time.Now().UTC()
	session := Evaluate(deck, bundle, request.Answers, now, now)
	session.Useful = request.Useful
	if request.Provider != "" && request.Provider != "none" {
		if s.Providers == nil {
			session.ProviderError = "coach provider configuration is missing"
		} else if feedback, feedbackErr := RunFeedback(r.Context(), s.Providers, request.Provider, bundle, deck, session); feedbackErr != nil {
			session.ProviderError = feedbackErr.Error()
		} else {
			session.Feedback = feedback
		}
	}
	if err := s.Store.AppendSession(session); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if s.Providers == nil || s.Providers.Transcriber == nil {
		writeAPIError(w, http.StatusNotImplemented, fmt.Errorf("coach transcriber is not configured"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 20*1024*1024)
	temp, err := os.CreateTemp(s.Store.Root, ".coach-audio-*.wav")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	path := temp.Name()
	defer os.Remove(path)
	if err := privatefs.Restrict(path, privatefs.FileMode); err != nil {
		temp.Close()
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := io.Copy(temp, r.Body); err != nil {
		temp.Close()
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	if err := temp.Close(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	text, err := RunTranscriber(r.Context(), s.Providers, path)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}

func writeAPIError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": err.Error()}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
