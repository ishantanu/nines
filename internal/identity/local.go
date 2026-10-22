package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ishantanu/nines/internal/tenancy"
)

var localUsername = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._@+-]{0,127}$`)

// A fixed dummy hash keeps unknown-user password checks on the bcrypt path.
var dummyLocalHash = []byte("$2a$12$R9h/cIPz0gi.URNNX3kh2OPST9/PgBkqquzi.Ss7KIUgO2t0jWMUW")

func localJSON(value localCredential) json.RawMessage { b, _ := json.Marshal(value); return b }

type localCredential struct {
	Hash     []byte `json:"hash"`
	Disabled bool   `json:"disabled"`
}

func passwordHash(password string) ([]byte, error) {
	if len(password) < 12 || len(password) > 72 {
		return nil, ErrInvalid
	}
	return bcrypt.GenerateFromPassword([]byte(password), 12)
}

// demoPassword is limited to the explicitly enabled loopback playground account.
// Ordinary users and persistent deployments keep the normal password policy.
func demoPassword(p ProviderConfig, username, password string) bool {
	return p.AllowDemoCredentials && p.Type == "local" && p.BootstrapUsername == "admin" && username == "admin" && password == "admin"
}

func localPasswordHash(p ProviderConfig, username, password string) ([]byte, error) {
	if demoPassword(p, username, password) {
		return bcrypt.GenerateFromPassword([]byte(password), 12)
	}
	return passwordHash(password)
}

func (s *Service) updatedPasswordHash(id, password string) ([]byte, error) {
	for _, p := range s.config.Providers {
		if demoPassword(p, p.BootstrapUsername, password) && s.user(p.ID, externalUser{Subject: p.BootstrapUsername}).ID == id {
			return localPasswordHash(p, p.BootstrapUsername, password)
		}
	}
	return passwordHash(password)
}

func (s *Service) bootstrapLocal(ctx context.Context) error {
	for _, p := range s.config.Providers {
		if p.Type != "local" || p.BootstrapUsername == "" {
			continue
		}
		u := s.user(p.ID, externalUser{Subject: p.BootstrapUsername})
		var exists bool
		if err := s.repo.View(ctx, func(tx Tx) error {
			_, e := tx.Get("local_password", "global", u.ID)
			exists = e == nil
			if errors.Is(e, ErrNotFound) {
				return nil
			}
			return e
		}); err != nil {
			return err
		}
		if exists {
			continue
		}
		value, err := localPasswordHash(p, p.BootstrapUsername, p.secret)
		if err != nil {
			return err
		}
		if err = s.repo.Update(ctx, "user:"+u.ID, func(tx Tx) error {
			if err := tx.Ensure("local_password", "global", u.ID, localJSON(localCredential{Hash: value})); err != nil {
				return err
			}
			existing, e := read[User](tx, "user", "global", u.ID)
			if errors.Is(e, ErrNotFound) {
				existing = u
			} else if e != nil {
				return e
			}
			if existing.Version == 0 {
				existing.Version = 1
			}
			return put(tx, "user", "global", u.ID, existing, time.Time{})
		}); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) localLogin(w http.ResponseWriter, r *http.Request) {
	p, ok := s.providers[r.PathValue("provider")]
	if !ok || p.config.Type != "local" {
		authFailure(w, ErrNotFound)
		return
	}
	login, err := s.cookie(r, "login")
	if err != nil || !equal(r.Header.Get("X-CSRF-Token"), hash("login:"+login)) {
		authFailure(w, ErrForbidden)
		return
	}
	if !s.rateLogin(w, r) {
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if decode(w, r, &in) != nil {
		authFailure(w, ErrInvalid)
		return
	}
	select {
	case s.loginSlots <- struct{}{}:
		defer func() { <-s.loginSlots }()
	default:
		authJSON(w, 429, map[string]string{"error": "Sign-in is busy. Try again shortly."})
		return
	}
	user, err := s.authenticateLocal(r.Context(), p.config.ID, in.Username, in.Password)
	in.Password = ""
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			authJSON(w, 401, map[string]string{"error": "Sign-in failed. Check your credentials or contact your administrator."})
		} else {
			authFailure(w, err)
		}
		return
	}
	s.finishLogin(w, r, p.config.ID, user, false)
}

type LocalUserInput struct {
	Provider        string `json:"provider"`
	Username        string `json:"username"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	Role            string `json:"role"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (s *Service) CreateLocalUser(ctx context.Context, a Actor, tenant string, in LocalUserInput) (User, error) {
	p, ok := s.providers[in.Provider]
	if !ok || p.config.Type != "local" || !localUsername.MatchString(in.Username) || strings.TrimSpace(in.Name) == "" || len(in.Name) > 300 || len(in.Email) > 320 || !validRole(in.Role, true) || in.ExpectedVersion != 0 {
		return User{}, ErrInvalid
	}
	h, err := passwordHash(in.Password)
	if err != nil {
		return User{}, err
	}
	u := s.user(in.Provider, externalUser{Subject: in.Username, Name: in.Name, Email: in.Email})
	u.Version = 1
	err = s.repo.Update(ctx, "tenant:"+tenant, func(tx Tx) error {
		if e := s.checkOwner(tx, tenant, a); e != nil {
			return e
		}
		if _, e := tx.Get("user", "global", u.ID); e == nil {
			return ErrConflict
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		if _, e := tx.Get("local_password", "global", u.ID); e == nil {
			return ErrConflict
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		if e := tx.Ensure("local_password", "global", u.ID, localJSON(localCredential{Hash: h})); e != nil {
			return e
		}
		current, e := read[localCredential](tx, "local_password", "global", u.ID)
		if e != nil {
			return e
		}
		if !equal(string(current.Hash), string(h)) {
			return ErrConflict
		}
		if e = put(tx, "user", "global", u.ID, u, time.Time{}); e != nil {
			return e
		}
		if e = s.saveMember(tx, tenant, Member{UserID: u.ID, Role: in.Role}); e != nil {
			return e
		}
		return s.recompute(tx, tenant, u.ID)
	})
	return u, err
}
func (s *Service) UpdateLocalUser(ctx context.Context, a Actor, tenant, id string, in LocalUserInput) (User, error) {
	var out User
	if in.ExpectedVersion < 1 || strings.TrimSpace(in.Name) == "" || len(in.Name) > 300 || len(in.Email) > 320 || in.Username != "" || in.Provider != "" || in.Role != "" {
		return out, ErrInvalid
	}
	var h []byte
	var err error
	if in.Password != "" {
		h, err = s.updatedPasswordHash(id, in.Password)
		if err != nil {
			return out, err
		}
	}
	err = s.repo.Update(ctx, "user:"+id, func(tx Tx) error {
		if e := s.checkOwner(tx, tenant, a); e != nil {
			return e
		}
		if _, e := tx.Get("member", tenant, id); e != nil {
			return ErrForbidden
		}
		u, e := read[User](tx, "user", "global", id)
		if e != nil {
			return e
		}
		if s.providers[u.Provider].config.Type != "local" {
			return ErrForbidden
		}
		if u.Version != in.ExpectedVersion {
			return ErrConflict
		}
		// Profiles/passwords are global identities. A tenant owner may change one
		// only when they administer every tenant to which that user has access.
		grants, e := s.effectiveGrants(tx, u)
		if e != nil {
			return e
		}
		for other := range grants {
			if e = s.checkOwner(tx, other, a); e != nil {
				return ErrForbidden
			}
		}
		u.Name, u.Email, u.Version = in.Name, in.Email, u.Version+1
		if len(h) > 0 {
			u.Epoch++
			if e = put(tx, "local_password", "global", id, localCredential{Hash: h}, time.Time{}); e != nil {
				return e
			}
		}
		out = u
		return put(tx, "user", "global", id, u, time.Time{})
	})
	return out, err
}
func (s *Service) mountLocalUsers(mux *http.ServeMux) {
	actor := func(r *http.Request) (Actor, string) {
		p, _ := tenancy.FromContext(r.Context())
		return Actor{ID: p.Subject, Kind: p.Kind, Name: p.Name, Grants: map[string]string{p.TenantID: p.Role}}, p.TenantID
	}
	mux.HandleFunc("POST /api/team-access/users", func(w http.ResponseWriter, r *http.Request) {
		var in LocalUserInput
		if err := decode(w, r, &in); err != nil {
			authFailure(w, err)
			return
		}
		a, t := actor(r)
		out, err := s.CreateLocalUser(r.Context(), a, t, in)
		if err != nil {
			authFailure(w, err)
			return
		}
		authJSON(w, 201, out)
	})
	mux.HandleFunc("PUT /api/team-access/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in LocalUserInput
		if err := decode(w, r, &in); err != nil {
			authFailure(w, err)
			return
		}
		a, t := actor(r)
		out, err := s.UpdateLocalUser(r.Context(), a, t, r.PathValue("id"), in)
		if err != nil {
			authFailure(w, err)
			return
		}
		authJSON(w, 200, out)
	})
}

func (s *Service) authenticateLocal(ctx context.Context, provider, username, password string) (externalUser, error) {
	if !demoPassword(s.providers[provider].config, username, password) && (len(password) < 12 || len(password) > 72) {
		return externalUser{}, ErrForbidden
	}
	u := s.user(provider, externalUser{Subject: username})
	var credential localCredential
	err := s.repo.View(ctx, func(tx Tx) error {
		var e error
		credential, e = read[localCredential](tx, "local_password", "global", u.ID)
		return e
	})
	if err != nil && !errors.Is(err, ErrNotFound) {
		return externalUser{}, err
	}
	h := credential.Hash
	if len(h) == 0 {
		h = dummyLocalHash
	}
	if bcrypt.CompareHashAndPassword(h, []byte(password)) != nil || credential.Disabled || len(credential.Hash) == 0 {
		return externalUser{}, ErrForbidden
	}
	var user User
	if err = s.repo.View(ctx, func(tx Tx) error { var e error; user, e = read[User](tx, "user", "global", u.ID); return e }); err != nil {
		return externalUser{}, err
	}
	return externalUser{Subject: user.Subject, Name: user.Name, Email: user.Email, LocalCredentialDigest: hash(string(credential.Hash))}, nil
}
