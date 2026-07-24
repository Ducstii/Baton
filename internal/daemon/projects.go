package daemon

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Project represents an opened project.
type Project struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	BuildSystem string    `json:"build_system"`
	HasTests    bool      `json:"has_tests"`
	OpenedAt    time.Time `json:"opened_at"`
}

// OpenProjectRequest is the JSON body for POST /projects.
type OpenProjectRequest struct {
	Path string `json:"path"`
}

// ProjectStore is an in-memory, concurrency-safe store for projects.
type ProjectStore struct {
	mu       sync.RWMutex
	projects map[string]*Project
}

// NewProjectStore creates a new project store.
func NewProjectStore() *ProjectStore {
	return &ProjectStore{
		projects: make(map[string]*Project),
	}
}

// Add stores a project, keyed by path.
func (s *ProjectStore) Add(p *Project) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[p.Path] = p
}

// Get retrieves a project by path.
func (s *ProjectStore) Get(path string) (*Project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[path]
	return p, ok
}

// List returns all projects.
func (s *ProjectStore) List() []*Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Project, 0, len(s.projects))
	for _, p := range s.projects {
		result = append(result, p)
	}
	return result
}

// detectBuildSystem scans the given directory for known build files.
func detectBuildSystem(path string) string {
	type entry struct {
		name   string
		system string
	}
	entries := []entry{
		{"go.mod", "go"},
		{"package.json", "node"},
		{"Cargo.toml", "rust"},
		{"pom.xml", "maven"},
		{"build.gradle", "gradle"},
		{"Makefile", "make"},
		{"CMakeLists.txt", "cmake"},
		{"pyproject.toml", "python"},
		{"setup.py", "python"},
		{"Gemfile", "ruby"},
		{"mix.exs", "elixir"},
		{"composer.json", "php"},
	}
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(path, e.name)); err == nil {
			return e.system
		}
	}
	return "unknown"
}

// hasTests checks for the presence of test directories or test files.
func hasTests(projectPath string) bool {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && (name == "test" || name == "tests" || name == "__tests__") {
			return true
		}
		if strings.Contains(name, "_test") || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
			return true
		}
	}
	return false
}

func (d *Daemon) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects := d.projects.List()
	writeJSON(w, http.StatusOK, projects)
}

func (d *Daemon) handleOpenProject(w http.ResponseWriter, r *http.Request) {
	var req OpenProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "path is required")
		return
	}

	// Resolve to absolute path.
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_path", err.Error())
		return
	}

	// Check it exists and is a directory.
	info, err := os.Stat(absPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "path does not exist")
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "not_a_directory", "path is not a directory")
		return
	}

	project := &Project{
		Path:        absPath,
		Name:        filepath.Base(absPath),
		BuildSystem: detectBuildSystem(absPath),
		HasTests:    hasTests(absPath),
		OpenedAt:    time.Now(),
	}

	d.projects.Add(project)
	writeJSON(w, http.StatusCreated, project)
}
