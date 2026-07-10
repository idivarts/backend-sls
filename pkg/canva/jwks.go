package canva

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwksCache caches Canva's public keys (keyed by kid) for return-nav JWT
// verification. Refreshed at most every ttl.
type jwksCache struct {
	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

const jwksTTL = 6 * time.Hour

var jwks = &jwksCache{keys: map[string]*rsa.PublicKey{}}

func (c *jwksCache) get(kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if k, ok := c.keys[kid]; ok && time.Since(c.fetched) < jwksTTL {
		return k, nil
	}
	if err := c.refreshLocked(); err != nil {
		return nil, err
	}
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("canva jwks: unknown kid %q", kid)
}

func (c *jwksCache) refreshLocked() error {
	resp, err := httpClient.Get(jwksURL)
	if err != nil {
		return fmt.Errorf("canva jwks fetch: %w", err)
	}
	defer resp.Body.Close()
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
			Kty string `json:"kty"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("canva jwks decode: %w", err)
	}
	next := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 | int(b)
		}
		if e == 0 {
			e = int(binary.BigEndian.Uint32(pad4(eBytes)))
		}
		next[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	}
	c.keys = next
	c.fetched = time.Now()
	return nil
}

func pad4(b []byte) []byte {
	if len(b) >= 4 {
		return b[len(b)-4:]
	}
	out := make([]byte, 4)
	copy(out[4-len(b):], b)
	return out
}

// ReturnClaims are the fields we consume from a verified return-navigation JWT.
type ReturnClaims struct {
	DesignID         string
	CorrelationState string
}

// VerifyReturnJWT verifies a Canva return-navigation JWT against the cached
// JWKS and returns its claims. Rejects wrong alg / bad signature / expired.
func VerifyReturnJWT(tokenString string) (*ReturnClaims, error) {
	parsed, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		return jwks.get(kid)
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		return nil, fmt.Errorf("canva return jwt: %w", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		return nil, fmt.Errorf("canva return jwt: invalid claims")
	}
	rc := &ReturnClaims{}
	if v, ok := claims["design_id"].(string); ok {
		rc.DesignID = v
	}
	if v, ok := claims["correlation_state"].(string); ok {
		rc.CorrelationState = v
	}
	return rc, nil
}
