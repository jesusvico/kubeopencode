// Copyright Contributors to the KubeOpenCode project

package controller

import (
	"encoding/json"
	"testing"

	kubeopenv1alpha1 "github.com/kubeopencode/kubeopencode/api/v1alpha1"
)

func TestProcessSkills(t *testing.T) {
	t.Run("empty skills returns nil", func(t *testing.T) {
		gitMounts, skillPaths := processSkills(nil)
		if gitMounts != nil {
			t.Errorf("expected nil gitMounts, got %v", gitMounts)
		}
		if skillPaths != nil {
			t.Errorf("expected nil skillPaths, got %v", skillPaths)
		}
	})

	t.Run("single skill without names filter", func(t *testing.T) {
		depth := 1
		skills := []kubeopenv1alpha1.SkillSource{
			{
				Name: "my-skills",
				Git: &kubeopenv1alpha1.GitSkillSource{
					Repository: "https://github.com/anthropics/skills.git",
					Ref:        "main",
					Path:       "skills/",
					Depth:      &depth,
				},
			},
		}

		gitMounts, skillPaths := processSkills(skills)

		if len(gitMounts) != 1 {
			t.Fatalf("expected 1 gitMount, got %d", len(gitMounts))
		}
		gm := gitMounts[0]
		if gm.contextName != "skill-my-skills" {
			t.Errorf("contextName = %q, want %q", gm.contextName, "skill-my-skills")
		}
		if gm.repository != "https://github.com/anthropics/skills.git" {
			t.Errorf("repository = %q", gm.repository)
		}
		if gm.ref != "main" {
			t.Errorf("ref = %q, want %q", gm.ref, "main")
		}
		if gm.repoPath != "skills/" {
			t.Errorf("repoPath = %q, want %q", gm.repoPath, "skills/")
		}
		if gm.mountPath != "/skills/my-skills" {
			t.Errorf("mountPath = %q, want %q", gm.mountPath, "/skills/my-skills")
		}
		if gm.depth != 1 {
			t.Errorf("depth = %d, want %d", gm.depth, 1)
		}

		if len(skillPaths) != 1 {
			t.Fatalf("expected 1 skillPath, got %d", len(skillPaths))
		}
		if skillPaths[0] != "/skills/my-skills" {
			t.Errorf("skillPath = %q, want %q", skillPaths[0], "/skills/my-skills")
		}
	})

	t.Run("skill with names filter", func(t *testing.T) {
		skills := []kubeopenv1alpha1.SkillSource{
			{
				Name: "official",
				Git: &kubeopenv1alpha1.GitSkillSource{
					Repository: "https://github.com/anthropics/skills.git",
					Path:       "skills/",
					Names:      []string{"frontend-design", "webapp-testing"},
				},
			},
		}

		gitMounts, skillPaths := processSkills(skills)

		if len(gitMounts) != 1 {
			t.Fatalf("expected 1 gitMount, got %d", len(gitMounts))
		}

		if len(skillPaths) != 2 {
			t.Fatalf("expected 2 skillPaths, got %d", len(skillPaths))
		}
		if skillPaths[0] != "/skills/official/frontend-design" {
			t.Errorf("skillPath[0] = %q, want %q", skillPaths[0], "/skills/official/frontend-design")
		}
		if skillPaths[1] != "/skills/official/webapp-testing" {
			t.Errorf("skillPath[1] = %q, want %q", skillPaths[1], "/skills/official/webapp-testing")
		}
	})

	t.Run("multiple skills", func(t *testing.T) {
		skills := []kubeopenv1alpha1.SkillSource{
			{
				Name: "source-a",
				Git: &kubeopenv1alpha1.GitSkillSource{
					Repository: "https://github.com/org/skills-a.git",
				},
			},
			{
				Name: "source-b",
				Git: &kubeopenv1alpha1.GitSkillSource{
					Repository: "https://github.com/org/skills-b.git",
					Names:      []string{"skill-x"},
				},
			},
		}

		gitMounts, skillPaths := processSkills(skills)

		if len(gitMounts) != 2 {
			t.Fatalf("expected 2 gitMounts, got %d", len(gitMounts))
		}
		if len(skillPaths) != 2 {
			t.Fatalf("expected 2 skillPaths, got %d", len(skillPaths))
		}
		if skillPaths[0] != "/skills/source-a" {
			t.Errorf("skillPath[0] = %q", skillPaths[0])
		}
		if skillPaths[1] != "/skills/source-b/skill-x" {
			t.Errorf("skillPath[1] = %q", skillPaths[1])
		}
	})

	t.Run("skill with secretRef", func(t *testing.T) {
		skills := []kubeopenv1alpha1.SkillSource{
			{
				Name: "private",
				Git: &kubeopenv1alpha1.GitSkillSource{
					Repository: "https://github.com/org/private-skills.git",
					SecretRef:  &kubeopenv1alpha1.GitSecretReference{Name: "git-creds"},
				},
			},
		}

		gitMounts, _ := processSkills(skills)

		if len(gitMounts) != 1 {
			t.Fatalf("expected 1 gitMount, got %d", len(gitMounts))
		}
		if gitMounts[0].secretName != "git-creds" {
			t.Errorf("secretName = %q, want %q", gitMounts[0].secretName, "git-creds")
		}
	})

	t.Run("skill with nil git is skipped", func(t *testing.T) {
		skills := []kubeopenv1alpha1.SkillSource{
			{Name: "empty"},
		}

		gitMounts, skillPaths := processSkills(skills)

		if len(gitMounts) != 0 {
			t.Errorf("expected 0 gitMounts, got %d", len(gitMounts))
		}
		if len(skillPaths) != 0 {
			t.Errorf("expected 0 skillPaths, got %d", len(skillPaths))
		}
	})

	t.Run("default ref is HEAD", func(t *testing.T) {
		skills := []kubeopenv1alpha1.SkillSource{
			{
				Name: "test",
				Git: &kubeopenv1alpha1.GitSkillSource{
					Repository: "https://github.com/org/repo.git",
				},
			},
		}

		gitMounts, _ := processSkills(skills)
		if gitMounts[0].ref != "HEAD" {
			t.Errorf("ref = %q, want %q", gitMounts[0].ref, "HEAD")
		}
	})
}

