package identity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Setup links let a person choose their own local-account password, so an
// administrator never sets or sees one. A link is single use, expires, and
// is the only live link for its user. The token travels in the URL fragment
// (/setup#TOKEN), which browsers never send to servers or proxies.
const (
	setupKind     = "local_setup"
	setupIndex    = "local_setup_user"
	setupLifetime = 72 * time.Hour
)

type setupGrant struct {
	UserID  string    `json:"userId"`
	Expires time.Time `json:"expiresAt"`
}

type setupPointer struct {
	TokenHash string `json:"tokenHash"`
}

// SetupLink is shown once to the administrator, who sends it to the person.
type SetupLink struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// issueSetupTx replaces the user's setup link with a new one.
func (s *Service) issueSetupTx(tx Tx, a Actor, u User) (SetupLink, error) {
	if previous, err := read[setupPointer](tx, setupIndex, "global", u.ID); err == nil {
		if err = tx.Delete(setupKind, "global", previous.TokenHash); err != nil {
			return SetupLink{}, err
		}
	} else if !errors.Is(err, ErrNotFound) {
		return SetupLink{}, err
	}
	token := randomToken()
	expires := s.Now().UTC().Add(setupLifetime)
	if err := put(tx, setupKind, "global", hash(token), setupGrant{UserID: u.ID, Expires: expires}, expires); err != nil {
		return SetupLink{}, err
	}
	if err := put(tx, setupIndex, "global", u.ID, setupPointer{TokenHash: hash(token)}, expires); err != nil {
		return SetupLink{}, err
	}
	if err := s.recordPlatform(tx, a, "user.setup_link", u.ID, "succeeded", map[string]any{"expiresAt": expires}); err != nil {
		return SetupLink{}, err
	}
	return SetupLink{URL: s.config.PublicURL + "/setup#" + token, ExpiresAt: expires}, nil
}

// IssueSetupLink gives an existing local user a new setup link, replacing
// any earlier one. Using it sets a new password and signs them out
// everywhere, so it also serves as a password reset.
func (s *Service) IssueSetupLink(ctx context.Context, a Actor, userID string) (SetupLink, error) {
	var link SetupLink
	err := s.repo.Update(ctx, "user:"+userID, func(tx Tx) error {
		if err := s.platformAdminTx(tx, a); err != nil {
			return err
		}
		u, err := read[User](tx, "user", "global", userID)
		if err != nil {
			return err
		}
		if p, ok := s.providers[u.Provider]; !ok || p.config.Type != "local" {
			return fmt.Errorf("setup links are for local accounts; this person signs in through %s: %w", u.Provider, ErrInvalid)
		}
		link, err = s.issueSetupTx(tx, a, u)
		return err
	})
	return link, err
}

// CompleteSetup sets the password of the link's user and spends the link.
func (s *Service) CompleteSetup(ctx context.Context, token, password string) error {
	if !validOpaque(token) {
		return ErrForbidden
	}
	h, err := passwordHash(password)
	if err != nil {
		return fmt.Errorf("choose a password of 12 to 72 characters: %w", ErrInvalid)
	}
	key := hash(token)
	return s.repo.Update(ctx, "setup:"+key, func(tx Tx) error {
		grant, err := read[setupGrant](tx, setupKind, "global", key)
		if errors.Is(err, ErrNotFound) || err == nil && !grant.Expires.After(s.Now()) {
			return ErrForbidden
		}
		if err != nil {
			return err
		}
		u, err := read[User](tx, "user", "global", grant.UserID)
		if err != nil {
			return err
		}
		credential, err := read[localCredential](tx, "local_password", "global", u.ID)
		if err != nil || credential.Disabled {
			return ErrForbidden
		}
		// A new password ends every existing session.
		u.Epoch++
		u.Version++
		if err = put(tx, "local_password", "global", u.ID, localCredential{Hash: h}, time.Time{}); err != nil {
			return err
		}
		if err = put(tx, "user", "global", u.ID, u, time.Time{}); err != nil {
			return err
		}
		if err = tx.Delete(setupKind, "global", key); err != nil {
			return err
		}
		if err = tx.Delete(setupIndex, "global", u.ID); err != nil {
			return err
		}
		self := Actor{ID: u.ID, Name: u.Name, Provider: u.Provider, Kind: "user", platformOperation: true}
		return s.recordPlatform(tx, self, "user.setup_completed", u.ID, "succeeded", map[string]any{"via": "setup link"})
	})
}

// setupHandler serves POST /auth/local/setup.
func (s *Service) setupHandler(w http.ResponseWriter, r *http.Request) {
	if !s.rateLogin(w, r) {
		return
	}
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &in); err != nil {
		authFailure(w, err)
		return
	}
	err := s.CompleteSetup(r.Context(), in.Token, in.Password)
	in.Password = ""
	if errors.Is(err, ErrForbidden) {
		authJSON(w, 403, map[string]string{"error": "This setup link has expired or was already used. Ask your administrator for a new one."})
		return
	}
	if err != nil {
		authFailure(w, err)
		return
	}
	authJSON(w, 200, StatusResult{Status: "password_set"})
}

// UserCreated is a new local account, with its setup link when the
// administrator did not set a password.
type UserCreated struct {
	User
	SetupLink *SetupLink `json:"setupLink,omitempty"`
}

func (s *Service) validLocalUser(in LocalUserInput) bool {
	p, ok := s.providers[in.Provider]
	return ok && p.config.Type == "local" && localUsername.MatchString(in.Username) && in.Name != "" && len(in.Name) <= 300 && len(in.Email) <= 320 && validRole(in.Role, true) && in.ExpectedVersion == 0
}

// CreatePlatformLocalUser creates a local account with a role in a tenant.
// Without a password it is pending until the person uses the returned setup
// link to choose one.
func (s *Service) CreatePlatformLocalUser(ctx context.Context, a Actor, tenant string, in LocalUserInput) (UserCreated, error) {
	if in.Password != "" {
		u, err := s.CreateLocalUser(ctx, a, tenant, in)
		return UserCreated{User: u}, err
	}
	if !s.validLocalUser(in) {
		return UserCreated{}, ErrInvalid
	}
	u := s.user(in.Provider, externalUser{Subject: in.Username, Name: in.Name, Email: in.Email})
	u.Version = 1
	var link SetupLink
	err := s.repo.Update(ctx, "tenant:"+tenant, func(tx Tx) error {
		if e := s.checkOwner(tx, tenant, a); e != nil {
			return e
		}
		if e := s.createLocalUserTx(tx, a, tenant, u, nil, in.Role); e != nil {
			return e
		}
		var e error
		link, e = s.issueSetupTx(tx, a, u)
		return e
	})
	return UserCreated{User: u, SetupLink: &link}, err
}
