package googleauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestServiceAccountTokenIsCached(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		r.ParseForm()
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || strings.Count(r.Form.Get("assertion"), ".") != 2 {
			w.WriteHeader(400)
			return
		}
		w.Write([]byte(`{"access_token":"ya29.test","expires_in":3600}`))
	}))
	defer srv.Close()
	creds, _ := json.Marshal(map[string]string{"type": "service_account", "client_email": "ats@p.iam.gserviceaccount.com",
		"private_key": pemKey, "token_uri": srv.URL})
	ts, err := FromConfig(string(creds), "", CloudPlatformScope)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		tok, err := ts.Token(context.Background())
		if err != nil || tok != "ya29.test" {
			t.Fatalf("token = %q, %v", tok, err)
		}
	}
	if calls != 1 {
		t.Fatalf("token fetched %d times, want 1 (cached)", calls)
	}
}
