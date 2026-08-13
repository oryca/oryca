package tool

import (
	"crypto/rsa"
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	GrantTypeAccessToken  = "AccessToken"
	GrantTypeRefreshToken = "RefreshToken"
)

type JWTCustomClaims struct {
	Session   bson.ObjectID `json:"ses"`
	UserID    bson.ObjectID `json:"uid"`
	GrantType string        `json:"grt"`
	Role      string        `json:"rol,omitempty"`
	PackageID string        `json:"pkg,omitempty"`
	jwt.RegisteredClaims
}

// ObjectIDHex returns the hex string of a pointer ObjectID, or empty string if nil.
func ObjectIDHex(id *bson.ObjectID) string {
	if id == nil {
		return ""
	}
	return id.Hex()
}

var (
	apiJWTPrivateKey *rsa.PrivateKey
	apiJWTPublicKey  *rsa.PublicKey
)

// InitAPIJWTKeys โหลด + parse RSA key ของ internal JWT ครั้งเดียวตอน startup
// ต้องเรียกก่อน GenerateJWT/ValidateJWT ตัวไหนทั้งนั้น (กัน os.ReadFile + parse PEM ซ้ำทุก request)
func InitAPIJWTKeys(privateKeyPath, publicKeyPath string) error {
	privPEM, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("read jwt private key: %w", err)
	}
	priv, err := jwt.ParseRSAPrivateKeyFromPEM(privPEM)
	if err != nil {
		return fmt.Errorf("parse jwt private key: %w", err)
	}

	pubPEM, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("read jwt public key: %w", err)
	}
	pub, err := jwt.ParseRSAPublicKeyFromPEM(pubPEM)
	if err != nil {
		return fmt.Errorf("parse jwt public key: %w", err)
	}

	apiJWTPrivateKey = priv
	apiJWTPublicKey = pub
	return nil
}

func GenerateJWT(claims *JWTCustomClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(apiJWTPrivateKey)
}

func ValidateJWT(tokenString string) (*JWTCustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTCustomClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return apiJWTPublicKey, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*JWTCustomClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}
