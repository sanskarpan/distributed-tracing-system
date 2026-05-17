package api

import (
	"context"
	"net/http"
	"os"
	"strings"

	"google.golang.org/grpc/metadata"
)

type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

type Principal struct {
	Token           string `json:"-"`
	Role            Role   `json:"role"`
	TenantID        string `json:"tenantId,omitempty"`
	IsGlobal        bool   `json:"isGlobal"`
	AuthDisabled    bool   `json:"authDisabled,omitempty"`
	RequestedTenant string `json:"requestedTenant,omitempty"`
}

type AuthConfig struct {
	Enabled bool
	Tokens  map[string]Principal
}

type principalContextKey struct{}

func LoadAuthConfig(apiKey string) AuthConfig {
	config := AuthConfig{Tokens: make(map[string]Principal)}

	raw := strings.TrimSpace(os.Getenv("AUTH_TOKENS"))
	if raw != "" {
		for _, entry := range strings.Split(raw, ";") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			parts := strings.Split(entry, "|")
			if len(parts) != 3 {
				continue
			}
			token := strings.TrimSpace(parts[0])
			role := normalizeRole(parts[1])
			tenantID := strings.TrimSpace(parts[2])
			if token == "" || role == "" {
				continue
			}
			principal := Principal{
				Token:    token,
				Role:     role,
				TenantID: tenantID,
				IsGlobal: tenantID == "" || tenantID == "*",
			}
			config.Tokens[token] = principal
		}
	}

	if len(config.Tokens) == 0 && apiKey != "" {
		config.Tokens[apiKey] = Principal{
			Token:    apiKey,
			Role:     RoleAdmin,
			TenantID: "*",
			IsGlobal: true,
		}
	}

	config.Enabled = len(config.Tokens) > 0
	return config
}

func AuthMiddleware(config AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !config.Enabled {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				principal := Principal{
					Role:         RoleAdmin,
					TenantID:     "*",
					IsGlobal:     true,
					AuthDisabled: true,
				}
				next.ServeHTTP(w, withPrincipal(r, principal))
			})
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r.Header.Get("Authorization"))
			if token == "" {
				token = r.URL.Query().Get("apiKey")
			}
			principal, ok := config.Tokens[token]
			if !ok {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			principal.RequestedTenant = strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
			if principal.RequestedTenant == "" {
				principal.RequestedTenant = strings.TrimSpace(r.URL.Query().Get("tenantId"))
			}
			next.ServeHTTP(w, withPrincipal(r, principal))
		})
	}
}

func RequireRole(min Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := PrincipalFromContext(r.Context())
			if roleRank(principal.Role) < roleRank(min) {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func PrincipalFromContext(ctx context.Context) Principal {
	principal, _ := ctx.Value(principalContextKey{}).(Principal)
	return principal
}

func EffectiveTenant(principal Principal) string {
	if principal.IsGlobal {
		return principal.RequestedTenant
	}
	return principal.TenantID
}

func AuthenticateGRPCPrincipal(ctx context.Context, config AuthConfig) (Principal, bool) {
	if !config.Enabled {
		return Principal{
			Role:         RoleAdmin,
			TenantID:     "*",
			IsGlobal:     true,
			AuthDisabled: true,
		}, true
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return Principal{}, false
	}
	token := ""
	if authHeaders := md.Get("authorization"); len(authHeaders) > 0 {
		token = bearerToken(authHeaders[0])
	}
	if token == "" {
		if apiKeys := md.Get("apikey"); len(apiKeys) > 0 {
			token = strings.TrimSpace(apiKeys[0])
		}
	}
	principal, ok := config.Tokens[token]
	if !ok {
		return Principal{}, false
	}
	if tenants := md.Get("x-tenant-id"); len(tenants) > 0 {
		principal.RequestedTenant = strings.TrimSpace(tenants[0])
	}
	return principal, true
}

func principalCanAccessTenant(principal Principal, tenantID string) bool {
	if principal.IsGlobal {
		return true
	}
	return tenantID == "" || principal.TenantID == tenantID
}

func withPrincipal(r *http.Request, principal Principal) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal))
}

func normalizeRole(raw string) Role {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(RoleViewer):
		return RoleViewer
	case string(RoleOperator):
		return RoleOperator
	case string(RoleAdmin):
		return RoleAdmin
	default:
		return ""
	}
}

func roleRank(role Role) int {
	switch role {
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

func bearerToken(header string) string {
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[len("Bearer "):])
}
