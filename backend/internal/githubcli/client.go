package githubcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const outputLimit = 1 << 20

var ErrUnauthenticated = errors.New("GitHub CLI is not authenticated")

type PullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}
type PullRequestState struct {
	State  string `json:"state"`
	URL    string `json:"url"`
	Number int    `json:"number"`
}
type Check struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Link     string `json:"link"`
	Workflow string `json:"workflow"`
}

var pullRequestURLPattern = regexp.MustCompile(`https://github\.com/[^\s]+/pull/([0-9]+)`)

type Client struct {
	Binary  string
	Timeout time.Duration
}

// CheckAuth relies on gh's own credential store and never reads or persists a token.
func (c Client) CheckAuth(ctx context.Context, directory string) error {
	_, err := c.run(ctx, directory, "auth", "status")
	if err != nil {
		return ErrUnauthenticated
	}
	return nil
}

// Run executes gh directly with an argument vector and returns bounded output.
func (c Client) Run(ctx context.Context, directory string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("gh command is required")
	}
	return c.run(ctx, directory, args...)
}

func (c Client) CreatePullRequest(ctx context.Context, directory, title, body, branch string) (PullRequest, error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(body) == "" || strings.TrimSpace(branch) == "" {
		return PullRequest{}, errors.New("pull request title, body, and branch are required")
	}
	output, err := c.run(ctx, directory, "pr", "create", "--title", title, "--body", body, "--head", branch)
	if err != nil {
		if existing, findErr := c.FindPullRequest(ctx, directory, branch); findErr == nil {
			return existing, nil
		}
		return PullRequest{}, err
	}
	match := pullRequestURLPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return PullRequest{}, errors.New("gh pr create returned no pull request URL")
	}
	var number int
	_, err = fmt.Sscan(match[1], &number)
	if err != nil || number < 1 {
		return PullRequest{}, errors.New("gh pr create returned an invalid pull request number")
	}
	return PullRequest{Number: number, URL: match[0]}, nil
}

func (c Client) FindPullRequest(ctx context.Context, directory, branch string) (PullRequest, error) {
	if strings.TrimSpace(branch) == "" {
		return PullRequest{}, errors.New("pull request branch is required")
	}
	output, err := c.run(ctx, directory, "pr", "view", branch, "--json", "number,url")
	if err != nil {
		return PullRequest{}, err
	}
	var pull PullRequest
	if err := json.Unmarshal([]byte(output), &pull); err != nil || pull.Number < 1 || !pullRequestURLPattern.MatchString(pull.URL) {
		return PullRequest{}, errors.New("gh pr view returned invalid pull request data")
	}
	return pull, nil
}

func (c Client) ViewPullRequest(ctx context.Context, directory string, number int) (PullRequestState, error) {
	if number < 1 {
		return PullRequestState{}, errors.New("pull request number must be positive")
	}
	output, err := c.run(ctx, directory, "pr", "view", fmt.Sprint(number), "--json", "state,url,number")
	if err != nil {
		return PullRequestState{}, err
	}
	var state PullRequestState
	if err := json.Unmarshal([]byte(output), &state); err != nil || state.Number != number {
		return PullRequestState{}, errors.New("gh pr view returned invalid pull request state")
	}
	return state, nil
}

func (c Client) Checks(ctx context.Context, directory string, number int) ([]Check, error) {
	if number < 1 {
		return nil, errors.New("pull request number must be positive")
	}
	output, err := c.run(ctx, directory, "pr", "checks", fmt.Sprint(number), "--required", "--json", "name,state,link,workflow")
	if err != nil {
		return nil, err
	}
	var checks []Check
	if err := json.Unmarshal([]byte(output), &checks); err != nil {
		return nil, errors.New("gh pr checks returned invalid JSON")
	}
	return checks, nil
}

func (c Client) MergePullRequest(ctx context.Context, directory string, number int) error {
	if number < 1 {
		return errors.New("pull request number must be positive")
	}
	_, err := c.run(ctx, directory, "pr", "merge", fmt.Sprint(number), "--merge")
	return err
}

func (c Client) run(parent context.Context, directory string, args ...string) (string, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	binary := c.Binary
	if binary == "" {
		binary = "gh"
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = directory
	var stdout, stderr boundedBuffer
	stdout.limit, stderr.limit = outputLimit, 16<<10
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return stdout.String(), ctx.Err()
		}
		// Do not return raw CLI output: it can contain account or repository metadata.
		return stdout.String(), fmt.Errorf("gh %s failed", safeOperation(args))
	}
	return stdout.String(), nil
}

func safeOperation(args []string) string {
	parts := make([]string, 0, 2)
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		parts = append(parts, arg)
		if len(parts) == 2 {
			break
		}
	}
	return strings.Join(parts, " ")
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := b.limit - b.Len()
	if remaining > 0 {
		if remaining > len(value) {
			remaining = len(value)
		}
		_, _ = b.Buffer.Write(value[:remaining])
	}
	return written, nil
}
