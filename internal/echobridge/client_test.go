package echobridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newServer returns a test EchoBridge that answers both API calls and
// passes each request to record.
func newServer(t *testing.T, record func(*http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record(r)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/eb/api/accounts":
			_, _ = w.Write([]byte(`[{"id":"a1","provider":"bluesky","display_name":"me"}]`))
		case "/eb/api/posts":
			_, _ = w.Write([]byte(`{"post_id":"p1","results":[{"account_id":"a1","success":true,"post_url":"https://x/1"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAuthorizationHeader(t *testing.T) {
	for _, tc := range []struct {
		name, token, want string
	}{
		{"with token", "s3cret", "Bearer s3cret"},
		{"without token", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			var present []bool
			srv := newServer(t, func(r *http.Request) {
				got = append(got, r.Header.Get("Authorization"))
				_, ok := r.Header["Authorization"]
				present = append(present, ok)
			})
			c := &Client{BaseURL: srv.URL + "/eb/", Token: tc.token, HTTP: srv.Client()}

			if _, err := c.Accounts(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Post(context.Background(), "a1", "hello"); err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 {
				t.Fatalf("got %d requests, want 2", len(got))
			}
			for i := range got {
				if got[i] != tc.want {
					t.Errorf("request %d: Authorization = %q, want %q", i, got[i], tc.want)
				}
				if present[i] != (tc.want != "") {
					t.Errorf("request %d: Authorization header present = %v", i, present[i])
				}
			}
		})
	}
}
