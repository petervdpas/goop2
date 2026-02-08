package rendezvous

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// RemoteCreditProvider implements CreditProvider by making HTTP calls
// to a standalone credits service.
type RemoteCreditProvider struct {
	baseURL string
	client  *http.Client
}

// NewRemoteCreditProvider creates a provider that talks to the credits service.
func NewRemoteCreditProvider(baseURL string) *RemoteCreditProvider {
	return &RemoteCreditProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// RegisterRoutes sets up a reverse proxy for /api/credits/* to the credits service.
func (p *RemoteCreditProvider) RegisterRoutes(mux *http.ServeMux) {
	target, err := url.Parse(p.baseURL)
	if err != nil {
		log.Printf("WARNING: invalid credits URL %q: %v", p.baseURL, err)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	mux.HandleFunc("/api/credits/", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})
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
	peerID := r.Header.Get("X-Goop-Peer-ID")
	if peerID == "" {
		peerID = r.URL.Query().Get("peer_id")
	}

	reqURL := fmt.Sprintf("%s/api/credits/access?template_dir=%s", p.baseURL, url.QueryEscape(tpl.Dir))
	if peerID != "" {
		reqURL += "&peer_id=" + url.QueryEscape(peerID)
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
	peerID := r.Header.Get("X-Goop-Peer-ID")
	if peerID == "" {
		peerID = r.URL.Query().Get("peer_id")
	}

	reqURL := p.baseURL + "/api/credits/store-data"
	if peerID != "" {
		reqURL += "?peer_id=" + url.QueryEscape(peerID)
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
		PeerID        string `json:"peer_id"`
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

	var banner template.HTML
	if data.PeerID == "" {
		banner = `<div class="store-banner store-banner-credits">` +
			`Credit system active. Add <code>?peer_id=YOUR_PEER_ID</code> to see your balance. ` +
			`<a href="/credits">🪙 Buy Credits</a>` +
			`</div>`
	} else {
		banner = template.HTML(fmt.Sprintf(
			`<div class="store-banner store-banner-credits">`+
				`<strong>%s</strong> — 🪙 <strong>%d credits</strong> — `+
				`<a href="/credits?peer_id=%s">Buy more</a>`+
				`</div>`,
			template.HTMLEscapeString(data.PeerID), data.Balance,
			url.QueryEscape(data.PeerID)))
	}

	return StorePageData{
		Banner: banner,
	}
}

// TemplateStoreInfo calls the credits service for per-template info and renders HTML locally.
func (p *RemoteCreditProvider) TemplateStoreInfo(r *http.Request, tpl StoreMeta) TemplateStoreInfo {
	peerID := r.Header.Get("X-Goop-Peer-ID")
	if peerID == "" {
		peerID = r.URL.Query().Get("peer_id")
	}

	reqURL := fmt.Sprintf("%s/api/credits/template-info?template_dir=%s", p.baseURL, url.QueryEscape(tpl.Dir))
	if peerID != "" {
		reqURL += "&peer_id=" + url.QueryEscape(peerID)
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

// GrantRegistrationCredits grants initial credits to a newly registered peer.
// It first checks if the peer already has credits (balance > 0) to be idempotent,
// then uses POST /api/credits/purchase to grant the amount.
func (p *RemoteCreditProvider) GrantRegistrationCredits(peerID string, amount int) error {
	if peerID == "" || amount <= 0 {
		return nil
	}

	// Check current balance — skip if peer already has credits
	reqURL := fmt.Sprintf("%s/api/credits/store-data?peer_id=%s", p.baseURL, url.QueryEscape(peerID))
	resp, err := p.client.Get(reqURL)
	if err != nil {
		return fmt.Errorf("balance check: %w", err)
	}
	defer resp.Body.Close()

	var balData struct {
		Balance int `json:"balance"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&balData); err != nil {
		return fmt.Errorf("balance decode: %w", err)
	}
	if balData.Balance > 0 {
		log.Printf("credits: peer %s already has %d credits, skipping grant", peerID, balData.Balance)
		return nil
	}

	// Grant credits via dedicated grant endpoint
	body := fmt.Sprintf(`{"peer_id":%q,"amount":%d,"reason":"registration"}`, peerID, amount)
	purchaseResp, err := p.client.Post(
		p.baseURL+"/api/credits/grant",
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("grant credits: %w", err)
	}
	defer purchaseResp.Body.Close()

	if purchaseResp.StatusCode/100 != 2 {
		respBody, _ := io.ReadAll(purchaseResp.Body)
		return fmt.Errorf("grant credits: status %s: %s", purchaseResp.Status, string(respBody))
	}

	log.Printf("credits: granted %d registration credits to peer %s", amount, peerID)
	return nil
}

func noCreditsStoreData() StorePageData {
	return StorePageData{
		Banner: `<div class="store-banner store-banner-free">All templates on this server are free — no credits needed.</div>`,
	}
}
