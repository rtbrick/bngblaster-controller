package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gavv/httpexpect/v2"
	isf "github.com/matryer/is"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

const (
	configFolder = "configs"
)

func TestServer_create(t *testing.T) {
	tests := []struct {
		name         string
		resultCreate error
		resultExists bool
		body         string
		wantBody     string
		want         int
	}{
		{
			name:         "not_exists",
			resultExists: false,
			resultCreate: nil,
			wantBody:     "body not readable",
			want:         http.StatusBadRequest,
		}, {
			name:         "exists",
			resultExists: true,
			resultCreate: nil,
			body:         "{}",
			want:         http.StatusNoContent,
		}, {
			name:         "running",
			resultExists: true,
			resultCreate: controller.ErrBlasterRunning,
			body:         "{}",
			wantBody:     "instance is running\n",
			want:         http.StatusPreconditionFailed,
		}, {
			name:         "error",
			resultExists: true,
			resultCreate: fmt.Errorf("other error"),
			body:         "{}",
			wantBody:     "not able to create instance\n",
			want:         http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := isf.New(t)
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				CreateFunc: func(name string, config []byte) error {
					return tt.resultCreate
				},
				ExistsFunc: func(name string) bool {
					return tt.resultExists
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.PUT("/api/v1/instances/{instance_name}", tt.name)
			request.WithText(tt.body)

			response := request.Expect().
				Status(tt.want)
			if tt.wantBody == "" {
				response.NoContent()
			} else {
				response.Text().Contains(tt.wantBody)
			}
			for _, call := range repository.ExistsCalls() {
				is.Equal(call.Name, tt.name)
			}
		})
	}
}

func TestServer_status(t *testing.T) {
	tests := []struct {
		name          string
		resultRunning bool
		resultExists  bool
		wantBody      string
		want          int
	}{
		{
			name:          "not_exists",
			resultExists:  false,
			resultRunning: false,
			wantBody:      "404 page not found",
			want:          http.StatusNotFound,
		}, {
			name:          "exists",
			resultExists:  true,
			resultRunning: false,
			wantBody:      "stopped",
			want:          http.StatusOK,
		}, {
			name:          "running",
			resultExists:  true,
			resultRunning: true,
			wantBody:      "started",
			want:          http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := isf.New(t)
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				RunningFunc: func(name string) bool {
					return tt.resultRunning
				},
				ExistsFunc: func(name string) bool {
					return tt.resultExists
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.GET("/api/v1/instances/{instance_name}", tt.name)

			response := request.Expect().
				Status(tt.want)

			if tt.wantBody == "" {
				response.NoContent()
			} else if response.Raw().StatusCode == http.StatusOK {
				response.JSON().Object().ContainsKey("status").ValueEqual("status", tt.wantBody)
			} else {
				response.JSON().Object().ContainsKey("message").ValueEqual("message", tt.wantBody)
			}
			for _, call := range repository.ExistsCalls() {
				is.Equal(call.Name, tt.name)
			}
		})
	}
}

func TestServer_delete(t *testing.T) {
	tests := []struct {
		name         string
		resultDelete error
		body         string
		want         int
	}{
		{
			name:         "not_exists",
			resultDelete: nil,
			want:         http.StatusNoContent,
		}, {
			name:         "exists",
			resultDelete: nil,
			want:         http.StatusNoContent,
		}, {
			name:         "running",
			resultDelete: controller.ErrBlasterRunning,
			body:         "instance is running",
			want:         http.StatusPreconditionFailed,
		}, {
			name:         "error",
			resultDelete: fmt.Errorf("other error"),
			body:         "not able to delete instance",
			want:         http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := isf.New(t)
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				DeleteFunc: func(name string) error {
					return tt.resultDelete
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			response := e.DELETE("/api/v1/instances/{instance_name}", tt.name).
				Expect().
				Status(tt.want)
			if tt.body == "" {
				response.NoContent()
			} else {
				response.JSON().Object().ContainsKey("message").ValueEqual("message", tt.body)
			}
			is.Equal(len(repository.DeleteCalls()), 1)
			for _, call := range repository.DeleteCalls() {
				is.Equal(call.Name, tt.name)
			}
		})
	}
}