func TestInjectSkillsIntoConfig(t *testing.T) {
	t.Run("nil config creates new config", func(t *testing.T) {
		result, err := injectSkillsIntoConfig(nil, []string{"/skills/a"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(*result), &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		skills := parsed["skills"].(map[string]interface{})
		paths := skills["paths"].([]interface{})
		if len(paths) != 1 || paths[0].(string) != "/skills/a" {
			t.Errorf("paths = %v, want [/skills/a]", paths)
		}
	})

	t.Run("empty config creates new config", func(t *testing.T) {
		empty := ""
		result, err := injectSkillsIntoConfig(&empty, []string{"/skills/a"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(*result), &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		skills := parsed["skills"].(map[string]interface{})
		paths := skills["paths"].([]interface{})
		if len(paths) != 1 {
			t.Errorf("expected 1 path, got %d", len(paths))
		}
	})

	t.Run("preserves existing config fields", func(t *testing.T) {
		existing := `{"model":"claude","someField":"value"}`
		result, err := injectSkillsIntoConfig(&existing, []string{"/skills/a"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(*result), &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		if parsed["model"] != "claude" {
			t.Errorf("model field lost: %v", parsed)
		}
		if parsed["someField"] != "value" {
			t.Errorf("someField lost: %v", parsed)
		}
	})

	t.Run("appends to existing skills.paths", func(t *testing.T) {
		existing := `{"skills":{"paths":["/existing/path"]}}`
		result, err := injectSkillsIntoConfig(&existing, []string{"/skills/new"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(*result), &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		skills := parsed["skills"].(map[string]interface{})
		paths := skills["paths"].([]interface{})
		if len(paths) != 2 {
			t.Errorf("expected 2 paths, got %d: %v", len(paths), paths)
		}
	})

	t.Run("deduplicates paths", func(t *testing.T) {
		existing := `{"skills":{"paths":["/skills/a"]}}`
		result, err := injectSkillsIntoConfig(&existing, []string{"/skills/a", "/skills/b"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(*result), &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		skills := parsed["skills"].(map[string]interface{})
		paths := skills["paths"].([]interface{})
		if len(paths) != 2 {
			t.Errorf("expected 2 paths (deduped), got %d: %v", len(paths), paths)
		}
	})

	t.Run("preserves skills.urls", func(t *testing.T) {
		existing := `{"skills":{"urls":["https://example.com/skills/"]}}`
		result, err := injectSkillsIntoConfig(&existing, []string{"/skills/a"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(*result), &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		skills := parsed["skills"].(map[string]interface{})
		urls := skills["urls"].([]interface{})
		if len(urls) != 1 || urls[0].(string) != "https://example.com/skills/" {
			t.Errorf("urls lost: %v", urls)
		}
	})

	t.Run("empty skillPaths returns original config", func(t *testing.T) {
		existing := `{"model":"claude"}`
		result, err := injectSkillsIntoConfig(&existing, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if *result != existing {
			t.Errorf("expected original config, got %q", *result)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		invalid := `{invalid`
		_, err := injectSkillsIntoConfig(&invalid, []string{"/skills/a"})
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}
