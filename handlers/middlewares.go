package handlers

import (
	"context"
	"net/http"

	"github.com/layer5io/meshery/models"
	"github.com/sirupsen/logrus"
)

// ProviderMiddleware is a middleware to validate if a provider is set
func (h *Handler) ProviderMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, req *http.Request) {
		var providerName string
		var provider models.Provider
		ck, err := req.Cookie(h.config.ProviderCookieName)
		if err == nil {
			providerName = ck.Value
		} else {
			providerName = req.Header.Get(h.config.ProviderCookieName)
		}
		if providerName != "" {
			provider, _ = h.config.Providers[providerName]
		}
		if provider == nil {
			http.Redirect(w, req, "/provider", http.StatusFound)
			return
		}
		ctx := context.WithValue(req.Context(), models.ProviderCtxKey, provider)
		req1 := req.WithContext(ctx)
		next.ServeHTTP(w, req1)
	}
	return http.HandlerFunc(fn)
}

// AuthMiddleware is a middleware to validate if a user is authenticated
func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, req *http.Request) {
		providerI := req.Context().Value(models.ProviderCtxKey)
		// logrus.Debugf("models.ProviderCtxKey %s", models.ProviderCtxKey)
		provider, ok := providerI.(models.Provider)
		if !ok {
			http.Redirect(w, req, "/provider", http.StatusFound)
			return
		}
		// logrus.Debugf("provider %s", provider)
		isValid := h.validateAuth(provider, req)
		// logrus.Debugf("validate auth: %t", isValid)
		if !isValid {
			// if h.GetProviderType() == models.RemoteProviderType {
			// 	http.Redirect(w, req, "/login", http.StatusFound)
			// } else { // Local Provider
			// 	h.LoginHandler(w, req)
			// }
			// return
			if provider.GetProviderType() == models.RemoteProviderType {
				provider.Logout(w, req)
				return
			}
			// Local Provider
			h.LoginHandler(w, req, provider, true)
		}
		next.ServeHTTP(w, req)
	}
	return http.HandlerFunc(fn)
}

func (h *Handler) validateAuth(provider models.Provider, req *http.Request) bool {
	err := provider.GetSession(req)
	if err == nil {
		// logrus.Debugf("session: %v", sess)
		return true
	}
	// logrus.Errorf("session invalid, error: %v", err)
	return false
}

// SessionInjectorMiddleware - is a middleware which injects user and session object
func (h *Handler) SessionInjectorMiddleware(next func(http.ResponseWriter, *http.Request, *models.Preference, *models.User, models.Provider)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		providerI := req.Context().Value(models.ProviderCtxKey)
		provider, ok := providerI.(models.Provider)
		if !ok {
			http.Redirect(w, req, "/provider", http.StatusFound)
			return
		}
		// ensuring session is intact before running load test
		err := provider.GetSession(req)
		if err != nil {
			provider.Logout(w, req)
			logrus.Errorf("Error: unable to get session: %v", err)
			http.Error(w, "unable to get session", http.StatusUnauthorized)
			return
		}

		user, _ := provider.GetUserDetails(req)

		prefObj, err := provider.ReadFromPersister(user.UserID)
		if err != nil {
			logrus.Warn("unable to read session from the session persister, starting with a new one")
		}

		if prefObj == nil {
			prefObj = &models.Preference{
				AnonymousUsageStats:  true,
				AnonymousPerfResults: true,
			}
		}
		provider.UpdateToken(w, req)
		next(w, req, prefObj, user, provider)
	})
}
