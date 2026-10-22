package testauth

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
	"sync"
	"sync/atomic"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
	"github.com/go-ldap/ldap/v3"
)

type LDAP struct {
	URL         string
	CAFile      string
	listener    net.Listener
	tls         *tls.Config
	mu          sync.Mutex
	connections map[net.Conn]bool
	Filters     []string
	Binds       atomic.Int64
	PlainBinds  atomic.Int64
	wg          sync.WaitGroup
	startTLS    bool
}

func NewLDAP(dir string, startTLS bool) (*LDAP, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Local LDAP fixture"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	pair, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		return nil, err
	}
	ca := filepath.Join(dir, "ldap-fixture-ca.pem")
	if err = os.WriteFile(ca, certPEM, 0600); err != nil {
		return nil, err
	}
	l := &LDAP{CAFile: ca, tls: &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}, connections: map[net.Conn]bool{}, startTLS: startTLS}
	if startTLS {
		l.listener, err = net.Listen("tcp", "127.0.0.1:0")
		l.URL = "ldap://"
	} else {
		l.listener, err = tls.Listen("tcp", "127.0.0.1:0", l.tls)
		l.URL = "ldaps://"
	}
	if err != nil {
		return nil, err
	}
	l.URL += l.listener.Addr().String()
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		for {
			c, e := l.listener.Accept()
			if e != nil {
				return
			}
			l.mu.Lock()
			l.connections[c] = true
			l.mu.Unlock()
			l.wg.Add(1)
			go func() {
				defer l.wg.Done()
				defer func() {
					_ = c.Close()
					l.mu.Lock()
					delete(l.connections, c)
					l.mu.Unlock()
				}()
				l.serve(c)
			}()
		}
	}()
	return l, nil
}
func (l *LDAP) Close() {
	_ = l.listener.Close()
	l.mu.Lock()
	for c := range l.connections {
		_ = c.Close()
	}
	l.mu.Unlock()
	l.wg.Wait()
}
func packet(id int64, op *ber.Packet) *ber.Packet {
	p := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "")
	p.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, id, ""))
	p.AppendChild(op)
	return p
}
func result(id int64, tag ber.Tag, code int64) *ber.Packet {
	p := ber.Encode(ber.ClassApplication, ber.TypeConstructed, tag, nil, "")
	p.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, code, ""))
	p.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", ""))
	p.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", ""))
	return packet(id, p)
}
func (l *LDAP) serve(conn net.Conn) {
	secure := !l.startTLS
	bound := false
	for {
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
		p, err := ber.ReadPacket(conn)
		if err != nil {
			return
		}
		if len(p.Children) < 2 {
			return
		}
		id, ok := p.Children[0].Value.(int64)
		if !ok {
			return
		}
		op := p.Children[1]
		switch op.Tag {
		case 23: // StartTLS extended request
			if len(op.Children) == 0 || op.Children[0].Data.String() != "1.3.6.1.4.1.1466.20037" {
				return
			}
			if _, err = conn.Write(result(id, 24, 0).Bytes()); err != nil {
				return
			}
			tlsConn := tls.Server(conn, l.tls)
			if err = tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			secure = true
		case 0:
			if !secure {
				l.PlainBinds.Add(1)
				return
			}
			l.Binds.Add(1)
			if len(op.Children) != 3 {
				return
			}
			dn := op.Children[1].Data.String()
			password := op.Children[2].Data.String()
			bound = dn == "cn=reader,dc=example,dc=test" && password == "fixture-reader"
			valid := bound || dn == "uid=alice,ou=people,dc=example,dc=test" && password == "fixture-alice" || dn == "uid=bob,ou=people,dc=example,dc=test" && password == "fixture-bob"
			code := int64(49)
			if valid {
				code = 0
			}
			if _, err = conn.Write(result(id, 1, code).Bytes()); err != nil {
				return
			}
		case 3:
			if !bound || len(op.Children) < 8 {
				return
			}
			filter, e := ldap.DecompileFilter(op.Children[6])
			if e != nil {
				return
			}
			l.mu.Lock()
			l.Filters = append(l.Filters, filter)
			l.mu.Unlock()
			name := ""
			if filter == "(uid=alice)" {
				name = "alice"
			}
			if filter == "(uid=bob)" {
				name = "bob"
			}
			if name != "" {
				entry := ber.Encode(ber.ClassApplication, ber.TypeConstructed, 4, nil, "")
				entry.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "uid="+name+",ou=people,dc=example,dc=test", ""))
				attrs := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "")
				for _, attr := range []struct {
					name  string
					value []byte
				}{{"entryUUID", []byte("uuid-" + name)}, {"objectGUID", []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}, {"displayName", []byte(map[string]string{"alice": "Alice Directory", "bob": "Bob Directory"}[name])}, {"mail", []byte(name + "@example.test")}} {
					a := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "")
					a.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, attr.name, ""))
					values := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSet, nil, "")
					values.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, string(attr.value), ""))
					a.AppendChild(values)
					attrs.AppendChild(a)
				}
				entry.AppendChild(attrs)
				if _, err = conn.Write(packet(id, entry).Bytes()); err != nil {
					return
				}
			}
			if _, err = conn.Write(result(id, 5, 0).Bytes()); err != nil {
				return
			}
		case 2:
			return // Unbind
		default:
			_, _ = fmt.Fprint(conn, "")
			return
		}
	}
}
