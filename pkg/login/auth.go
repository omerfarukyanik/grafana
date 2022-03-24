package login

import (
	"context"
	"errors"

	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/services/ldap"
	"github.com/grafana/grafana/pkg/services/login"
	"github.com/grafana/grafana/pkg/services/sqlstore"
)

var (
	ErrEmailNotAllowed       = errors.New("required email domain not fulfilled")
	ErrInvalidCredentials    = errors.New("invalid username or password")
	ErrNoEmail               = errors.New("login provider didn't return an email address")
	ErrProviderDeniedRequest = errors.New("login provider denied login request")
	ErrTooManyLoginAttempts  = errors.New("too many consecutive incorrect login attempts for user - login for user temporarily blocked")
	ErrPasswordEmpty         = errors.New("no password provided")
	ErrUserDisabled          = errors.New("user is disabled")
	ErrAbsoluteRedirectTo    = errors.New("absolute URLs are not allowed for redirect_to cookie value")
	ErrInvalidRedirectTo     = errors.New("invalid redirect_to cookie value")
	ErrForbiddenRedirectTo   = errors.New("forbidden redirect_to cookie value")
)

var loginLogger = log.New("login")

var AuthenticateUserFunc = AuthenticateUser

func Init() {
	bus.AddHandler("auth", AuthenticateUser)
}

// AuthenticateUser authenticates the user via username & password
func AuthenticateUser(ctx context.Context, query *models.LoginUserQuery, store sqlstore.Store, loginService login.Service, authInfoService login.AuthInfoService) error {
	if err := validateLoginAttempts(ctx, query, store); err != nil {
		return err
	}

	if err := validatePasswordSet(query.Password); err != nil {
		return err
	}

	err := loginUsingGrafanaDB(ctx, query, store)
	if err == nil || (!errors.Is(err, models.ErrUserNotFound) && !errors.Is(err, ErrInvalidCredentials) &&
		!errors.Is(err, ErrUserDisabled)) {
		query.AuthModule = "grafana"
		return err
	}

	ldapEnabled, ldapErr := loginUsingLDAP(ctx, query, store, loginService, authInfoService)
	if ldapEnabled {
		query.AuthModule = models.AuthModuleLDAP
		if ldapErr == nil || !errors.Is(ldapErr, ldap.ErrInvalidCredentials) {
			return ldapErr
		}

		if !errors.Is(err, ErrUserDisabled) || !errors.Is(ldapErr, ldap.ErrInvalidCredentials) {
			err = ldapErr
		}
	}

	if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ldap.ErrInvalidCredentials) {
		if err := saveInvalidLoginAttempt(ctx, query, store); err != nil {
			loginLogger.Error("Failed to save invalid login attempt", "err", err)
		}

		return ErrInvalidCredentials
	}

	return err
}

func validatePasswordSet(password string) error {
	if len(password) == 0 {
		return ErrPasswordEmpty
	}

	return nil
}
