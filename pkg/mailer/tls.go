package mailer

import "crypto/tls"

// tlsConfig verifies the server certificate: a STARTTLS upgrade that skipped
// verification would only hide the traffic from nobody.
func tlsConfig(host string) *tls.Config {
	return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
}
