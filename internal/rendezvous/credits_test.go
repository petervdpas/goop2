package rendezvous

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mockCreditsService(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/credits/balance", func(w http.ResponseWriter, r *http.Request) {
		email := r.URL.Query().Get("email")
		if email == "" {
			http.Error(w, "email required", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"balance": 500, "email": email})
	})

	mux.HandleFunc("/api/credits/access", func(w http.ResponseWriter, r *http.Request) {
		dir := r.URL.Query().Get("template_dir")
		email := r.URL.Query().Get("email")
		allowed := email != "" && dir != "premium"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"allowed": allowed})
	})

	mux.HandleFunc("/api/credits/spend", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email    string `json:"email"`
			Template string `json:"template"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Email == "" {
			http.Error(w, "email required", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "spent", "remaining": 400})
	})

	mux.HandleFunc("/api/credits/store-data", func(w http.ResponseWriter, r *http.Request) {
		email := r.URL.Query().Get("email")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"credits_active": true,
			"email":          email,
			"balance":        500,
			"app_name":       "TestGoop",
		})
	})

	mux.HandleFunc("/api/credits/template-info", func(w http.ResponseWriter, r *http.Request) {
		dir := r.URL.Query().Get("template_dir")
		email := r.URL.Query().Get("email")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case email != "" && dir == "owned-tpl":
			json.NewEncoder(w).Encode(map[string]any{"price": 100, "status": "owned"})
		case dir == "priced-tpl":
			json.NewEncoder(w).Encode(map[string]any{"price": 50, "status": "priced"})
		default:
			json.NewEncoder(w).Encode(map[string]any{"price": 0, "status": "free"})
		}
	})

	mux.HandleFunc("/api/credits/grant", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email  string `json:"email"`
			Amount int    `json:"amount"`
			Reason string `json:"reason"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "granted", "balance": 600})
	})

	return httptest.NewServer(mux)
}

func testCreditProvider(t *testing.T) (*RemoteCreditProvider, *httptest.Server) {
	t.Helper()
	svc := mockCreditsService(t)
	t.Cleanup(svc.Close)

	emails := map[string]string{"peer-1": "alice@test.com", "peer-2": ""}
	tokens := map[string]string{"peer-1": "token-abc"}

	p := NewRemoteCreditProvider(
		svc.URL,
		func(peerID string) string { return emails[peerID] },
		func(peerID string) string { return tokens[peerID] },
		"admin-secret",
	)
	return p, svc
}

func TestNoCreditsAllowsAll(t *testing.T) {
	nc := NoCredits{}

	r := httptest.NewRequest("GET", "/", nil)
	tpl := StoreMeta{Name: "Test", Dir: "test"}

	if !nc.TemplateAccessAllowed(r, tpl) {
		t.Fatal("NoCredits should allow all access")
	}

	data := nc.StorePageData(r)
	if data.Banner == "" {
		t.Fatal("NoCredits should return a banner")
	}
	if !strings.Contains(string(data.Banner), "free") {
		t.Fatal("NoCredits banner should mention free")
	}

	info := nc.TemplateStoreInfo(r, tpl)
	if !strings.Contains(string(info.PriceLabel), "Free") {
		t.Fatal("NoCredits price label should say Free")
	}
}

func TestRemoteCreditBalanceProxy(t *testing.T) {
	p, svc := testCreditProvider(t)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/credits/balance?peer_id=peer-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["balance"] != float64(500) {
		t.Fatalf("expected balance=500, got %v", resp["balance"])
	}
	_ = svc
}

func TestRemoteCreditBalanceNoEmail(t *testing.T) {
	p, _ := testCreditProvider(t)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/credits/balance?peer_id=peer-2", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["balance"] != float64(0) {
		t.Fatalf("no-email peer should get balance=0, got %v", resp["balance"])
	}
}

func TestRemoteCreditAccessAllowed(t *testing.T) {
	p, _ := testCreditProvider(t)

	tpl := StoreMeta{Name: "Free Template", Dir: "free-tpl"}
	r := httptest.NewRequest("GET", "/?peer_id=peer-1", nil)

	if !p.TemplateAccessAllowed(r, tpl) {
		t.Fatal("peer with email should be allowed for non-premium template")
	}
}

