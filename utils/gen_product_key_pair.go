package utils

import (
    "crypto/ecdsa"
    "crypto/elliptic"
    "crypto/rand"
    "crypto/x509"
    "encoding/pem"
    "fmt"
)

// ECDSAKeyPair holds the PEM-encoded private key and public key.
type ECDSAKeyPair struct {
    PrivateKeyPEM []byte // PEM-encoded ECDSA private key (PKCS#8)
    PublicKeyPEM  []byte // PEM-encoded ECDSA public key (PKIX)
}

// GenerateECDSAKeyPair creates a new ECDSA P-256 key pair entirely in Go.
// It returns an ECDSAKeyPair struct containing the PEM-encoded private and public keys.
func GenerateECDSAKeyPair() (*ECDSAKeyPair, error) {
    // 1. Generate a P-256 ECDSA private key
    privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil {
        return nil, fmt.Errorf("failed to generate ECDSA key pair: %w", err)
    }

    // 2. Convert private key to PKCS#8 in PEM format
    privateKeyPEM, err := encodeECPrivateKeyToPEM(privateKey)
    if err != nil {
        return nil, fmt.Errorf("failed to encode private key: %w", err)
    }

    // 3. Convert public key to PKIX in PEM format
    publicKeyPEM, err := encodeECPublicKeyToPEM(&privateKey.PublicKey)
    if err != nil {
        return nil, fmt.Errorf("failed to encode public key: %w", err)
    }

    return &ECDSAKeyPair{
        PrivateKeyPEM: privateKeyPEM,
        PublicKeyPEM:  publicKeyPEM,
    }, nil
}

// encodeECPrivateKeyToPEM marshals an ECDSA private key into PKCS#8 (DER),
// then PEM-encodes it.
func encodeECPrivateKeyToPEM(privateKey *ecdsa.PrivateKey) ([]byte, error) {
    der, err := x509.MarshalPKCS8PrivateKey(privateKey)
    if err != nil {
        return nil, err
    }
    return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// encodeECPublicKeyToPEM marshals an ECDSA public key into PKIX (DER),
// then PEM-encodes it.
func encodeECPublicKeyToPEM(pub *ecdsa.PublicKey) ([]byte, error) {
    der, err := x509.MarshalPKIXPublicKey(pub)
    if err != nil {
        return nil, err
    }
    return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}
