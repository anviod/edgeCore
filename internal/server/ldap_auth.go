package server

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/go-ldap/ldap/v3"
	"go.uber.org/zap"
)

// AuthenticateLDAP attempts to authenticate a user against the configured LDAP server
func (s *Server) AuthenticateLDAP(username, password string) (bool, *model.UserConfig, error) {
	cfg := s.sm.GetConfig()
	ldapCfg := cfg.LDAP

	if !ldapCfg.Enabled {
		return false, nil, fmt.Errorf("LDAP is disabled")
	}

	address := fmt.Sprintf("%s:%d", ldapCfg.Server, ldapCfg.Port)

	// 1. Connect
	var conn *ldap.Conn
	var err error

	if ldapCfg.UseSSL {
		tlsConfig := &tls.Config{InsecureSkipVerify: ldapCfg.SkipVerify}
		conn, err = ldap.DialTLS("tcp", address, tlsConfig)
	} else {
		conn, err = ldap.Dial("tcp", address)
		// Try StartTLS if not using SSL/LDAPS directly, standard port 389 usually supports it
		if err == nil && ldapCfg.Port == 389 {
			// For now, we only do explicit StartTLS if we wanted to enforce it.
			// But usually "UseSSL" implies LDAPS (636).
			// If Port is 389 and UseSSL is false, we proceed with cleartext or let user decide.
		}
	}

	if err != nil {
		zap.L().Error("LDAP Connect Failed", zap.Error(err))
		return false, nil, err
	}
	defer conn.Close()

	conn.SetTimeout(5 * time.Second)

	// 2. Bind (Admin/Service Account)
	if ldapCfg.BindDN != "" && ldapCfg.BindPassword != "" {
		err = conn.Bind(ldapCfg.BindDN, ldapCfg.BindPassword)
		if err != nil {
			zap.L().Error("LDAP Bind Failed", zap.Error(err))
			return false, nil, fmt.Errorf("LDAP configuration error: bind failed")
		}
	} else {
		// Anonymous bind
		err = conn.UnauthenticatedBind("")
		if err != nil {
			zap.L().Error("LDAP Anonymous Bind Failed", zap.Error(err))
			return false, nil, err
		}
	}

	// 3. Search for User
	// Construct filter: e.g. (uid=john)
	if ldapCfg.UserFilter == "" {
		ldapCfg.UserFilter = "(uid=%s)"
	}
	searchFilter := fmt.Sprintf(ldapCfg.UserFilter, username)

	searchRequest := ldap.NewSearchRequest(
		ldapCfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		searchFilter,
		[]string{"dn", "cn", "mail", "uid"}, // attributes to retrieve
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		zap.L().Warn("LDAP User Search Failed", zap.String("username", username), zap.Error(err))
		return false, nil, fmt.Errorf("user not found")
	}

	if len(sr.Entries) != 1 {
		zap.L().Warn("LDAP User Search: User not unique or not found", zap.String("username", username), zap.Int("count", len(sr.Entries)))
		return false, nil, fmt.Errorf("user not found or too many results")
	}

	userDN := sr.Entries[0].DN

	// 4. Bind as User (Verify Password)
	err = conn.Bind(userDN, password)
	if err != nil {
		zap.L().Warn("LDAP User Password Verify Failed", zap.String("username", username))
		return false, nil, fmt.Errorf("invalid credentials")
	}

	// Success
	zap.L().Info("LDAP Authentication Success", zap.String("username", username))

	// Map to internal user model
	// TODO: Map groups to roles if needed. For now, default to admin or restricted.
	// Assuming "admin" for now as this is a gateway.
	user := &model.UserConfig{
		Username: username,
		Role:     "admin",
		// Password is not stored locally
	}

	return true, user, nil
}
