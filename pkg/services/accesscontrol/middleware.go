package accesscontrol

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/grafana/grafana/pkg/middleware/cookies"
	"github.com/grafana/grafana/pkg/models/usertoken"
	"github.com/grafana/grafana/pkg/services/auth/identity"
	"github.com/grafana/grafana/pkg/services/authn"
	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	"github.com/grafana/grafana/pkg/services/org"
	"github.com/grafana/grafana/pkg/services/user"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/grafana/grafana/pkg/util"
	"github.com/grafana/grafana/pkg/web"
)

func Middleware(ac AccessControl) func(Evaluator) web.Handler {
	return func(evaluator Evaluator) web.Handler {
		return func(c *contextmodel.ReqContext) {
			if c.AllowAnonymous {
				forceLogin, _ := strconv.ParseBool(c.Req.URL.Query().Get("forceLogin")) // ignoring error, assuming false for non-true values is ok.
				orgID, err := strconv.ParseInt(c.Req.URL.Query().Get("orgId"), 10, 64)
				if err == nil && orgID > 0 && orgID != c.SignedInUser.GetOrgID() {
					forceLogin = true
				}

				if !c.IsSignedIn && forceLogin {
					unauthorized(c, nil)
					return
				}
			}

			if c.LookupTokenErr != nil {
				var revokedErr *usertoken.TokenRevokedError
				if errors.As(c.LookupTokenErr, &revokedErr) {
					tokenRevoked(c, revokedErr)
					return
				}

				unauthorized(c, c.LookupTokenErr)
				return
			}

			authorize(c, ac, c.SignedInUser, evaluator)
		}
	}
}

func authorize(c *contextmodel.ReqContext, ac AccessControl, user identity.Requester, evaluator Evaluator) {
	injected, err := evaluator.MutateScopes(c.Req.Context(), scopeInjector(scopeParams{
		OrgID:     user.GetOrgID(),
		URLParams: web.Params(c.Req),
	}))
	if err != nil {
		c.JsonApiErr(http.StatusInternalServerError, "Internal server error", err)
		return
	}

	hasAccess, err := ac.Evaluate(c.Req.Context(), user, injected)
	if !hasAccess || err != nil {
		deny(c, injected, err)
		return
	}
}

func deny(c *contextmodel.ReqContext, evaluator Evaluator, err error) {
	id := newID()
	if err != nil {
		c.Logger.Error("Error from access control system", "error", err, "accessErrorID", id)
	} else {
		namespace, identifier := c.SignedInUser.GetNamespacedID()
		c.Logger.Info(
			"Access denied",
			"namespace", namespace,
			"userID", identifier,
			"accessErrorID", id,
			"permissions", evaluator.GoString(),
		)
	}

	if !c.IsApiRequest() {
		// TODO(emil): I'd like to show a message after this redirect, not sure how that can be done?
		writeRedirectCookie(c)
		c.Redirect(setting.AppSubUrl + "/")
		return
	}

	message := ""
	if evaluator != nil {
		message = evaluator.String()
	}

	// If the user triggers an error in the access control system, we
	// don't want the user to be aware of that, so the user gets the
	// same information from the system regardless of if it's an
	// internal server error or access denied.
	c.JSON(http.StatusForbidden, map[string]string{
		"title":         "Access denied", // the component needs to pick this up
		"message":       fmt.Sprintf("You'll need additional permissions to perform this action. Permissions needed: %s", message),
		"accessErrorId": id,
	})
}

func unauthorized(c *contextmodel.ReqContext, err error) {
	if c.IsApiRequest() {
		c.WriteErrOrFallback(http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized), c.LookupTokenErr)
		return
	}

	writeRedirectCookie(c)
	if errors.Is(c.LookupTokenErr, authn.ErrTokenNeedsRotation) {
		c.Redirect(setting.AppSubUrl + "/user/auth-tokens/rotate")
		return
	}

	c.Redirect(setting.AppSubUrl + "/login")
}

func tokenRevoked(c *contextmodel.ReqContext, err *usertoken.TokenRevokedError) {
	if c.IsApiRequest() {
		c.JSON(http.StatusUnauthorized, map[string]any{
			"message": "Token revoked",
			"error": map[string]any{
				"id":                    "ERR_TOKEN_REVOKED",
				"maxConcurrentSessions": err.MaxConcurrentSessions,
			},
		})
		return
	}

	writeRedirectCookie(c)
	c.Redirect(setting.AppSubUrl + "/login")
}

