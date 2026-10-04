package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type fakeAgoraService struct {
	seen uuid.UUID
	err  error
}

func (s *fakeAgoraService) Account(_ context.Context, id uuid.UUID) (AgoraAccount, error) {
	s.seen = id
	return AgoraAccount{Enabled: true, State: "ready", Localpart: "agamemnon", Homeserver: "https://agora.test", UserID: "@agamemnon:agora.test"}, s.err
}
func (s *fakeAgoraService) Password(_ context.Context, id uuid.UUID) (AgoraPassword, error) {
	s.seen = id
	return AgoraPassword{Password: "response-only-secret", UserID: "@agamemnon:agora.test", Homeserver: "https://agora.test"}, s.err
}

func agoraTestToken(t *testing.T, id uuid.UUID) string {
	t.Helper()
	claims := auth.Claims{PlayerID: id, Username: "test", RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("agora-test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func agoraTestRouter(service AgoraService) http.Handler {
	h := NewAgoraHandler(service)
	r := chi.NewRouter()
	r.Use(RequireAgoraBearer, auth.Middleware(auth.NewService(nil, "agora-test-secret")))
	r.Get("/agora", h.Get)
	r.Post("/agora/password", h.Password)
	return r
}

func TestAgoraAPIRequiresBearerAndNoStoreForEveryOutcome(t *testing.T) {
	owner := uuid.New()
	token := agoraTestToken(t, owner)
	s := &fakeAgoraService{}
	for _, path := range []string{"/agora", "/agora/password"} {
		for _, credential := range []string{"missing", "cookie", "invalid", "bearer"} {
			t.Run(path+credential, func(t *testing.T) {
				method := http.MethodGet
				if strings.HasSuffix(path, "password") {
					method = http.MethodPost
				}
				req := httptest.NewRequest(method, path, nil)
				s.seen = uuid.Nil
				switch credential {
				case "cookie":
					req.AddCookie(&http.Cookie{Name: "poleia_token", Value: token})
				case "invalid":
					req.Header.Set("Authorization", "Bearer wrong")
				case "bearer":
					req.Header.Set("Authorization", "Bearer "+token)
				}
				w := httptest.NewRecorder()
				agoraTestRouter(s).ServeHTTP(w, req)
				want := http.StatusUnauthorized
				if credential == "bearer" {
					want = http.StatusOK
				}
				if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("status=%d cache=%q", w.Code, w.Header().Get("Cache-Control"))
				}
				if credential == "bearer" && s.seen != owner || credential != "bearer" && s.seen != uuid.Nil {
					t.Fatal("service saw incorrect owner or unauthorized call")
				}
			})
		}
	}
}

func TestAgoraDisabledPendingAndUpstreamFailuresAreExplicitAndRedacted(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		service    AgoraService
		status     int
		want       string
	}{
		{"disabled-get", "/agora", nil, 200, `"enabled":false`},
		{"disabled-password", "/agora/password", nil, 503, "Community chat is unavailable"},
		{"pending-password", "/agora/password", &fakeAgoraService{err: ErrAgoraPending}, 409, "not ready yet"},
		{"upstream-password", "/agora/password", &fakeAgoraService{err: errors.New("password-canary token-canary")}, 503, "try again later"},
		{"upstream-get", "/agora", &fakeAgoraService{err: errors.New("password-canary token-canary")}, 503, "try again later"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodGet
			if strings.HasSuffix(tc.path, "password") {
				method = http.MethodPost
			}
			req := httptest.NewRequest(method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+agoraTestToken(t, uuid.New()))
			w := httptest.NewRecorder()
			agoraTestRouter(tc.service).ServeHTTP(w, req)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), "canary") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d; expected safe error and no-store", w.Code)
			}
		})
	}
}
