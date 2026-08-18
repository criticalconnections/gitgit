package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"

	"golang.org/x/crypto/ssh"
)

// generateHostKeyPEM makes the server's permanent ed25519 identity.
func generateHostKeyPEM() ([]byte, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block, err := ssh.MarshalPrivateKey(priv, "gitgit host key")
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(block), nil
}
