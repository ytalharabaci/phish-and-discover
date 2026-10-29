package api

import (
	"net/http"
	"os"
	"sort"

	"github.com/ytalharabaci/phish-and-discover/models"
	"github.com/ytalharabaci/phish-and-discover/pkg/dockerstatus"
	"github.com/ytalharabaci/phish-and-discover/pkg/sysinfo"
)

// UserLoginInfo is the last-login summary exposed on the System Status page.
// It deliberately excludes Hash/ApiKey/Role — only what an operator needs to
// notice "who logged in, and when" at a glance.
type UserLoginInfo struct {
	Username      string `json:"username"`
	LastLogin     string `json:"last_login"`
	AccountLocked bool   `json:"account_locked"`
}

// SystemStatusResponse is the JSON response for GET /api/v1/system/status
type SystemStatusResponse struct {
	Host       sysinfo.Snapshot         `json:"host"`
	Containers []dockerstatus.Container `json:"containers"`
	// DockerError is set instead of failing the whole request when the
	// docker-proxy sidecar is unreachable — host metrics are still useful
	// on their own.
	DockerError string          `json:"docker_error,omitempty"`
	Users       []UserLoginInfo `json:"users"`
}

// SystemStatus reports host CPU/RAM/disk usage, container health (via the
// docker-proxy sidecar), and per-user last-login info, for the
// Administration > System Status admin page.
// GET /api/v1/system/status
func (as *Server) SystemStatus(w http.ResponseWriter, r *http.Request) {
	resp := SystemStatusResponse{
		Host: sysinfo.Get(os.Getenv("DISK_USAGE_PATH")),
	}

	containers, err := dockerstatus.List(os.Getenv("DOCKER_PROXY_URL"))
	if err != nil {
		resp.DockerError = err.Error()
	} else {
		resp.Containers = containers
	}

	users, err := models.GetUsers()
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "failed to load users: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	for _, u := range users {
		resp.Users = append(resp.Users, UserLoginInfo{
			Username:      u.Username,
			LastLogin:     u.LastLogin.UTC().Format("2006-01-02T15:04:05Z"),
			AccountLocked: u.AccountLocked,
		})
	}
	sort.Slice(resp.Users, func(i, j int) bool {
		return resp.Users[i].LastLogin > resp.Users[j].LastLogin
	})

	JSONResponse(w, resp, http.StatusOK)
}
