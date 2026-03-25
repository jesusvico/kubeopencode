// Copyright Contributors to the KubeOpenCode project

//go:build !integration

package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubeopenv1alpha1 "github.com/kubeopencode/kubeopencode/api/v1alpha1"
)

func TestConfigHasPermission(t *testing.T) {
	tests := []struct {
		name   string
		config *string
		want   bool
	}{
		{
			name:   "nil config",
			config: nil,
			want:   false,
		},
		{
			name:   "empty config",
			config: stringPtr(""),
			want:   false,
		},
		{
			name:   "config with permission field",
			config: stringPtr(`{"permission": "ask", "model": "gpt-4"}`),
			want:   true,
		},
		{
			name:   "config without permission field",
			config: stringPtr(`{"model": "gpt-4", "small_model": "gpt-3.5"}`),
			want:   false,
		},
		{
			name:   "invalid JSON",
			config: stringPtr(`{invalid json`),
			want:   false,
		},
		{
			name:   "permission field is null",
			config: stringPtr(`{"permission": null, "model": "gpt-4"}`),
			want:   true, // field exists even if value is null
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configHasPermission(tt.config)
			if got != tt.want {
				t.Errorf("configHasPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildServerDeployment_WithCredentials(t *testing.T) {
	agent := &kubeopenv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: kubeopenv1alpha1.AgentSpec{
			ServerConfig: &kubeopenv1alpha1.ServerConfig{
				Port: 4096,
			},
		},
	}

	envName := "GITHUB_TOKEN"
	mountPath := "/home/agent/.ssh/id_rsa"

	cfg := agentConfig{
		executorImage: "test-executor:v1.0.0",
		agentImage:    "test-agent:v1.0.0",
		workspaceDir:  "/workspace",
		credentials: []kubeopenv1alpha1.Credential{
			{
				Name: "github-token",
				SecretRef: kubeopenv1alpha1.SecretReference{
					Name: "github-secret",
					Key:  stringPtr("token"),
				},
				Env: &envName,
			},
			{
				Name: "ssh-key",
				SecretRef: kubeopenv1alpha1.SecretReference{
					Name: "ssh-secret",
					Key:  stringPtr("private-key"),
				},
				MountPath: &mountPath,
			},
		},
	}

	deployment := BuildServerDeployment(agent, cfg, defaultSystemConfig(), nil, nil, nil, nil)

	if deployment == nil {
		t.Fatal("BuildServerDeployment returned nil")
	}

	container := deployment.Spec.Template.Spec.Containers[0]

	// Verify env credential
	var foundEnvCred bool
	for _, env := range container.Env {
		if env.Name == "GITHUB_TOKEN" {
			foundEnvCred = true
			if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
				t.Errorf("GITHUB_TOKEN env should have SecretKeyRef")
			} else {
				if env.ValueFrom.SecretKeyRef.Name != "github-secret" {
					t.Errorf("SecretKeyRef.Name = %q, want %q", env.ValueFrom.SecretKeyRef.Name, "github-secret")
				}
				if env.ValueFrom.SecretKeyRef.Key != "token" {
					t.Errorf("SecretKeyRef.Key = %q, want %q", env.ValueFrom.SecretKeyRef.Key, "token")
				}
			}
		}
	}
	if !foundEnvCred {
		t.Errorf("GITHUB_TOKEN env not found")
	}

	// Verify mount credential
	var foundMountCred bool
	for _, mount := range container.VolumeMounts {
		if mount.MountPath == "/home/agent/.ssh/id_rsa" {
			foundMountCred = true
		}
	}
	if !foundMountCred {
		t.Errorf("SSH key mount not found at /home/agent/.ssh/id_rsa")
	}

	// Verify volume exists
	var foundVolume bool
	for _, vol := range deployment.Spec.Template.Spec.Volumes {
		if vol.Secret != nil && vol.Secret.SecretName == "ssh-secret" {
			foundVolume = true
		}
	}
	if !foundVolume {
		t.Errorf("Secret volume for ssh-secret not found")
	}
}

func TestBuildServerDeployment_WithEntireSecretCredential(t *testing.T) {
	agent := &kubeopenv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: kubeopenv1alpha1.AgentSpec{
			ServerConfig: &kubeopenv1alpha1.ServerConfig{
				Port: 4096,
			},
		},
	}

	cfg := agentConfig{
		executorImage: "test-executor:v1.0.0",
		agentImage:    "test-agent:v1.0.0",
		workspaceDir:  "/workspace",
		credentials: []kubeopenv1alpha1.Credential{
			{
				// No Key specified - mount entire secret as env vars
				Name: "api-keys",
				SecretRef: kubeopenv1alpha1.SecretReference{
					Name: "api-credentials",
				},
			},
		},
	}

	deployment := BuildServerDeployment(agent, cfg, defaultSystemConfig(), nil, nil, nil, nil)

	if deployment == nil {
		t.Fatal("BuildServerDeployment returned nil")
	}

	container := deployment.Spec.Template.Spec.Containers[0]

	// Verify envFrom is set with secretRef
	if len(container.EnvFrom) != 1 {
		t.Fatalf("Expected 1 envFrom entry, got %d", len(container.EnvFrom))
	}

	envFrom := container.EnvFrom[0]
	if envFrom.SecretRef == nil {
		t.Errorf("EnvFrom.SecretRef should not be nil")
	} else if envFrom.SecretRef.Name != "api-credentials" {
		t.Errorf("EnvFrom.SecretRef.Name = %q, want %q", envFrom.SecretRef.Name, "api-credentials")
	}
}

func TestBuildServerDeployment_WithHOMEAndSHELLEnvVars(t *testing.T) {
	agent := &kubeopenv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: kubeopenv1alpha1.AgentSpec{
			ServerConfig: &kubeopenv1alpha1.ServerConfig{
				Port: 4096,
			},
		},
	}

	cfg := agentConfig{
		executorImage: "test-executor:v1.0.0",
		agentImage:    "test-agent:v1.0.0",
		workspaceDir:  "/workspace",
	}

	deployment := BuildServerDeployment(agent, cfg, defaultSystemConfig(), nil, nil, nil, nil)

	if deployment == nil {
		t.Fatal("BuildServerDeployment returned nil")
	}

	container := deployment.Spec.Template.Spec.Containers[0]

	// Verify HOME env var
	var foundHOME bool
	for _, env := range container.Env {
		if env.Name == "HOME" {
			foundHOME = true
			if env.Value != DefaultHomeDir {
				t.Errorf("HOME = %q, want %q", env.Value, DefaultHomeDir)
			}
		}
	}
	if !foundHOME {
		t.Errorf("HOME env var not found")
	}

	// Verify SHELL env var
	var foundSHELL bool
	for _, env := range container.Env {
		if env.Name == "SHELL" {
			foundSHELL = true
			if env.Value != DefaultShell {
				t.Errorf("SHELL = %q, want %q", env.Value, DefaultShell)
			}
		}
	}
	if !foundSHELL {
		t.Errorf("SHELL env var not found")
	}
}

