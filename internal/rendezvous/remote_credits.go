package rendezvous

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RemoteCreditProvider implements CreditProvider by making HTTP calls
// to a standalone credits service, translating peer_id → email.
type RemoteCreditProvider struct {
	baseURL       string
	adminToken    string
	client        *http.Client
	emailResolver func(string) string // peer_id → email
}

// NewRemoteCreditProvider creates a provider that talks to the credits service.
// The emailResolver translates a peer ID into an email address.
func NewRemoteCreditProvider(baseURL string, emailResolver func(string) string, adminToken string) *RemoteCreditProvider {
	return &RemoteCreditProvider{
		baseURL:       strings.TrimRight(baseURL, "/"),
		adminToken:    adminToken,
		client:        &http.Client{Timeout: 5 * time.Second},
		emailResolver: emailResolver,
	}
}

// resolveEmail extracts peer_id from a request and resolves it to email.
func (p *RemoteCreditProvider) resolveEmail(r *http.Request) string {
	peerID := r.Header.Get("X-Goop-Peer-ID")
	if peerID == "" {
		peerID = r.URL.Query().Get("peer_id")
	}
	if peerID == "" {
		return ""
	}
	return p.emailResolver(peerID)
}

// RegisterRoutes sets up handlers for /api/credits/* that translate peer_id→email
// before forwarding to the credits service.
func (p *RemoteCreditProvider) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/credits/balance", p.proxyBalance)
	mux.HandleFunc("/api/credits/purchase", p.proxyPurchase)
	mux.HandleFunc("/api/credits/grant", p.proxyGrant)
	mux.HandleFunc("/api/credits/spend", p.proxySpend)
	mux.HandleFunc("/api/credits/prices", p.proxyPrices)
	mux.HandleFunc("/api/credits/access", p.proxyAccess)
	mux.HandleFunc("/api/credits/store-data", p.proxyStoreData)
	mux.HandleFunc("/api/credits/template-info", p.proxyTemplateInfo)
}

// proxyBalance translates peer_id→email for GET /api/credits/balance
func (p *RemoteCreditProvider) proxyBalance(w http.ResponseWriter, r *http.Request) {
	email := p.resolveEmail(r)
	if email == "" {
		// Peer has no email — return zero balance instead of error
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"balance": 0})
		return
	}
	p.proxyGet(w, "/api/credits/balance", url.Values{"email": {email}})
}

// proxyPostJSON is the shared handler for POST endpoints that need
// peer_id→email translation. It decodes the request body into reqPtr,
// calls addEmail to merge the resolved email into the outgoing payload,
// then forwards the JSON POST to the credits service at path.
func (p *RemoteCreditProvider) proxyPostJSON(w http.ResponseWriter, r *http.Request, path string, reqPtr any, addEmail func(email string) map[string]any) {
	email := p.resolveEmail(r)
	if err := json.NewDecoder(r.Body).Decode(reqPtr); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if email == "" {
		http.Error(w, "email required — register an email to use credits", http.StatusBadRequest)
		return
	}

	body, _ := json.Marshal(addEmail(email))
	resp, err := p.client.Post(
		p.baseURL+path,
		"application/json",
		strings.NewReader(string(body)),
	)
	if err != nil {
		log.Printf("credits: POST %s failed: %v", path, err)
		http.Error(w, "credits service error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	forwardResponse(w, resp)
}

// proxyPurchase translates peer_id→email for POST /api/credits/purchase
func (p *RemoteCreditProvider) proxyPurchase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Amount int `json:"amount"`
	}
	p.proxyPostJSON(w, r, "/api/credits/purchase", &req, func(email string) map[string]any {
		return map[string]any{"email": email, "amount": req.Amount}
	})
}

// proxyGrant translates peer_id→email for POST /api/credits/grant
func (p *RemoteCreditProvider) proxyGrant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Amount int    `json:"amount"`
		Reason string `json:"reason"`
	}
	p.proxyPostJSON(w, r, "/api/credits/grant", &req, func(email string) map[string]any {
		return map[string]any{"email": email, "amount": req.Amount, "reason": req.Reason}
	})
}

