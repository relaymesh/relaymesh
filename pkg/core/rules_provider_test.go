package core

import "testing"

// ---------------------------------------------------------------------------
// Bitbucket-style payloads (push.changes[0].new.name)
// ---------------------------------------------------------------------------

// TestBitbucketPushBranchMatch tests the common Bitbucket rule pattern that
// indexes into the changes array to check the target branch.
func TestBitbucketPushBranchMatch(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `push.changes[0].new.name == "main"`, Emit: EmitList{"push.main"}},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider: "bitbucket",
		Name:     "push",
		RawPayload: []byte(`{
			"push": {
				"changes": [
					{
						"new": {"name": "main", "type": "branch"},
						"old": {"name": "main", "type": "branch"}
					}
				]
			}
		}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Topic != "push.main" {
		t.Fatalf("expected topic push.main, got %q", matches[0].Topic)
	}
}

// TestBitbucketPushBranchNoMatch verifies that a push to a different branch
// does not trigger a rule that targets "main".
func TestBitbucketPushBranchNoMatch(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `push.changes[0].new.name == "main"`, Emit: EmitList{"push.main"}},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider: "bitbucket",
		Name:     "push",
		RawPayload: []byte(`{
			"push": {
				"changes": [
					{
						"new": {"name": "develop", "type": "branch"},
						"old": {"name": "develop", "type": "branch"}
					}
				]
			}
		}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

// TestBitbucketPushMultipleConditions tests combining array index access with
// a second nested condition in the same rule.
func TestBitbucketPushMultipleConditions(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{
				When: `push.changes[0].new.name == "main" && push.changes[0].new.type == "branch"`,
				Emit: EmitList{"push.main.branch"},
			},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider: "bitbucket",
		Name:     "push",
		RawPayload: []byte(`{
			"push": {
				"changes": [
					{
						"new": {"name": "main", "type": "branch"},
						"old": {"name": "main", "type": "branch"}
					}
				]
			}
		}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Topic != "push.main.branch" {
		t.Fatalf("expected topic push.main.branch, got %q", matches[0].Topic)
	}
}

// TestBitbucketPushTagNotBranch ensures a rule checking type == "branch"
// does not fire when the change is a tag.
func TestBitbucketPushTagNotBranch(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{
				When: `push.changes[0].new.type == "branch"`,
				Emit: EmitList{"push.branch"},
			},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider: "bitbucket",
		Name:     "push",
		RawPayload: []byte(`{
			"push": {
				"changes": [
					{
						"new": {"name": "v1.0.0", "type": "tag"}
					}
				]
			}
		}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches for tag push, got %d", len(matches))
	}
}

// TestBitbucketPushEmptyChanges verifies that the engine does not panic or
// match when the changes array is empty.
func TestBitbucketPushEmptyChanges(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `push.changes[0].new.name == "main"`, Emit: EmitList{"push.main"}},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider:   "bitbucket",
		Name:       "push",
		RawPayload: []byte(`{"push": {"changes": []}}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches for empty changes, got %d", len(matches))
	}
}

// TestBitbucketPushMissingNewField verifies graceful handling when the nested
// "new" object is null (branch delete event).
func TestBitbucketPushMissingNewField(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `push.changes[0].new.name == "main"`, Emit: EmitList{"push.main"}},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider:   "bitbucket",
		Name:       "push",
		RawPayload: []byte(`{"push": {"changes": [{"new": null, "old": {"name": "main"}}]}}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches when new is null, got %d", len(matches))
	}
}

// ---------------------------------------------------------------------------
// Single = operator (invalid syntax)
// ---------------------------------------------------------------------------

// TestSingleEqualsOperatorFails verifies that using = instead of == causes a
// compilation error. govaluate requires == for equality.
func TestSingleEqualsOperatorFails(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `push.changes[0].new.name = "main"`, Emit: EmitList{"push.main"}},
		},
	}

	_, err := NewRuleEngine(cfg)
	if err == nil {
		t.Fatalf("expected compilation error for single = operator, but got nil")
	}
}

// ---------------------------------------------------------------------------
// GitLab-style payloads
// ---------------------------------------------------------------------------