func TestBuildServerDeployment_WithTextContext(t *testing.T) {
	agent := &kubeopenv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: kubeopenv1alpha1.AgentSpec{
			ServerConfig: &kubeopenv1alpha1.ServerConfig{
				Port: 4096,
			},
		},
	}

	cfg := agentConfig{
		executorImage: "test-executor:v1.0.0",
		agentImage:    "test-agent:v1.0.0",
		workspaceDir:  "/workspace",
	}

	contextConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent-server-context",
			Namespace: "default",
		},
		Data: map[string]string{
			"workspace-.kubeopencode-context.md": "<context>test content</context>",
		},
	}

	fileMounts := []fileMount{
		{filePath: "/workspace/.kubeopencode/context.md"},
	}

	deployment := BuildServerDeployment(agent, cfg, defaultSystemConfig(), contextConfigMap, fileMounts, nil, nil)

	if deployment == nil {
		t.Fatal("BuildServerDeployment returned nil")
	}

	// Verify context-files volume exists
	var foundContextVolume bool
	for _, vol := range deployment.Spec.Template.Spec.Volumes {
		if vol.Name == "context-files" && vol.ConfigMap != nil {
			foundContextVolume = true
			if vol.ConfigMap.Name != "test-agent-server-context" {
				t.Errorf("context-files volume ConfigMap.Name = %q, want %q", vol.ConfigMap.Name, "test-agent-server-context")
			}
		}
	}
	if !foundContextVolume {
		t.Errorf("context-files volume not found")
	}

	// Verify context-init container exists
	var foundContextInit bool
	for _, ic := range deployment.Spec.Template.Spec.InitContainers {
		if ic.Name == "context-init" {
			foundContextInit = true
		}
	}
	if !foundContextInit {
		t.Errorf("context-init init container not found")
	}

	// Verify OPENCODE_CONFIG_CONTENT env var is set
	container := deployment.Spec.Template.Spec.Containers[0]
	var foundConfigContentEnv bool
	for _, env := range container.Env {
		if env.Name == OpenCodeConfigContentEnvVar {
			foundConfigContentEnv = true
			expectedValue := `{"instructions":["` + ContextFileRelPath + `"]}`
			if env.Value != expectedValue {
				t.Errorf("OPENCODE_CONFIG_CONTENT = %q, want %q", env.Value, expectedValue)
			}
		}
	}
	if !foundConfigContentEnv {
		t.Errorf("OPENCODE_CONFIG_CONTENT env var not found")
	}
}