// proxySpend translates peer_id→email for POST /api/credits/spend
func (p *RemoteCreditProvider) proxySpend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Template string `json:"template"`
	}
	p.proxyPostJSON(w, r, "/api/credits/spend", &req, func(email string) map[string]any {
		return map[string]any{"email": email, "template": req.Template}
	})
}

// proxyPrices forwards GET/POST /api/credits/prices unchanged (no peer/email involved).
func (p *RemoteCreditProvider) proxyPrices(w http.ResponseWriter, r *http.Request) {
	var resp *http.Response
	var err error

	switch r.Method {
	case http.MethodGet:
		resp, err = p.client.Get(p.baseURL + "/api/credits/prices")
	case http.MethodPost:
		resp, err = p.client.Post(p.baseURL+"/api/credits/prices", "application/json", r.Body)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err != nil {
		http.Error(w, "credits service error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	forwardResponse(w, resp)
}

// proxyGet forwards a GET request to the credits service with the given
// path and query parameters, then copies the response back to the client.
func (p *RemoteCreditProvider) proxyGet(w http.ResponseWriter, path string, params url.Values) {
	reqURL := p.baseURL + path
	if q := params.Encode(); q != "" {
		reqURL += "?" + q
	}
	resp, err := p.client.Get(reqURL)
	if err != nil {
		log.Printf("credits: GET %s failed: %v", path, err)
		http.Error(w, "credits service error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	forwardResponse(w, resp)
}

// proxyAccess translates peer_id→email for GET /api/credits/access
func (p *RemoteCreditProvider) proxyAccess(w http.ResponseWriter, r *http.Request) {
	params := url.Values{"template_dir": {r.URL.Query().Get("template_dir")}}
	if email := p.resolveEmail(r); email != "" {
		params.Set("email", email)
	}
	p.proxyGet(w, "/api/credits/access", params)
}

// proxyStoreData translates peer_id→email for GET /api/credits/store-data
func (p *RemoteCreditProvider) proxyStoreData(w http.ResponseWriter, r *http.Request) {
	params := url.Values{}
	if email := p.resolveEmail(r); email != "" {
		params.Set("email", email)
	}
	p.proxyGet(w, "/api/credits/store-data", params)
}

// proxyTemplateInfo translates peer_id→email for GET /api/credits/template-info
func (p *RemoteCreditProvider) proxyTemplateInfo(w http.ResponseWriter, r *http.Request) {
	params := url.Values{"template_dir": {r.URL.Query().Get("template_dir")}}
	if email := p.resolveEmail(r); email != "" {
		params.Set("email", email)
	}
	p.proxyGet(w, "/api/credits/template-info", params)
}

// creditsStatus holds the cached status fields from the credits service.
type creditsStatus struct {
	DummyMode  bool
	Version    string
	APIVersion int
}

// fetchStoreStatus fetches status from the credits service.
func (p *RemoteCreditProvider) fetchStoreStatus() creditsStatus {
	resp, err := p.client.Get(p.baseURL + "/api/credits/store-data")
	if err != nil {
		return creditsStatus{}
	}
	defer resp.Body.Close()

	var data struct {
		DummyMode  bool   `json:"dummy_mode"`
		Version    string `json:"version"`
		APIVersion int    `json:"api_version"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	return creditsStatus{
		DummyMode:  data.DummyMode,
		Version:    data.Version,
		APIVersion: data.APIVersion,
	}
}

// TemplateAccessAllowed calls the credits service to check template access.
func (p *RemoteCreditProvider) TemplateAccessAllowed(r *http.Request, tpl StoreMeta) bool {
	email := p.resolveEmail(r)

	reqURL := fmt.Sprintf("%s/api/credits/access?template_dir=%s", p.baseURL, url.QueryEscape(tpl.Dir))
	if email != "" {
		reqURL += "&email=" + url.QueryEscape(email)
	}

	resp, err := p.client.Get(reqURL)
	if err != nil {
		log.Printf("credits: access check error: %v", err)
		return true // fail open
	}
	defer resp.Body.Close()

	var result struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return true // fail open
	}
	return result.Allowed
}

// StorePageData calls the credits service for store page data and renders HTML locally.
func (p *RemoteCreditProvider) StorePageData(r *http.Request) StorePageData {
	email := p.resolveEmail(r)

	reqURL := p.baseURL + "/api/credits/store-data"
	if email != "" {
		reqURL += "?email=" + url.QueryEscape(email)
	}

	resp, err := p.client.Get(reqURL)
	if err != nil {
		log.Printf("credits: store-data error: %v", err)
		return noCreditsStoreData()
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return noCreditsStoreData()
	}

	var data struct {
		CreditsActive bool   `json:"credits_active"`
		Email         string `json:"email"`
		Balance       int    `json:"balance"`
		AppName       string `json:"app_name"`
		CreditPacks   []struct {
			Amount int    `json:"amount"`
			Name   string `json:"name"`
			Label  string `json:"label"`
		} `json:"credit_packs"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return noCreditsStoreData()
	}

	if !data.CreditsActive {
		return noCreditsStoreData()
	}

	// Use the peer_id from the original request for display (user-facing)
	peerID := r.Header.Get("X-Goop-Peer-ID")
	if peerID == "" {
		peerID = r.URL.Query().Get("peer_id")
	}

	var banner template.HTML
	if data.Email == "" {
		banner = `<div class="store-banner store-banner-credits">` +
			`Credit system active. Add <code>?peer_id=YOUR_PEER_ID</code> to the URL to link your account. ` +
			`Copy your Peer ID from <strong>Me → Settings</strong> in your Goop² viewer. ` +
			`<a href="/credits">🪙 Buy Credits</a>` +
			`</div>`
	} else {
		banner = template.HTML(fmt.Sprintf(
			`<div class="store-banner store-banner-credits">`+
				`<strong>%s</strong> — 🪙 <strong>%d credits</strong> — `+
				`<a href="/credits?peer_id=%s">Buy more</a>`+
				`</div>`,
			template.HTMLEscapeString(data.Email), data.Balance,
			url.QueryEscape(peerID)))
	}

	return StorePageData{
		Banner: banner,
	}
}

// TemplateStoreInfo calls the credits service for per-template info and renders HTML locally.
func (p *RemoteCreditProvider) TemplateStoreInfo(r *http.Request, tpl StoreMeta) TemplateStoreInfo {
	email := p.resolveEmail(r)

	reqURL := fmt.Sprintf("%s/api/credits/template-info?template_dir=%s", p.baseURL, url.QueryEscape(tpl.Dir))
	if email != "" {
		reqURL += "&email=" + url.QueryEscape(email)
	}

	resp, err := p.client.Get(reqURL)
	if err != nil {
		log.Printf("credits: template-info error: %v", err)
		return TemplateStoreInfo{PriceLabel: `<span class="tpl-price-free">Free</span>`}
	}
	defer resp.Body.Close()

	var data struct {
		Price  int    `json:"price"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return TemplateStoreInfo{PriceLabel: `<span class="tpl-price-free">Free</span>`}
	}

	switch data.Status {
	case "owned":
		return TemplateStoreInfo{PriceLabel: `<span class="tpl-price-owned">Owned</span>`}
	case "priced":
		return TemplateStoreInfo{
			PriceLabel: template.HTML(fmt.Sprintf(
				`<span class="tpl-price-credits">🪙 %d</span>`, data.Price)),
		}
	default:
		return TemplateStoreInfo{PriceLabel: `<span class="tpl-price-free">Free</span>`}
	}
}

// FetchAccounts fetches all credit accounts from the credits service.
func (p *RemoteCreditProvider) FetchAccounts() (json.RawMessage, error) {
	req, err := http.NewRequest("GET", p.baseURL+"/api/credits/accounts", nil)
	if err != nil {
		return nil, err
	}
	if p.adminToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.adminToken)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("credits service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("credits service returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

// forwardResponse copies the status code, content-type, and body from the
// credits service response to the client.
func forwardResponse(w http.ResponseWriter, resp *http.Response) {
	ct := resp.Header.Get("Content-Type")
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func noCreditsStoreData() StorePageData {
	return StorePageData{
		Banner: `<div class="store-banner store-banner-free">All templates on this server are free — no credits needed.</div>`,
	}
}
