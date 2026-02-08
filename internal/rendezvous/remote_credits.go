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

// IsDummyMode queries the credits service to check if it's running in dummy mode.
func (p *RemoteCreditProvider) IsDummyMode() bool {
	resp, err := p.client.Get(p.baseURL + "/api/credits/store-data")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var data struct {
		DummyMode bool `json:"dummy_mode"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	return data.DummyMode
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
			`Credit system active. Add <code>?peer_id=YOUR_PEER_ID</code> to see your balance.` +
			`</div>`
	} else {
		banner = template.HTML(fmt.Sprintf(
			`<div class="store-banner store-banner-credits">`+
				`<strong>%s</strong> — Balance: <strong>%d credits</strong>`+
				`</div>`,
			template.HTMLEscapeString(data.PeerID), data.Balance))
	}

	packs := template.HTML(`<div class="credit-packs">` +
		`<h3>Buy Credits</h3>` +
		`<div class="credit-pack-grid">` +
		creditPackButton(100, "Starter Pack", "100 credits") +
		creditPackButton(500, "Pro Pack", "500 credits") +
		creditPackButton(1000, "Power Pack", "1000 credits") +
		`</div></div>`)

	return StorePageData{
		Banner:      banner,
		CreditPacks: packs,
	}
}

func creditPackButton(amount int, name, label string) string {
	return fmt.Sprintf(
		`<div class="credit-pack">`+
			`<div class="credit-pack-name">%s</div>`+
			`<div class="credit-pack-amount">%s</div>`+
			`<button class="credit-pack-buy" onclick="buyCredits(%d)">Buy</button>`+
			`</div>`,
		template.HTMLEscapeString(name),
		template.HTMLEscapeString(label),
		amount)
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
				`<span class="tpl-price-credits">%d credits</span>`, data.Price)),
		}
	default:
		return TemplateStoreInfo{PriceLabel: `<span class="tpl-price-free">Free</span>`}
	}
}

func noCreditsStoreData() StorePageData {
	return StorePageData{
		Banner: `<div class="store-banner store-banner-free">All templates on this server are free — no credits needed.</div>`,
	}
}