// TestGitLabPushRefMatch tests a GitLab push event matching on the ref field.
func TestGitLabPushRefMatch(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `object_kind == "push" && ref == "refs/heads/main"`, Emit: EmitList{"gl.push.main"}},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider: "gitlab",
		Name:     "push",
		RawPayload: []byte(`{
			"object_kind": "push",
			"ref": "refs/heads/main",
			"project": {"path_with_namespace": "org/repo"}
		}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Topic != "gl.push.main" {
		t.Fatalf("expected topic gl.push.main, got %q", matches[0].Topic)
	}
}

// TestGitLabMergeRequestOpened tests a GitLab merge request event with nested access.
func TestGitLabMergeRequestOpened(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{
				When: `object_kind == "merge_request" && object_attributes.action == "open" && object_attributes.target_branch == "main"`,
				Emit: EmitList{"gl.mr.opened"},
			},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider: "gitlab",
		Name:     "merge_request",
		RawPayload: []byte(`{
			"object_kind": "merge_request",
			"object_attributes": {
				"action": "open",
				"target_branch": "main",
				"source_branch": "feature/xyz"
			}
		}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

// ---------------------------------------------------------------------------
// GitHub-style payloads
// ---------------------------------------------------------------------------

// TestGitHubPushCommitMessage tests matching on the first commit message in a
// GitHub push event payload.
func TestGitHubPushCommitMessage(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `ref == "refs/heads/main"`, Emit: EmitList{"gh.push.main"}},
			{When: `commits[0].message == "fix: typo"`, Emit: EmitList{"gh.push.fix"}},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider: "github",
		Name:     "push",
		RawPayload: []byte(`{
			"ref": "refs/heads/main",
			"commits": [
				{"id": "abc123", "message": "fix: typo"},
				{"id": "def456", "message": "feat: add login"}
			]
		}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

// TestGitHubPRLabelsContains tests the contains function with a nested array
// field from a GitHub pull_request event.
func TestGitHubPRLabelsContains(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `action == "labeled" && contains(pull_request.labels, "urgent")`, Emit: EmitList{"pr.urgent"}},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider: "github",
		Name:     "pull_request",
		RawPayload: []byte(`{
			"action": "labeled",
			"pull_request": {
				"labels": ["bug", "urgent", "p0"]
			}
		}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

// TestGitHubRefLikePattern tests the like function for branch pattern matching
// on a GitHub push event.
func TestGitHubRefLikePattern(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `like(ref, "refs/heads/release/%")`, Emit: EmitList{"release.push"}},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	matched := Event{
		Provider:   "github",
		Name:       "push",
		RawPayload: []byte(`{"ref": "refs/heads/release/v1.2.3"}`),
	}
	if matches := engine.Evaluate(matched); len(matches) != 1 {
		t.Fatalf("expected 1 match for release branch, got %d", len(matches))
	}

	unmatched := Event{
		Provider:   "github",
		Name:       "push",
		RawPayload: []byte(`{"ref": "refs/heads/feature/login"}`),
	}
	if matches := engine.Evaluate(unmatched); len(matches) != 0 {
		t.Fatalf("expected 0 matches for feature branch, got %d", len(matches))
	}
}

// ---------------------------------------------------------------------------
// Cross-provider: multiple rules, same event
// ---------------------------------------------------------------------------

// TestMultipleProviderRulesSameEngine tests that rules for different provider
// payload shapes coexist in the same engine without interfering.
func TestMultipleProviderRulesSameEngine(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			// Bitbucket-style
			{When: `push.changes[0].new.name == "main"`, Emit: EmitList{"bb.push.main"}},
			// GitHub/GitLab-style
			{When: `ref == "refs/heads/main"`, Emit: EmitList{"gh.push.main"}},
			// Should not match either
			{When: `action == "opened"`, Emit: EmitList{"pr.opened"}},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	// Bitbucket push payload - only the BB rule should match.
	bbEvent := Event{
		Provider: "bitbucket",
		Name:     "push",
		RawPayload: []byte(`{
			"push": {"changes": [{"new": {"name": "main", "type": "branch"}}]}
		}`),
	}
	bbMatches := engine.Evaluate(bbEvent)
	if len(bbMatches) != 1 {
		t.Fatalf("bitbucket: expected 1 match, got %d", len(bbMatches))
	}
	if bbMatches[0].Topic != "bb.push.main" {
		t.Fatalf("bitbucket: expected topic bb.push.main, got %q", bbMatches[0].Topic)
	}

	// GitHub push payload - only the GH rule should match.
	ghEvent := Event{
		Provider: "github",
		Name:     "push",
		RawPayload: []byte(`{
			"ref": "refs/heads/main",
			"commits": []
		}`),
	}
	ghMatches := engine.Evaluate(ghEvent)
	if len(ghMatches) != 1 {
		t.Fatalf("github: expected 1 match, got %d", len(ghMatches))
	}
	if ghMatches[0].Topic != "gh.push.main" {
		t.Fatalf("github: expected topic gh.push.main, got %q", ghMatches[0].Topic)
	}
}

// ---------------------------------------------------------------------------
// Strict mode with nested paths
// ---------------------------------------------------------------------------

// TestStrictModeMissingNestedPath verifies that strict mode rejects evaluation
// when a deeply nested JSONPath variable cannot be resolved.
func TestStrictModeMissingNestedPath(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `push.changes[0].new.name == "main"`, Emit: EmitList{"push.main"}},
		},
		Strict: true,
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	// Payload has completely different structure; the path won't resolve.
	event := Event{
		Provider:   "bitbucket",
		Name:       "push",
		RawPayload: []byte(`{"action": "opened"}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches in strict mode with missing path, got %d", len(matches))
	}
}

// ---------------------------------------------------------------------------
// Dollar-prefixed JSONPath syntax
// ---------------------------------------------------------------------------

// TestDollarPrefixedArrayIndex tests the explicit $. JSONPath syntax with
// array indexing.
func TestDollarPrefixedArrayIndex(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `$.push.changes[0].new.name == "main"`, Emit: EmitList{"push.main"}},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider: "bitbucket",
		Name:     "push",
		RawPayload: []byte(`{
			"push": {"changes": [{"new": {"name": "main"}}]}
		}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match with $ prefix, got %d", len(matches))
	}
}