func TestBuildServerDeployment_WithConfigMapContext(t *testing.T) {
	agent := &kubeopenv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: kubeopenv1alpha1.AgentSpec{
			ServerConfig: &kubeopenv1alpha1.ServerConfig{
				Port: 4096,
			},
		},
	}

	cfg := agentConfig{
		executorImage: "test-executor:v1.0.0",
		agentImage:    "test-agent:v1.0.0",
		workspaceDir:  "/workspace",
	}

	dirMounts := []dirMount{
		{
			dirPath:       "/workspace/guides",
			configMapName: "guides-configmap",
			optional:      true,
		},
	}

	deployment := BuildServerDeployment(agent, cfg, defaultSystemConfig(), nil, nil, dirMounts, nil)

	if deployment == nil {
		t.Fatal("BuildServerDeployment returned nil")
	}

	// Verify dir-mount volume exists
	var foundDirVolume bool
	for _, vol := range deployment.Spec.Template.Spec.Volumes {
		if vol.Name == "dir-mount-0" && vol.ConfigMap != nil {
			foundDirVolume = true
			if vol.ConfigMap.Name != "guides-configmap" {
				t.Errorf("dir-mount-0 volume ConfigMap.Name = %q, want %q", vol.ConfigMap.Name, "guides-configmap")
			}
			if vol.ConfigMap.Optional == nil || *vol.ConfigMap.Optional != true {
				t.Errorf("dir-mount-0 volume ConfigMap.Optional = %v, want true", vol.ConfigMap.Optional)
			}
		}
	}
	if !foundDirVolume {
		t.Errorf("dir-mount-0 volume not found")
	}

	// Verify context-init container exists and mounts the ConfigMap
	var foundContextInit bool
	for _, ic := range deployment.Spec.Template.Spec.InitContainers {
		if ic.Name == "context-init" {
			foundContextInit = true
			// Verify init container mounts the dir-mount ConfigMap
			var foundDirMount bool
			for _, mount := range ic.VolumeMounts {
				if mount.Name == "dir-mount-0" && mount.MountPath == "/configmap-dir-0" {
					foundDirMount = true
				}
			}
			if !foundDirMount {
				t.Errorf("context-init container should mount dir-mount-0 ConfigMap at /configmap-dir-0")
			}
		}
	}
	if !foundContextInit {
		t.Errorf("context-init init container not found")
	}
}

