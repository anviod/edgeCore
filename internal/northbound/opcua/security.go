package opcua

import (
	"crypto/x509"
	"strings"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/awcullen/opcua/server"
	"github.com/awcullen/opcua/ua"
	"go.uber.org/zap"
)

// SecurityCapability describes one endpoint the server may publish.
type SecurityCapability struct {
	Policy      string `json:"policy"`
	Mode        string `json:"mode"`
	Recommended bool   `json:"recommended"`
}

var securePolicyURIs = []string{
	ua.SecurityPolicyURIBasic128Rsa15,
	ua.SecurityPolicyURIBasic256,
	ua.SecurityPolicyURIBasic256Sha256,
	ua.SecurityPolicyURIAes128Sha256RsaOaep,
	ua.SecurityPolicyURIAes256Sha256RsaPss,
}

// SupportedSecurityCapabilities documents endpoint combinations for UI / API consumers.
func SupportedSecurityCapabilities(cfg model.OPCUAConfig) []SecurityCapability {
	out := make([]SecurityCapability, 0, 16)
	if allowSecurityPolicyNone(cfg) {
		out = append(out, SecurityCapability{
			Policy: "None",
			Mode:   "None",
		})
	}
	preferredMode := preferredSecurityMode(cfg)
	for _, uri := range securePolicyURIs {
		if !policyEnabled(cfg, uri) {
			continue
		}
		name := policyNameFromURI(uri)
		out = append(out,
			SecurityCapability{Policy: name, Mode: "Sign"},
			SecurityCapability{
				Policy:      name,
				Mode:        "SignAndEncrypt",
				Recommended: name == "Basic256Sha256" && preferredMode == "SignAndEncrypt",
			},
		)
	}
	return out
}

func appendSecurityOptions(cfg model.OPCUAConfig, opts []server.Option, strictClientPKI bool) []server.Option {
	if hasAuthMethod(cfg, "Anonymous") {
		opts = append(opts, server.WithAuthenticateAnonymousIdentityFunc(func(_ ua.AnonymousIdentity, _ string, _ string) error {
			return nil
		}))
	}

	if hasAuthMethod(cfg, "UserName") {
		opts = append(opts, server.WithAuthenticateUserNameIdentityFunc(func(id ua.UserNameIdentity, _ string, _ string) error {
			return authenticateUserName(cfg, id)
		}))
	}

	if hasAuthMethod(cfg, "Certificate") {
		opts = append(opts, server.WithAuthenticateX509IdentityFunc(authenticateX509Identity))
	}

	// Auto / explicit policy: None endpoint only when allowed; secure policies always enabled with cert.
	opts = append(opts, server.WithSecurityPolicyNone(allowSecurityPolicyNone(cfg)))

	// Clients (UaExpert, Prosys) ship self-signed certs by default; skip PKI unless trusted certs configured.
	if !strictClientPKI {
		opts = append(opts, server.WithInsecureSkipVerify())
	}

	return opts
}

func hasAuthMethod(cfg model.OPCUAConfig, method string) bool {
	if len(cfg.AuthMethods) == 0 {
		return method == "Anonymous"
	}
	for _, m := range cfg.AuthMethods {
		if m == method {
			return true
		}
	}
	return false
}

func allowSecurityPolicyNone(cfg model.OPCUAConfig) bool {
	switch normalizePolicyName(cfg.SecurityPolicy) {
	case "", "auto", "none":
		return true
	default:
		return false
	}
}

func preferredSecurityMode(cfg model.OPCUAConfig) string {
	switch strings.ToLower(strings.TrimSpace(cfg.SecurityMode)) {
	case "sign":
		return "Sign"
	case "none":
		return "None"
	case "signandencrypt", "sign_and_encrypt":
		return "SignAndEncrypt"
	default:
		return "SignAndEncrypt"
	}
}

func normalizePolicyName(policy string) string {
	return strings.ToLower(strings.TrimSpace(policy))
}

func policyEnabled(cfg model.OPCUAConfig, uri string) bool {
	name := normalizePolicyName(cfg.SecurityPolicy)
	switch name {
	case "", "auto":
		return true
	case "none":
		return false
	}
	return strings.EqualFold(policyNameFromURI(uri), policyDisplayName(cfg.SecurityPolicy))
}

func policyDisplayName(policy string) string {
	p := strings.TrimSpace(policy)
	if p == "" {
		return "Auto"
	}
	return p
}

func policyNameFromURI(uri string) string {
	const prefix = "http://opcfoundation.org/UA/SecurityPolicy#"
	return strings.TrimPrefix(uri, prefix)
}

func authenticateUserName(cfg model.OPCUAConfig, id ua.UserNameIdentity) error {
	if len(cfg.Users) == 0 {
		zap.L().Warn("OPC UA username auth rejected: no users configured",
			zap.String("user", id.UserName),
			zap.String("component", "opcua-server"),
		)
		return ua.BadUserAccessDenied
	}
	expected, ok := cfg.Users[id.UserName]
	if !ok || expected != id.Password {
		zap.L().Warn("OPC UA username auth failed",
			zap.String("user", id.UserName),
			zap.String("component", "opcua-server"),
		)
		return ua.BadUserAccessDenied
	}
	return nil
}

func authenticateX509Identity(userIdentity ua.X509Identity, _ string, _ string) error {
	cert, err := parseClientCertificate(userIdentity.Certificate)
	if err != nil {
		zap.L().Error("OPC UA certificate auth failed",
			zap.Error(err),
			zap.String("component", "opcua-server"),
		)
		return ua.BadUserAccessDenied
	}
	zap.L().Info("OPC UA client authenticated via certificate",
		zap.String("subject", cert.Subject.String()),
		zap.String("issuer", cert.Issuer.String()),
		zap.String("component", "opcua-server"),
	)
	return nil
}

func parseClientCertificate(raw ua.ByteString) (*x509.Certificate, error) {
	return x509.ParseCertificate([]byte(raw))
}
