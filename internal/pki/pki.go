package pki

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type Authority struct {
	dir      string
	ca       *x509.Certificate
	caKey    ed25519.PrivateKey
	certPool *x509.CertPool
}

type Bundle struct {
	CertificatePEM string `json:"certificate_pem"`
	PrivateKeyPEM  string `json:"private_key_pem"`
	CAPEM          string `json:"ca_pem"`
}

type CredentialFiles struct {
	Certificate []byte
	PrivateKey  []byte
	CA          []byte
}

func Ensure(dataDir, publicName string) (*Authority, Bundle, error) {
	dir := filepath.Join(dataDir, "pki")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, Bundle{}, err
	}
	authority, err := loadOrCreateAuthority(dir)
	if err != nil {
		return nil, Bundle{}, err
	}
	serverCertPath := filepath.Join(dir, "host.crt")
	serverKeyPath := filepath.Join(dir, "host.key")
	if _, err := os.Stat(serverCertPath); os.IsNotExist(err) {
		cert, key, err := authority.issueServer(publicName)
		if err != nil {
			return nil, Bundle{}, err
		}
		if err := writePair(serverCertPath, serverKeyPath, cert, key); err != nil {
			return nil, Bundle{}, err
		}
	}
	serverCert, err := os.ReadFile(serverCertPath)
	if err != nil {
		return nil, Bundle{}, err
	}
	serverKey, err := os.ReadFile(serverKeyPath)
	if err != nil {
		return nil, Bundle{}, err
	}
	caPEM, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return nil, Bundle{}, err
	}
	return authority, Bundle{CertificatePEM: string(serverCert), PrivateKeyPEM: string(serverKey), CAPEM: string(caPEM)}, nil
}

func (a *Authority) ClientPool() *x509.CertPool {
	return a.certPool
}

func (a *Authority) IssueInstance(id string) (Bundle, error) {
	files, err := a.IssueInstanceFiles(id)
	if err != nil {
		return Bundle{}, err
	}
	return Bundle{CertificatePEM: string(files.Certificate), PrivateKeyPEM: string(files.PrivateKey), CAPEM: string(files.CA)}, nil
}

func (a *Authority) IssueInstanceFiles(id string) (CredentialFiles, error) {
	uri, err := url.Parse("spiffe://ulcer/instance/" + id)
	if err != nil {
		return CredentialFiles{}, err
	}
	template := &x509.Certificate{
		SerialNumber: randomSerial(),
		Subject:      pkix.Name{CommonName: "ulcer-instance-" + id},
		NotBefore:    time.Now().Add(-5 * time.Minute),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs:         []*url.URL{uri},
	}
	cert, key, err := a.issue(template)
	if err != nil {
		return CredentialFiles{}, err
	}
	caPEM, err := os.ReadFile(filepath.Join(a.dir, "ca.crt"))
	if err != nil {
		return CredentialFiles{}, err
	}
	return CredentialFiles{Certificate: cert, PrivateKey: key, CA: caPEM}, nil
}

func loadOrCreateAuthority(dir string) (*Authority, error) {
	certPath := filepath.Join(dir, "ca.crt")
	keyPath := filepath.Join(dir, "ca.key")
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		template := &x509.Certificate{
			SerialNumber:          randomSerial(),
			Subject:               pkix.Name{CommonName: "ulcer local instance authority"},
			NotBefore:             time.Now().Add(-5 * time.Minute),
			NotAfter:              time.Now().AddDate(10, 0, 0),
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
			MaxPathLen:            0,
			MaxPathLenZero:        true,
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
		if err != nil {
			return nil, err
		}
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
		if err != nil {
			return nil, err
		}
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		if err := writePair(certPath, keyPath, certPEM, keyPEM); err != nil {
			return nil, err
		}
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	certBlock, _ := pem.Decode(certPEM)
	keyBlock, _ := pem.Decode(keyPEM)
	if certBlock == nil || keyBlock == nil {
		return nil, fmt.Errorf("invalid authority PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := keyAny.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("authority key is not ed25519")
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &Authority{dir: dir, ca: cert, caKey: key, certPool: pool}, nil
}

func (a *Authority) issueServer(publicName string) ([]byte, []byte, error) {
	template := &x509.Certificate{
		SerialNumber: randomSerial(),
		Subject:      pkix.Name{CommonName: publicName},
		NotBefore:    time.Now().Add(-5 * time.Minute),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if address := net.ParseIP(publicName); address != nil {
		template.IPAddresses = []net.IP{address}
	} else {
		template.DNSNames = []string{publicName}
	}
	return a.issue(template)
}

func (a *Authority) issue(template *x509.Certificate) ([]byte, []byte, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.CreateCertificate(rand.Reader, template, a.ca, publicKey, a.caKey)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
}

func writePair(certPath, keyPath string, certPEM, keyPEM []byte) error {
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return err
	}
	return os.WriteFile(keyPath, keyPEM, 0o600)
}

func randomSerial() *big.Int {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		panic(err)
	}
	return serial
}
