package proxy

import (
	"errors"
	"net/http"
	"sync/atomic"

	"tensors-router/internal/auth"
	"tensors-router/internal/openai"
)

const (
	securityProfilePath   = "/router/v1/site/security/profile"
	securityProfileSecure = "secure"

	secureProfileLocksOutCallerMessage = "this request carries no admin token, so the WebUI would lose access after the switch; set router.token in webui.yaml to an admin token first"
)

type SecurityProfileControl struct {
	Running         string
	SecureBlocker   func() error
	WriteSecureFile func() error
}

type securityProfile struct {
	control        SecurityProfileControl
	secureInConfig atomic.Bool
}

type securityProfileStatus struct {
	Profile                string `json:"profile"`
	ProfileRestartRequired bool   `json:"profile_restart_required"`
	SecureBlocker          string `json:"secure_profile_blocker,omitempty"`
}

func newSecurityProfile(control SecurityProfileControl) *securityProfile {
	profile := &securityProfile{control: control}
	profile.secureInConfig.Store(control.Running == securityProfileSecure)
	return profile
}

func (profile *securityProfile) status() securityProfileStatus {
	status := securityProfileStatus{
		Profile:                profile.control.Running,
		ProfileRestartRequired: profile.secureInConfig.Load() && profile.control.Running != securityProfileSecure,
	}
	if !profile.secureInConfig.Load() {
		if err := profile.secureBlocker(); err != nil {
			status.SecureBlocker = err.Error()
		}
	}
	return status
}

func (profile *securityProfile) secureBlocker() error {
	if profile.control.SecureBlocker == nil || profile.control.WriteSecureFile == nil {
		return errors.New("this router cannot change its configuration file")
	}
	return profile.control.SecureBlocker()
}

func (admin *securityAdmin) handleSwitchToSecureProfile(w http.ResponseWriter, r *http.Request) {
	if admin.profile.secureInConfig.Load() {
		openai.WriteJSON(w, http.StatusOK, admin.profile.status())
		return
	}
	if err := admin.profile.secureBlocker(); err != nil {
		openai.WriteError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	keys := admin.credentials.KeyHashes()
	if len(keys.Admin) > 0 && !auth.PrincipalFor(r.Header.Get("Authorization"), keys).Admin {
		openai.WriteError(w, http.StatusConflict, "conflict", secureProfileLocksOutCallerMessage)
		return
	}
	if err := admin.profile.control.WriteSecureFile(); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", "could not update the configuration file: "+err.Error())
		return
	}
	admin.profile.secureInConfig.Store(true)
	openai.WriteJSON(w, http.StatusOK, admin.profile.status())
}
