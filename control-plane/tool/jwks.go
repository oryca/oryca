package tool

import (
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"sort"
)

type JWK struct {
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

// GenerateJWKS แปลง public keys เป็น JWK Set (เรียง kid ให้ output คงที่)
func GenerateJWKS(pubKeys map[string]*rsa.PublicKey) JWKS {
	kids := make([]string, 0, len(pubKeys))
	for kid := range pubKeys {
		kids = append(kids, kid)
	}
	sort.Strings(kids)

	keys := make([]JWK, 0, len(kids))
	for _, kid := range kids {
		key := pubKeys[kid]
		keys = append(keys, JWK{
			Kty: "RSA",
			Alg: "RS256",
			Use: "sig",
			Kid: kid,
			N:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		})
	}
	return JWKS{Keys: keys}
}