func TestRemoteCreditAccessDeniedPremium(t *testing.T) {
	p, _ := testCreditProvider(t)

	tpl := StoreMeta{Name: "Premium", Dir: "premium"}
	r := httptest.NewRequest("GET", "/?peer_id=peer-1", nil)

	if p.TemplateAccessAllowed(r, tpl) {
		t.Fatal("premium template should be denied")
	}
}

func TestRemoteCreditSpendProxy(t *testing.T) {
	p, _ := testCreditProvider(t)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	body := `{"template":"quiz"}`
	req := httptest.NewRequest("POST", "/api/credits/spend?peer_id=peer-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "spent" {
		t.Fatalf("expected status=spent, got %v", resp["status"])
	}
}

func TestRemoteCreditSpendNoEmail(t *testing.T) {
	p, _ := testCreditProvider(t)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	body := `{"template":"quiz"}`
	req := httptest.NewRequest("POST", "/api/credits/spend?peer_id=peer-2", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("spend without email should return 400, got %d", w.Code)
	}
}

func TestRemoteCreditStorePageData(t *testing.T) {
	p, _ := testCreditProvider(t)

	r := httptest.NewRequest("GET", "/?peer_id=peer-1", nil)
	data := p.StorePageData(r)

	if data.Banner == "" {
		t.Fatal("expected a banner")
	}
	if !strings.Contains(string(data.Banner), "alice@test.com") {
		t.Fatalf("banner should contain email, got %s", data.Banner)
	}
	if !strings.Contains(string(data.Banner), "500") {
		t.Fatalf("banner should contain balance, got %s", data.Banner)
	}
}

func TestRemoteCreditStorePageDataNoEmail(t *testing.T) {
	p, _ := testCreditProvider(t)

	r := httptest.NewRequest("GET", "/?peer_id=peer-2", nil)
	data := p.StorePageData(r)

	if !strings.Contains(string(data.Banner), "peer_id") {
		t.Fatalf("no-email banner should prompt to link account, got %s", data.Banner)
	}
}

func TestRemoteCreditTemplateInfoOwned(t *testing.T) {
	p, _ := testCreditProvider(t)

	tpl := StoreMeta{Dir: "owned-tpl"}
	r := httptest.NewRequest("GET", "/?peer_id=peer-1", nil)
	info := p.TemplateStoreInfo(r, tpl)

	if !strings.Contains(string(info.PriceLabel), "★") && !strings.Contains(string(info.PriceLabel), "&#9733;") {
		t.Fatalf("owned template should show star, got %s", info.PriceLabel)
	}
}

func TestRemoteCreditTemplateInfoPriced(t *testing.T) {
	p, _ := testCreditProvider(t)

	tpl := StoreMeta{Dir: "priced-tpl"}
	r := httptest.NewRequest("GET", "/?peer_id=peer-1", nil)
	info := p.TemplateStoreInfo(r, tpl)

	if !strings.Contains(string(info.PriceLabel), "50") {
		t.Fatalf("priced template should show price, got %s", info.PriceLabel)
	}
}

func TestRemoteCreditTemplateInfoFree(t *testing.T) {
	p, _ := testCreditProvider(t)

	tpl := StoreMeta{Dir: "something-else"}
	r := httptest.NewRequest("GET", "/?peer_id=peer-1", nil)
	info := p.TemplateStoreInfo(r, tpl)

	if !strings.Contains(string(info.PriceLabel), "Free") {
		t.Fatalf("free template should say Free, got %s", info.PriceLabel)
	}
}

func TestRemoteCreditServiceDown(t *testing.T) {
	p := NewRemoteCreditProvider(
		"http://127.0.0.1:1",
		func(string) string { return "test@test.com" },
		func(string) string { return "" },
		"",
	)

	r := httptest.NewRequest("GET", "/?peer_id=peer-1", nil)
	tpl := StoreMeta{Dir: "test"}

	if !p.TemplateAccessAllowed(r, tpl) {
		t.Fatal("should fail open when credits service is down")
	}

	data := p.StorePageData(r)
	if !strings.Contains(string(data.Banner), "free") {
		t.Fatal("should fall back to free banner when service is down")
	}
}