func TestServer_start(t *testing.T) {
	tests := []struct {
		name        string
		resultStart error
		body        *controller.RunningConfig
		wantBody    string
		want        int
	}{
		{
			name:        "bad_body",
			resultStart: nil,
			want:        http.StatusBadRequest,
		}, {
			name:        "not_exists",
			resultStart: controller.ErrBlasterNotExists,
			body:        &controller.RunningConfig{},
			wantBody:    "404 page not found",
			want:        http.StatusNotFound,
		}, {
			name:        "exists",
			resultStart: nil,
			body:        &controller.RunningConfig{},
			want:        http.StatusNoContent,
		}, {
			name:        "running",
			resultStart: controller.ErrBlasterRunning,
			body:        &controller.RunningConfig{},
			wantBody:    "instance is running",
			want:        http.StatusPreconditionFailed,
		}, {
			name:        "error",
			resultStart: fmt.Errorf("other error"),
			body:        &controller.RunningConfig{},
			wantBody:    "not able to start",
			want:        http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := isf.New(t)
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				StartFunc: func(name string, config controller.RunningConfig) error {
					return tt.resultStart
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.POST("/api/v1/instances/{instance_name}/_start", tt.name)
			if tt.body != nil {
				request.WithJSON(tt.body)
			}
			response := request.Expect().Status(tt.want)
			if tt.want == http.StatusBadRequest {
				return
			}
			if tt.wantBody == "" {
				response.NoContent()
			} else {
				response.JSON().Object().ContainsKey("message").ValueEqual("message", tt.wantBody)
			}
			for _, call := range repository.StartCalls() {
				is.Equal(call.Name, tt.name)
			}
		})
	}
}

func TestServer_stop(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{
			name: "not_exists",
			want: http.StatusAccepted,
		}, {
			name: "exists",
			want: http.StatusAccepted,
		}, {
			name: "running",
			want: http.StatusAccepted,
		}, {
			name: "error",
			want: http.StatusAccepted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := isf.New(t)
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				StopFunc: func(name string) {
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.POST("/api/v1/instances/{instance_name}/_stop", tt.name)

			response := request.Expect().Status(tt.want)
			if tt.want == http.StatusBadRequest {
				return
			}
			response.NoContent()
			for _, call := range repository.StopCalls() {
				is.Equal(call.Name, tt.name)
			}
		})
	}
}

func TestServer_kill(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{
			name: "not_exists",
			want: http.StatusAccepted,
		}, {
			name: "exists",
			want: http.StatusAccepted,
		}, {
			name: "running",
			want: http.StatusAccepted,
		}, {
			name: "error",
			want: http.StatusAccepted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := isf.New(t)
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				KillFunc: func(name string) {
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.POST("/api/v1/instances/{instance_name}/_kill", tt.name)

			response := request.Expect().Status(tt.want)
			if tt.want == http.StatusBadRequest {
				return
			}
			response.NoContent()
			for _, call := range repository.KillCalls() {
				is.Equal(call.Name, tt.name)
			}
		})
	}
}

func TestServer_command(t *testing.T) {
	tests := []struct {
		name        string
		resultStart []byte
		resultError error
		body        *controller.SocketCommand
		wantBody    interface{}
		wantCode    int
		want        int
	}{
		{
			name:        "bad_body",
			resultStart: nil,
			want:        http.StatusBadRequest,
		}, {
			name:        "not_exists",
			resultError: controller.ErrBlasterNotExists,
			body:        &controller.SocketCommand{},
			wantBody:    &message{Message: "404 page not found"},
			want:        http.StatusNotFound,
		}, {
			name:        "not running",
			resultError: controller.ErrBlasterNotRunning,
			body:        &controller.SocketCommand{},
			wantBody:    &message{Message: "instance is not running"},
			want:        http.StatusPreconditionFailed,
		}, {
			name:        "null returned or not json",
			resultStart: nil,
			body:        &controller.SocketCommand{},
			wantBody:    &message{Message: "EOF"},
			want:        http.StatusInternalServerError,
		}, {
			name:        "exists ok",
			resultStart: []byte(`{"code": 200}`),
			body:        &controller.SocketCommand{},
			wantBody:    " ",
			wantCode:    200,
			want:        http.StatusOK,
		}, {
			name:        "exists not found",
			resultStart: []byte(`{"code": 404}`),
			body:        &controller.SocketCommand{},
			wantBody:    " ",
			wantCode:    404,
			want:        http.StatusNotFound,
		}, {
			name:        "error",
			resultError: fmt.Errorf("other error"),
			body:        &controller.SocketCommand{},
			wantBody:    &message{Message: "not able to send command"},
			want:        http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := isf.New(t)
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				CommandFunc: func(name string, config controller.SocketCommand) ([]byte, error) {
					return tt.resultStart, tt.resultError
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.POST("/api/v1/instances/{instance_name}/_command", tt.name)
			if tt.body != nil {
				request.WithJSON(tt.body)
			}
			response := request.Expect().Status(tt.want)
			if tt.want == http.StatusBadRequest {
				return
			}
			if tt.wantCode > 0 {
				response.JSON().Object().ContainsKey("code").Value("code").Equal(tt.wantCode)
			} else if tt.wantBody == "" {
				response.NoContent()
			} else {
				response.JSON().Equal(tt.wantBody)
			}
			for _, call := range repository.StartCalls() {
				is.Equal(call.Name, tt.name)
			}
		})
	}
}
