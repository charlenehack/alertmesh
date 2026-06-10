// Package middleware: WebhookSignatureFilter implements RFC 9421 HTTP Message
// Signatures verification for the /api/v1/alerts/webhook/{source} endpoint.
//
// Each external webhook source owns an Ed25519 keypair (managed in the
// `webhook_sources` table). The private key is sent to the source operator
// only once at create / rotate time; only the PEM-encoded PKIX public key is
// stored. Inbound POSTs are required to carry an RFC 9421 signature whose
// `keyid` matches `webhook_sources.client_id` and which covers the fields:
//
//	@method  @request-target  @authority  content-digest  created  nonce
//
// Replay protection is enforced in two layers:
//
//  1. The signature's `created` parameter must be within ±row.AllowSkew
//     seconds (configurable per source, default 300).
//  2. The `nonce` parameter must not have been seen recently. Phase 1 keeps
//     a per-process sync.Map with a TTL twice the allow-skew window; Phase 3
//     can swap this for Redis SETNX without touching call sites.
//
// Any verification failure (bad signature / unknown client / clock skew /
// replay) returns 401 with no retry hint, matching safety.md section B.1.
package middleware

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	restful "github.com/emicklei/go-restful/v3"
	"github.com/rs/zerolog/log"
	"github.com/yaronf/httpsign"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/model"
)

// signatureName is the value used in `Signature-Input: <name>=...` headers by
// the external client. Documented in the README so source operators sign with
// the same name.
const signatureName = "alertmesh"

// requiredFields are the RFC 9421 signature components that MUST be covered
// by the client's signature. Matches safety.md Plan B step 2.
var requiredFields = httpsign.Headers(
	"@method",
	"@request-target",
	"@authority",
	"content-digest",
)

// nonceCache is a small in-process replay cache. Keys are formatted as
// "<client_id>:<nonce>"; values are the absolute expiry time. A single
// background goroutine evicts expired keys every minute.
type nonceCache struct {
	m sync.Map // map[string]time.Time
}

func newNonceCache() *nonceCache {
	c := &nonceCache{}
	go c.gcLoop()
	return c
}

// SetNX returns false when the nonce is already known; otherwise it inserts
// it with the supplied TTL and returns true.
func (c *nonceCache) SetNX(key string, ttl time.Duration) bool {
	expires := time.Now().Add(ttl)
	if existing, loaded := c.m.LoadOrStore(key, expires); loaded {
		if exp, ok := existing.(time.Time); ok && time.Now().Before(exp) {
			return false
		}
		c.m.Store(key, expires)
	}
	return true
}

func (c *nonceCache) gcLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.m.Range(func(k, v interface{}) bool {
			if exp, ok := v.(time.Time); ok && now.After(exp) {
				c.m.Delete(k)
			}
			return true
		})
	}
}

// WebhookSignatureFilter returns a restful.FilterFunction that gates routes
// whose `kind` metadata equals "AlertWebhook" on a valid RFC 9421 signature
// matching one of the rows in `webhook_sources`. All other routes pass
// through untouched.
func WebhookSignatureFilter(db *gorm.DB) restful.FilterFunction {
	cache := newNonceCache()
	return func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
		route := req.SelectedRoute()
		if route == nil || metaString(route.Metadata(), "kind") != "AlertWebhook" {
			chain.ProcessFilter(req, resp)
			return
		}

		source := req.PathParameter("source")
		if source == "" {
			rejectSig(resp, "missing source path parameter")
			return
		}

		var row model.WebhookSource
		err := db.WithContext(req.Request.Context()).
			Where("name = ? AND is_enabled = ?", source, true).
			First(&row).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				rejectSig(resp, "unknown or disabled webhook source: "+source)
			} else {
				log.Warn().Err(err).Str("source", source).Msg("webhook signature: source lookup failed")
				rejectSig(resp, "webhook source lookup failed")
			}
			return
		}

		pubKey, err := parseEd25519PublicKey(row.PublicKey)
		if err != nil {
			log.Error().Err(err).Str("source", source).Msg("webhook signature: invalid stored public key")
			rejectSig(resp, "invalid public key for source")
			return
		}

		skew := time.Duration(row.AllowSkew) * time.Second
		if skew <= 0 {
			skew = 300 * time.Second
		}

		cfg := httpsign.NewVerifyConfig().
			SetKeyID(row.ClientID).
			SetAllowedAlgs([]string{"ed25519"}).
			SetVerifyCreated(true).
			SetNotNewerThan(skew).
			SetNotOlderThan(skew)

		// 手动提取 nonce 并做防重放校验（替代 v0.5.x 的 SetNonceValidator）
		nonce := extractNonce(req.Request)
		if nonce == "" {
			rejectSig(resp, "nonce required")
			return
		}
		nonceKey := row.ClientID + ":" + nonce
		if !cache.SetNX(nonceKey, skew*2) {
			rejectSig(resp, "nonce already seen (replay)")
			return
		}

		verifier, err := httpsign.NewEd25519Verifier(pubKey, cfg, requiredFields)
		if err != nil {
			log.Error().Err(err).Msg("webhook signature: verifier construction failed")
			rejectSig(resp, "verifier construction failed")
			return
		}

		if err := httpsign.VerifyRequest(signatureName, *verifier, req.Request); err != nil {
			log.Warn().
				Err(err).
				Str("source", source).
				Str("client_id", row.ClientID).
				Msg("webhook signature: verification failed")
			rejectSig(resp, "signature verification failed")
			return
		}

		// Best-effort touch of last_used_at; failures must not block delivery.
		go func(id string) {
			if updErr := db.Model(&model.WebhookSource{}).
				Where("id = ?", id).
				Update("last_used_at", time.Now()).Error; updErr != nil {
				log.Debug().Err(updErr).Str("id", id).Msg("webhook signature: last_used_at update failed")
			}
		}(row.ID)

		chain.ProcessFilter(req, resp)
	}
}

// rejectSig emits a 401 with a generic body and signals "do not retry" via
// the WWW-Authenticate header (mirrors Woodpecker's backoff.Permanent path).
func rejectSig(resp *restful.Response, detail string) {
	resp.AddHeader("WWW-Authenticate", `Signature realm="alertmesh-webhook"`)
	_ = resp.WriteErrorString(http.StatusUnauthorized, fmt.Sprintf("unauthorized: %s", detail))
}

// parseEd25519PublicKey decodes a PEM-encoded PKIX public key into an
// ed25519.PublicKey, rejecting non-Ed25519 keys explicitly.
func parseEd25519PublicKey(pemStr string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemStr)))
	if block == nil {
		return nil, errors.New("public key is not PEM-encoded")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKIX public key: %w", err)
	}
	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("public key is not Ed25519")
	}
	return edPub, nil
}

// metaString returns a string-typed metadata value or "".
func metaString(meta map[string]interface{}, key string) string {
	if v, ok := meta[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// extractNonce parses the nonce parameter from the Signature-Input header.
// Format: alertmesh=("@method" ...);nonce="xxx";created=...
func extractNonce(r *http.Request) string {
	sigInput := r.Header.Get("Signature-Input")
	for _, part := range strings.Split(sigInput, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "nonce=") {
			return strings.Trim(strings.TrimPrefix(part, "nonce="), `"`)
		}
	}
	return ""
}