// TestDollarPrefixAndBareEquivalent verifies that $.path and bare path produce
// the same result.
func TestDollarPrefixAndBareEquivalent(t *testing.T) {
	bareCfg := RulesConfig{
		Rules: []Rule{
			{When: `push.changes[0].new.name == "main"`, Emit: EmitList{"bare"}},
		},
	}
	dollarCfg := RulesConfig{
		Rules: []Rule{
			{When: `$.push.changes[0].new.name == "main"`, Emit: EmitList{"dollar"}},
		},
	}

	bareEngine, err := NewRuleEngine(bareCfg)
	if err != nil {
		t.Fatalf("bare engine: %v", err)
	}
	dollarEngine, err := NewRuleEngine(dollarCfg)
	if err != nil {
		t.Fatalf("dollar engine: %v", err)
	}

	event := Event{
		Provider:   "bitbucket",
		Name:       "push",
		RawPayload: []byte(`{"push": {"changes": [{"new": {"name": "main"}}]}}`),
	}

	bareMatches := bareEngine.Evaluate(event)
	dollarMatches := dollarEngine.Evaluate(event)

	if len(bareMatches) != len(dollarMatches) {
		t.Fatalf("bare matches=%d dollar matches=%d; expected same count", len(bareMatches), len(dollarMatches))
	}
	if len(bareMatches) != 1 {
		t.Fatalf("expected 1 match from both, got %d", len(bareMatches))
	}
}

// ---------------------------------------------------------------------------
// Second array index
// ---------------------------------------------------------------------------

// TestSecondArrayElement tests accessing a non-zero array index.
func TestSecondArrayElement(t *testing.T) {
	cfg := RulesConfig{
		Rules: []Rule{
			{When: `push.changes[1].new.name == "develop"`, Emit: EmitList{"push.develop"}},
		},
	}

	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatalf("new rule engine: %v", err)
	}

	event := Event{
		Provider: "bitbucket",
		Name:     "push",
		RawPayload: []byte(`{
			"push": {
				"changes": [
					{"new": {"name": "main"}},
					{"new": {"name": "develop"}}
				]
			}
		}`),
	}

	matches := engine.Evaluate(event)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match on second element, got %d", len(matches))
	}
}
