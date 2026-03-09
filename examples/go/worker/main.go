package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/google/go-github/v57/github"
	"github.com/ktrysmt/go-bitbucket"
	"github.com/xanzy/go-gitlab"
	worker "github.com/relaymesh/githook/sdk/go/worker"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	endpoint := os.Getenv("GITHOOK_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://relaymesh.vercel.app/api/connect"
	}
	ruleID := os.Getenv("GITHOOK_RULE_ID")
	if ruleID == "" {
		ruleID = "85101e9f-3bcf-4ed0-b561-750c270ef6c3"
	}
	concurrency := intFromEnv("GITHOOK_CONCURRENCY", 4)
	retryCount := intFromEnv("GITHOOK_RETRY_COUNT", 1)

	wk := worker.New(
		worker.WithEndpoint(endpoint),
		worker.WithClientProvider(worker.NewRemoteSCMClientProvider()),
		worker.WithConcurrency(concurrency),
		worker.WithRetryCount(retryCount),
		worker.WithLogger(exampleLogger{}),
		worker.WithListener(worker.Listener{
			OnMessageStart: func(_ context.Context, evt *worker.Event) {
				if evt == nil {
					return
				}
				log.Printf("listener start log_id=%s provider=%s topic=%s", evt.Metadata["log_id"], evt.Provider, evt.Topic)
			},
			OnMessageFinish: func(_ context.Context, evt *worker.Event, err error) {
				if evt == nil {
					return
				}
				status := "success"
				if err != nil {
					status = "failed"
				}
				log.Printf("listener finish log_id=%s status=%s err=%v", evt.Metadata["log_id"], status, err)
			},
			OnError: func(_ context.Context, evt *worker.Event, err error) {
				if evt == nil {
					log.Printf("listener error err=%v", err)
					return
				}
				log.Printf("listener error log_id=%s provider=%s err=%v", evt.Metadata["log_id"], evt.Provider, err)
			},
		}),
	)

	wk.HandleRule(ruleID, func(ctx context.Context, evt *worker.Event) error {
		if evt == nil {
			return nil
		}

		provider := strings.ToLower(strings.TrimSpace(evt.Provider))
		log.Printf("handler topic=%s provider=%s type=%s retry_count=%d concurrency=%d", evt.Topic, provider, evt.Type, retryCount, concurrency)
		if payload, err := json.Marshal(evt.Normalized); err == nil {
			log.Printf("event normalized=%s", string(payload))
		}

		switch provider {
		case "github":
			ghClient, ok := worker.GitHubClient(evt)
			if !ok {
				log.Printf("github client not available (installation may not be configured)")
				return nil
			}
			owner, repo := repositoryFromEvent(evt)
			if owner == "" || repo == "" {
				log.Printf("repository info missing in payload; skipping github read")
				return nil
			}
			ghCommits, _, err := ghClient.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
				ListOptions: github.ListOptions{PerPage: 5},
			})
			if err != nil {
				log.Printf("github list commits failed owner=%s repo=%s err=%v", owner, repo, err)
				return nil
			}
			log.Printf("github commits count=%d", len(ghCommits))
			for i, c := range ghCommits {
				sha := c.GetSHA()
				if len(sha) > 7 {
					sha = sha[:7]
				}
				msg := firstLine(c.Commit.GetMessage())
				log.Printf("  commit[%d] sha=%s message=%s", i+1, sha, msg)
			}
		case "gitlab":
			glClient, ok := worker.GitLabClient(evt)
			if !ok {
				log.Printf("gitlab client not available (installation may not be configured)")
				return nil
			}
			owner, repo := repositoryFromEvent(evt)
			if owner == "" || repo == "" {
				log.Printf("repository info missing in payload; skipping gitlab read")
				return nil
			}
			glCommits, _, err := glClient.Commits.ListCommits(fmt.Sprintf("%s/%s", owner, repo), &gitlab.ListCommitsOptions{
				ListOptions: gitlab.ListOptions{PerPage: 5},
			})
			if err != nil {
				log.Printf("gitlab list commits failed project=%s/%s err=%v", owner, repo, err)
				return nil
			}
			log.Printf("gitlab commits count=%d", len(glCommits))
			for i, c := range glCommits {
				log.Printf("  commit[%d] sha=%s message=%s", i+1, c.ShortID, firstLine(c.Title))
			}
		case "bitbucket":
			bbClient, ok := worker.BitbucketClient(evt)
			if !ok {
				log.Printf("bitbucket client not available (installation may not be configured)")
				return nil
			}
			owner, repo := repositoryFromEvent(evt)
			if owner == "" || repo == "" {
				log.Printf("repository info missing in payload; skipping bitbucket read")
				return nil
			}
			result, err := bbClient.Repositories.Commits.GetCommits(&bitbucket.CommitsOptions{
				Owner:    owner,
				RepoSlug: repo,
			})
			if err != nil {
				log.Printf("bitbucket list commits failed owner=%s repo=%s err=%v", owner, repo, err)
				return nil
			}
			logBitbucketCommits(result, 5)
		default:
			log.Printf("unsupported provider=%s; skipping scm call", provider)
		}
		
		return nil
	})

	if err := wk.Run(ctx); err != nil {
		log.Fatalf("worker run failed: %v", err)
	}
}

type exampleLogger struct{}

func (exampleLogger) Printf(format string, args ...interface{}) {
	log.Printf("example-worker "+format, args...)
}

func intFromEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func repositoryFromEvent(evt *worker.Event) (string, string) {
	if evt == nil || evt.Normalized == nil {
		return "", ""
	}

	repoValue, ok := evt.Normalized["repository"]
	if !ok {
		return "", ""
	}
	repoMap, ok := repoValue.(map[string]interface{})
	if !ok {
		return "", ""
	}

	if fullName, ok := repoMap["full_name"].(string); ok {
		parts := strings.SplitN(strings.TrimSpace(fullName), "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1]
		}
	}

	name, _ := repoMap["name"].(string)
	ownerMap, _ := repoMap["owner"].(map[string]interface{})
	owner, _ := ownerMap["login"].(string)

	return strings.TrimSpace(owner), strings.TrimSpace(name)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func logBitbucketCommits(result interface{}, limit int) {
	data, ok := result.(map[string]interface{})
	if !ok {
		log.Printf("bitbucket commits: unexpected response type")
		return
	}
	values, ok := data["values"].([]interface{})
	if !ok {
		log.Printf("bitbucket commits: no values in response")
		return
	}
	if len(values) > limit {
		values = values[:limit]
	}
	log.Printf("bitbucket commits count=%d", len(values))
	for i, v := range values {
		cm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		hash, _ := cm["hash"].(string)
		if len(hash) > 7 {
			hash = hash[:7]
		}
		msg := firstLine(fmt.Sprintf("%v", cm["message"]))
		log.Printf("  commit[%d] sha=%s message=%s", i+1, hash, msg)
	}
}