func writeRedirectCookie(c *contextmodel.ReqContext) {
	redirectTo := c.Req.RequestURI
	if setting.AppSubUrl != "" && !strings.HasPrefix(redirectTo, setting.AppSubUrl) {
		redirectTo = setting.AppSubUrl + c.Req.RequestURI
	}

	// remove any forceLogin=true params
	redirectTo = removeForceLoginParams(redirectTo)

	cookies.WriteCookie(c.Resp, "redirect_to", url.QueryEscape(redirectTo), 0, nil)
}

var forceLoginParamsRegexp = regexp.MustCompile(`&?forceLogin=true`)

func removeForceLoginParams(str string) string {
	return forceLoginParamsRegexp.ReplaceAllString(str, "")
}

func newID() string {
	// Less ambiguity than alphanumerical.
	numerical := []byte("0123456789")
	id, err := util.GetRandomString(10, numerical...)
	if err != nil {
		// this should not happen, but if it does, a timestamp is as
		// useful as anything.
		id = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return "ACE" + id
}

type OrgIDGetter func(c *contextmodel.ReqContext) (int64, error)

type userCache interface {
	GetSignedInUserWithCacheCtx(ctx context.Context, query *user.GetSignedInUserQuery) (*user.SignedInUser, error)
}

func AuthorizeInOrgMiddleware(ac AccessControl, service Service, cache userCache) func(OrgIDGetter, Evaluator) web.Handler {
	return func(getTargetOrg OrgIDGetter, evaluator Evaluator) web.Handler {
		return func(c *contextmodel.ReqContext) {
			// We need to copy the user here because we're going to mutate it
			userCopy := *(c.SignedInUser)
			targetOrgID, err := getTargetOrg(c)
			if err != nil {
				deny(c, nil, fmt.Errorf("failed to get target org: %w", err))
				return
			}

			if userCopy.OrgID != targetOrgID {
				switch targetOrgID {
				case GlobalOrgID:
					userCopy.OrgID = GlobalOrgID
					userCopy.OrgRole = org.RoleNone
					userCopy.OrgName = ""
				default:
					query := user.GetSignedInUserQuery{UserID: c.UserID, OrgID: targetOrgID}
					queryResult, err := cache.GetSignedInUserWithCacheCtx(c.Req.Context(), &query)
					if err != nil {
						deny(c, nil, fmt.Errorf("failed to authenticate user in target org: %w", err))
						return
					}
					userCopy.OrgID = queryResult.OrgID
					userCopy.OrgName = queryResult.OrgName
					userCopy.OrgRole = queryResult.OrgRole
				}
			}

			if userCopy.Permissions[targetOrgID] == nil {
				permissions, err := service.GetUserPermissions(c.Req.Context(), &userCopy, Options{})
				if err != nil {
					deny(c, nil, fmt.Errorf("failed to authenticate user in target org: %w", err))
				}

				// guard against nil map
				if userCopy.Permissions == nil {
					userCopy.Permissions = make(map[int64]map[string][]string)
				}
				userCopy.Permissions[targetOrgID] = GroupScopesByAction(permissions)
			}

			authorize(c, ac, &userCopy, evaluator)

			// guard against nil map
			if c.SignedInUser.Permissions == nil {
				c.SignedInUser.Permissions = make(map[int64]map[string][]string)
			}
			c.SignedInUser.Permissions[targetOrgID] = userCopy.Permissions[targetOrgID]
		}
	}
}

func UseOrgFromContextParams(c *contextmodel.ReqContext) (int64, error) {
	orgID, err := strconv.ParseInt(web.Params(c.Req)[":orgId"], 10, 64)

	// Special case of macaron handling invalid params
	if err != nil {
		return 0, org.ErrOrgNotFound.Errorf("failed to get organization from context: %w", err)
	}

	if orgID == 0 {
		return 0, org.ErrOrgNotFound.Errorf("empty org ID")
	}

	return orgID, nil
}

func UseGlobalOrg(c *contextmodel.ReqContext) (int64, error) {
	return GlobalOrgID, nil
}

// UseGlobalOrSingleOrg returns the global organization or the current organization in a single organization setup
func UseGlobalOrSingleOrg(cfg *setting.Cfg) OrgIDGetter {
	return func(c *contextmodel.ReqContext) (int64, error) {
		if cfg.RBACSingleOrganization {
			return c.GetOrgID(), nil
		}
		return GlobalOrgID, nil
	}
}

// scopeParams holds the parameters used to fill in scope templates
type scopeParams struct {
	OrgID     int64
	URLParams map[string]string
}

// scopeInjector inject request params into the templated scopes. e.g. "settings:" + eval.Parameters(":id")
func scopeInjector(params scopeParams) ScopeAttributeMutator {
	return func(_ context.Context, scope string) ([]string, error) {
		tmpl, err := template.New("scope").Parse(scope)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if err = tmpl.Execute(&buf, params); err != nil {
			return nil, err
		}
		return []string{buf.String()}, nil
	}
}