func TestBuildServerDeployment_WithGitContext(t *testing.T) {
	agent := &kubeopenv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: kubeopenv1alpha1.AgentSpec{
			ServerConfig: &kubeopenv1alpha1.ServerConfig{
				Port: 4096,
			},
		},
	}

	cfg := agentConfig{
		executorImage: "test-executor:v1.0.0",
		agentImage:    "test-agent:v1.0.0",
		workspaceDir:  "/workspace",
	}

	gitMounts := []gitMount{
		{
			contextName: "my-context",
			repository:  "https://github.com/org/repo.git",
			ref:         "main",
			repoPath:    "",
			mountPath:   "/workspace/repo",
			depth:       1,
			secretName:  "",
		},
	}

	deployment := BuildServerDeployment(agent, cfg, defaultSystemConfig(), nil, nil, nil, gitMounts)

	if deployment == nil {
		t.Fatal("BuildServerDeployment returned nil")
	}

	// Verify git-context volume exists
	var foundGitVolume bool
	for _, vol := range deployment.Spec.Template.Spec.Volumes {
		if vol.Name == "git-context-0" && vol.EmptyDir != nil {
			foundGitVolume = true
		}
	}
	if !foundGitVolume {
		t.Errorf("git-context-0 emptyDir volume not found")
	}

	// Verify git-init container exists (should be after opencode-init)
	var foundGitInit bool
	for _, ic := range deployment.Spec.Template.Spec.InitContainers {
		if ic.Name == "git-init-0" {
			foundGitInit = true
			// Verify environment variables
			envMap := make(map[string]string)
			for _, env := range ic.Env {
				envMap[env.Name] = env.Value
			}
			if envMap["GIT_REPO"] != "https://github.com/org/repo.git" {
				t.Errorf("GIT_REPO = %q, want %q", envMap["GIT_REPO"], "https://github.com/org/repo.git")
			}
			if envMap["GIT_REF"] != "main" {
				t.Errorf("GIT_REF = %q, want %q", envMap["GIT_REF"], "main")
			}
		}
	}
	if !foundGitInit {
		t.Errorf("git-init-0 init container not found")
	}

	// Verify GIT_CONFIG_GLOBAL env var is set
	container := deployment.Spec.Template.Spec.Containers[0]
	var foundGitConfigGlobal bool
	for _, env := range container.Env {
		if env.Name == "GIT_CONFIG_GLOBAL" {
			foundGitConfigGlobal = true
			expectedValue := DefaultGitRoot + "/.gitconfig"
			if env.Value != expectedValue {
				t.Errorf("GIT_CONFIG_GLOBAL = %q, want %q", env.Value, expectedValue)
			}
		}
	}
	if !foundGitConfigGlobal {
		t.Errorf("GIT_CONFIG_GLOBAL env var not found")
	}
}

func TestBuildServerDeployment_SkipsOPENCODE_PERMISSIONWhenConfigHasPermission(t *testing.T) {
	agent := &kubeopenv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: kubeopenv1alpha1.AgentSpec{
			ServerConfig: &kubeopenv1alpha1.ServerConfig{
				Port: 4096,
			},
		},
	}

	configWithPermission := `{"permission": "ask", "model": "gpt-4"}`
	cfg := agentConfig{
		executorImage: "test-executor:v1.0.0",
		agentImage:    "test-agent:v1.0.0",
		workspaceDir:  "/workspace",
		config:        &configWithPermission,
	}

	deployment := BuildServerDeployment(agent, cfg, defaultSystemConfig(), nil, nil, nil, nil)

	if deployment == nil {
		t.Fatal("BuildServerDeployment returned nil")
	}

	container := deployment.Spec.Template.Spec.Containers[0]

	// Verify OPENCODE_PERMISSION env var is NOT set
	for _, env := range container.Env {
		if env.Name == OpenCodePermissionEnvVar {
			t.Errorf("OPENCODE_PERMISSION should not be set when config has permission field")
		}
	}
}

func TestBuildServerDeployment_SetsOPENCODE_PERMISSIONWhenConfigHasNoPermission(t *testing.T) {
	agent := &kubeopenv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: kubeopenv1alpha1.AgentSpec{
			ServerConfig: &kubeopenv1alpha1.ServerConfig{
				Port: 4096,
			},
		},
	}

	configWithoutPermission := `{"model": "gpt-4"}`
	cfg := agentConfig{
		executorImage: "test-executor:v1.0.0",
		agentImage:    "test-agent:v1.0.0",
		workspaceDir:  "/workspace",
		config:        &configWithoutPermission,
	}

	deployment := BuildServerDeployment(agent, cfg, defaultSystemConfig(), nil, nil, nil, nil)

	if deployment == nil {
		t.Fatal("BuildServerDeployment returned nil")
	}

	container := deployment.Spec.Template.Spec.Containers[0]

	// Verify OPENCODE_PERMISSION env var is set
	var foundPermissionEnv bool
	for _, env := range container.Env {
		if env.Name == OpenCodePermissionEnvVar {
			foundPermissionEnv = true
			if env.Value != DefaultOpenCodePermission {
				t.Errorf("OPENCODE_PERMISSION = %q, want %q", env.Value, DefaultOpenCodePermission)
			}
		}
	}
	if !foundPermissionEnv {
		t.Errorf("OPENCODE_PERMISSION env var should be set when config has no permission field")
	}
}
