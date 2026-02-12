package handler

import (
	"net/http"
	"path/filepath"
	"strings"

	"insightify/gen/go/insightify/v1/insightifyv1connect"
	"insightify/internal/gateway/projectstore"
	"insightify/internal/gateway/runtime"
)

// Compile-time interface checks.
var _ insightifyv1connect.PipelineServiceHandler = (*Service)(nil)
var _ insightifyv1connect.LlmChatServiceHandler = (*Service)(nil)

// Service implements all gateway RPC interfaces.
// It holds the runtime App as its single dependency.
type Service struct {
	app *runtime.App
}

// NewService creates a gateway service backed by the given runtime App.
func NewService(app *runtime.App) *Service {
	return &Service{app: app}
}

// App returns the underlying runtime App (used by tests / compat shims).
func (s *Service) App() *runtime.App { return s.app }

// DefaultApp creates a runtime.App with the default project store.
func DefaultApp() *runtime.App {
	return runtime.New(projectstore.NewFromEnv(filepath.Join("tmp", "project_states.json")))
}

// BuildMux registers all RPC handlers on a new ServeMux.
func BuildMux(s *Service) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle(insightifyv1connect.NewPipelineServiceHandler(s))
	mux.Handle(insightifyv1connect.NewLlmChatServiceHandler(s))
	return mux
}

// isProjectID checks whether the given string looks like a generated project ID.
func isProjectID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), "project-")
}
