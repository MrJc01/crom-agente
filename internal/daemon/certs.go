package daemon

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// CertManager lida com a criacao e carregamento de certificados mTLS
type CertManager struct {
	CertsDir   string
	CAKey      *ecdsa.PrivateKey
	CACert     *x509.Certificate
	ServerKey  *ecdsa.PrivateKey
	ServerCert *x509.Certificate
	ClientKey  *ecdsa.PrivateKey
	ClientCert *x509.Certificate
}

func NewCertManager() (*CertManager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	certsDir := filepath.Join(home, ".crom", "certs")
	
	err = os.MkdirAll(certsDir, 0700)
	if err != nil {
		return nil, err
	}

	cm := &CertManager{CertsDir: certsDir}
	err = cm.initOrLoadCerts()
	return cm, err
}

func (cm *CertManager) initOrLoadCerts() error {
	caCertPath := filepath.Join(cm.CertsDir, "ca.crt")
	caKeyPath := filepath.Join(cm.CertsDir, "ca.key")
	serverCertPath := filepath.Join(cm.CertsDir, "server.crt")
	serverKeyPath := filepath.Join(cm.CertsDir, "server.key")
	clientCertPath := filepath.Join(cm.CertsDir, "client.crt")
	clientKeyPath := filepath.Join(cm.CertsDir, "client.key")

	// Verifica se a CA ja existe
	if _, err := os.Stat(caCertPath); os.IsNotExist(err) {
		// Gera tudo do zero
		if err := cm.generateAll(caCertPath, caKeyPath, serverCertPath, serverKeyPath, clientCertPath, clientKeyPath); err != nil {
			return err
		}
	} else {
		// CA existe, assumimos que os outros também existem (para fins de simplicidade, ignoramos a rotacao auto)
		// Aqui poderiamos carregar as structs parsadas se fossemos usar para assinar novos clients
	}
	return nil
}

func (cm *CertManager) generateAll(caCertPath, caKeyPath, srvCertPath, srvKeyPath, cliCertPath, cliKeyPath string) error {
	// 1. Gera CA
	caPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Crom Agente Local CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 anos
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return err
	}
	if err := saveCert(caCertPath, caBytes); err != nil {
		return err
	}
	if err := saveKey(caKeyPath, caPrivKey); err != nil {
		return err
	}

	// 2. Gera Servidor
	srvPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	srvTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	srvBytes, err := x509.CreateCertificate(rand.Reader, srvTemplate, caTemplate, &srvPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return err
	}
	if err := saveCert(srvCertPath, srvBytes); err != nil {
		return err
	}
	if err := saveKey(srvKeyPath, srvPrivKey); err != nil {
		return err
	}

	// 3. Gera Cliente
	cliPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	cliTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "Crom CLI Client"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	cliBytes, err := x509.CreateCertificate(rand.Reader, cliTemplate, caTemplate, &cliPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return err
	}
	if err := saveCert(cliCertPath, cliBytes); err != nil {
		return err
	}
	if err := saveKey(cliKeyPath, cliPrivKey); err != nil {
		return err
	}

	return nil
}

func saveCert(path string, certBytes []byte) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	return pem.Encode(out, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
}

func saveKey(path string, key *ecdsa.PrivateKey) error {
	b, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	return pem.Encode(out, &pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
}

// GetServerTLSConfig retorna a configuracao TLS mTLS para o gRPC Server
func (cm *CertManager) GetServerTLSConfig() (*tls.Config, error) {
	serverCertPath := filepath.Join(cm.CertsDir, "server.crt")
	serverKeyPath := filepath.Join(cm.CertsDir, "server.key")
	caCertPath := filepath.Join(cm.CertsDir, "ca.crt")

	// Carrega certificado do servidor
	cert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		return nil, fmt.Errorf("falha ao carregar cert servidor: %v", err)
	}

	// Carrega CA para validar clientes
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler CA: %v", err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("falha ao parsear CA")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12,
	}

	return tlsConfig, nil
}
