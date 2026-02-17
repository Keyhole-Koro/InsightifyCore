package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"insightify/internal/artifact"
	"insightify/internal/common/scan"
)

type githubCloneTool struct{ host Host }

func newGitHubCloneTool(h Host) *githubCloneTool { return &githubCloneTool{host: h} }

func (t *githubCloneTool) Spec() artifact.ToolSpec {
	return artifact.ToolSpec{
		Name:        "github.clone",
		Description: "Clone a GitHub repository into repos root.",
	}
}

type githubCloneInput struct {
	RepoURL    string `json:"repo_url"`
	Branch     string `json:"branch"`
	Depth      int    `json:"depth"`
	TargetName string `json:"target_name"`
	IfExists   string `json:"if_exists"`
}

type githubCloneOutput struct {
	RepoName string `json:"repo_name"`
	RepoPath string `json:"repo_path"`
	Status   string `json:"status"`
}

// runGitCommand is injectable in tests.
var runGitCommand = func(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (t *githubCloneTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in githubCloneInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	cloneURL, inferredName, err := normalizeGitHubCloneInput(in.RepoURL)
	if err != nil {
		return nil, err
	}

	targetName := strings.TrimSpace(in.TargetName)
	if targetName == "" {
		targetName = inferredName
	}
	if err := validateRepoDirName(targetName); err != nil {
		return nil, err
	}

	ifExists := strings.ToLower(strings.TrimSpace(in.IfExists))
	if ifExists == "" {
		ifExists = "skip"
	}
	if ifExists != "skip" && ifExists != "error" && ifExists != "pull" {
		return nil, fmt.Errorf("github.clone: invalid if_exists %q", in.IfExists)
	}

	reposRoot := strings.TrimSpace(t.host.ReposRoot)
	if reposRoot == "" {
		reposRoot = scan.ReposDir()
	}
	if reposRoot == "" {
		return nil, fmt.Errorf("github.clone: repos root is not configured")
	}

	if err := os.MkdirAll(reposRoot, 0o755); err != nil {
		return nil, fmt.Errorf("github.clone: mkdir repos root: %w", err)
	}

	targetPath := filepath.Join(reposRoot, targetName)
	if st, err := os.Stat(targetPath); err == nil && st.IsDir() {
		switch ifExists {
		case "skip":
			return json.Marshal(githubCloneOutput{RepoName: targetName, RepoPath: targetPath, Status: "skipped"})
		case "error":
			return nil, fmt.Errorf("github.clone: target already exists: %s", targetPath)
		case "pull":
			pullArgs := []string{"-C", targetPath, "pull", "--ff-only"}
			if b := strings.TrimSpace(in.Branch); b != "" {
				pullArgs = append(pullArgs, "origin", b)
			}
			if err := runGitCommand(ctx, pullArgs...); err != nil {
				return nil, err
			}
			return json.Marshal(githubCloneOutput{RepoName: targetName, RepoPath: targetPath, Status: "updated"})
		}
	}

	depth := in.Depth
	if depth < 0 {
		return nil, fmt.Errorf("github.clone: depth must be >= 0")
	}
	if depth == 0 {
		depth = 1
	}

	args := []string{"clone", "--depth", strconv.Itoa(depth)}
	if b := strings.TrimSpace(in.Branch); b != "" {
		args = append(args, "--branch", b, "--single-branch")
	}
	args = append(args, cloneURL, targetPath)
	if err := runGitCommand(ctx, args...); err != nil {
		return nil, err
	}

	return json.Marshal(githubCloneOutput{RepoName: targetName, RepoPath: targetPath, Status: "cloned"})
}

func normalizeGitHubCloneInput(raw string) (cloneURL, repoName string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("github.clone: repo_url required")
	}

	if strings.HasPrefix(raw, "git@github.com:") {
		repoPath := strings.TrimPrefix(raw, "git@github.com:")
		repoPath = strings.TrimSuffix(repoPath, ".git")
		owner, repo, ok := splitOwnerRepo(repoPath)
		if !ok {
			return "", "", fmt.Errorf("github.clone: invalid github repo_url %q", raw)
		}
		return fmt.Sprintf("git@github.com:%s/%s.git", owner, repo), repo, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("github.clone: invalid repo_url: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(u.Host), "github.com") {
		return "", "", fmt.Errorf("github.clone: only github.com is supported")
	}
	repoPath := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
	owner, repo, ok := splitOwnerRepo(repoPath)
	if !ok {
		return "", "", fmt.Errorf("github.clone: invalid github repo_url %q", raw)
	}
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), repo, nil
}

func splitOwnerRepo(repoPath string) (owner, repo string, ok bool) {
	repoPath = strings.Trim(repoPath, "/")
	parts := strings.Split(repoPath, "/")
	if len(parts) < 2 {
		return "", "", false
	}
	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSpace(parts[1])
	if owner == "" || repo == "" {
		return "", "", false
	}
	return owner, repo, true
}

func validateRepoDirName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("github.clone: target_name is empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("github.clone: invalid target_name %q", name)
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("github.clone: target_name must be a single path segment")
	}
	if path.Clean(name) != name {
		return fmt.Errorf("github.clone: invalid target_name %q", name)
	}
	return nil
}
