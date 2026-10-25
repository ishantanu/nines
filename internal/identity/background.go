package identity

import "context"

// AuthorizeBackground rechecks present membership or service-account validity
// without persisting bearer credentials. It grants no new permissions.
func (s *Service) AuthorizeBackground(ctx context.Context, tenant, subject, kind string) error {
	return s.authorizeBackground(ctx, tenant, subject, kind, rank("agent"))
}
func (s *Service) AuthorizeBackgroundReview(ctx context.Context, tenant, subject, kind string, owner bool) error {
	minimum := rank("editor")
	if owner {
		minimum = rank("owner")
	}
	return s.authorizeBackground(ctx, tenant, subject, kind, minimum)
}
func (s *Service) AuthorizeBackgroundRead(ctx context.Context, tenant, subject, kind string) error {
	return s.authorizeBackground(ctx, tenant, subject, kind, rank("viewer"))
}
func (s *Service) authorizeBackground(ctx context.Context, tenant, subject, kind string, minimum int) error {
	if !s.tenants[tenant] {
		return ErrForbidden
	}
	return s.repo.View(ctx, func(tx Tx) error {
		switch kind {
		case "user":
			user, err := read[User](tx, "user", "global", subject)
			if err != nil {
				return err
			}
			grants, err := s.effectiveGrants(tx, user)
			if err != nil {
				return err
			}
			if rank(grants[tenant]) < minimum {
				return ErrForbidden
			}
			return nil
		case "service_account":
			account, err := read[Account](tx, "account", tenant, subject)
			if err != nil {
				return err
			}
			if account.Tenant != tenant || !account.Expires.After(s.Now()) || rank(account.Role) < minimum {
				return ErrForbidden
			}
			token, err := read[Account](tx, "service_token", "global", account.TokenHash)
			if err != nil {
				return err
			}
			if token.ID != account.ID || token.Tenant != tenant {
				return ErrForbidden
			}
			return nil
		default:
			return ErrForbidden
		}
	})
}
