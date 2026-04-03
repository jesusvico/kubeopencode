// Copyright Contributors to the KubeOpenCode project

package controller

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubeopenv1alpha1 "github.com/kubeopencode/kubeopencode/api/v1alpha1"
)

const (
	// GitHashAnnotationPrefix is the prefix for pod template annotations that track
	// the current Git commit hash for Rollout sync policy contexts.
	// Changing the annotation value triggers a Deployment rolling update.
	GitHashAnnotationPrefix = "kubeopencode.io/git-hash-"

	// GitSyncPendingTimeout is the maximum time to wait for active Tasks to complete
	// before forcing a rollout. This prevents long-running Tasks from indefinitely
	// blocking Git sync updates.
	GitSyncPendingTimeout = 1 * time.Hour

	// GitSyncPendingRecheck is the requeue interval when waiting for Tasks to complete.
	GitSyncPendingRecheck = 30 * time.Second

	// gitLsRemoteTimeout is the maximum time to wait for a git ls-remote command.
	gitLsRemoteTimeout = 30 * time.Second
)

// pendingRollout tracks a detected change that may be delayed by Task protection.
type pendingRollout struct {
	contextName string
	oldHash     string // hash BEFORE the change was detected
	newHash     string // the newly detected remote hash
}

// reconcileGitSync checks for remote Git changes on contexts with Rollout sync policy
// and returns pod template annotations to trigger rolling updates when changes are detected.
//
// Task protection: if active Tasks exist when a change is detected, the rollout is delayed
// until all Tasks complete (or the safety timeout expires).
//
// Returns:
//   - gitHashAnnotations: map of annotation key→commit hash for pod template
//   - requeueAfter: shortest sync interval (0 if no Rollout contexts)
//   - err: any error encountered
func (r *AgentReconciler) reconcileGitSync(ctx context.Context, agent *kubeopenv1alpha1.Agent, gitMounts []gitMount) (map[string]string, time.Duration, error) {
	logger := log.FromContext(ctx)

	// Collect all Rollout sync contexts
	hasRolloutSync := false
	for _, gm := range gitMounts {
		if gm.syncEnabled && gm.syncPolicy == kubeopenv1alpha1.GitSyncPolicyRollout {
			hasRolloutSync = true
			break
		}
	}

	if !hasRolloutSync {
		// No Rollout contexts — clear any stale GitSyncPending condition
		clearGitSyncPending(agent)
		return nil, 0, nil
	}

	annotations := make(map[string]string)
	var shortestInterval time.Duration
	var pendingRollouts []pendingRollout

	lsRemoteFn := r.GitLsRemoteFn
	if lsRemoteFn == nil {
		lsRemoteFn = gitLsRemote
	}

	for _, gm := range gitMounts {
		if !gm.syncEnabled || gm.syncPolicy != kubeopenv1alpha1.GitSyncPolicyRollout {
			continue
		}

		// Track shortest interval for requeue
		if shortestInterval == 0 || gm.syncInterval < shortestInterval {
			shortestInterval = gm.syncInterval
		}

		annotationKey := GitHashAnnotationPrefix + gm.contextName

		// Save old hash BEFORE any status mutations
		oldHash := getStatusCommitHash(agent, gm.contextName)

		// Get the remote commit hash
		remoteHash, err := lsRemoteFn(ctx, gm.repository, gm.ref, gm.secretName)
		if err != nil {
			logger.Error(err, "Failed to check remote Git ref", "context", gm.contextName, "repo", gm.repository)
			// On error, keep the current hash (don't trigger rollout)
			if oldHash != "" {
				annotations[annotationKey] = oldHash
			}
			continue
		}

		// Update LastSynced timestamp
		now := metav1.Now()
		updateSyncStatus(agent, gm.contextName, "", &now)

		// Compare with the last applied commit hash
		if oldHash == "" || oldHash == remoteHash {
			// No change — use current hash for annotation
			if remoteHash != "" {
				annotations[annotationKey] = remoteHash
				// First time: set the hash without triggering rollout
				if oldHash == "" {
					updateSyncStatus(agent, gm.contextName, remoteHash, &now)
				}
			}
			continue
		}

		// Change detected
		logger.Info("Git remote change detected",
			"context", gm.contextName,
			"repo", gm.repository,
			"oldHash", truncateHash(oldHash),
			"newHash", truncateHash(remoteHash),
		)

		pendingRollouts = append(pendingRollouts, pendingRollout{
			contextName: gm.contextName,
			oldHash:     oldHash,
			newHash:     remoteHash,
		})

		// Set new hash in annotations (may be reverted below if Tasks are active).
		// Do NOT update status hash yet — only update after rollout is confirmed,
		// so the change is re-detected on next reconcile if blocked by Task protection.
		annotations[annotationKey] = remoteHash
	}

	if len(pendingRollouts) == 0 {
		clearGitSyncPending(agent)
		return annotations, shortestInterval, nil
	}

	// Task protection: check for active Tasks before allowing rollout
	countFn := r.CountActiveTasksFn
	if countFn == nil {
		countFn = r.countActiveTasks
	}
	activeTasks, err := countFn(ctx, agent.Name, agent.Namespace)
	if err != nil {
		return annotations, shortestInterval, fmt.Errorf("failed to count active tasks: %w", err)
	}

	pendingContextNames := make([]string, len(pendingRollouts))
	for i, pr := range pendingRollouts {
		pendingContextNames[i] = pr.contextName
	}

	if activeTasks > 0 {
		// Check safety timeout
		pendingCondition := meta.FindStatusCondition(agent.Status.Conditions, AgentConditionGitSyncPending)
		if pendingCondition != nil && pendingCondition.Status == metav1.ConditionTrue {
			elapsed := time.Since(pendingCondition.LastTransitionTime.Time)
			if elapsed >= GitSyncPendingTimeout {
				// Safety timeout expired — force rollout
				logger.Info("Git sync safety timeout expired, forcing rollout",
					"agent", agent.Name,
					"activeTasks", activeTasks,
					"elapsed", elapsed,
				)
				// Force rollout: update status hashes
				forceNow := metav1.Now()
				for _, pr := range pendingRollouts {
					updateSyncStatus(agent, pr.contextName, pr.newHash, &forceNow)
				}
				clearGitSyncPending(agent)
				return annotations, shortestInterval, nil
			}
		}

		// Set or maintain GitSyncPending condition
		setGitSyncPending(agent, activeTasks, pendingContextNames)
		logger.Info("Git sync rollout delayed, waiting for active tasks",
			"agent", agent.Name,
			"activeTasks", activeTasks,
			"pendingContexts", pendingContextNames,
		)

		// Revert annotations to OLD hashes to prevent Deployment update
		for _, pr := range pendingRollouts {
			annotationKey := GitHashAnnotationPrefix + pr.contextName
			annotations[annotationKey] = pr.oldHash
		}

		// Requeue quickly to re-check
		if GitSyncPendingRecheck < shortestInterval {
			shortestInterval = GitSyncPendingRecheck
		}
		return annotations, shortestInterval, nil
	}

	// No active Tasks — proceed with rollout. Update status hashes now.
	now := metav1.Now()
	for _, pr := range pendingRollouts {
		updateSyncStatus(agent, pr.contextName, pr.newHash, &now)
	}
	logger.Info("Git sync rollout proceeding, no active tasks",
		"agent", agent.Name,
		"contexts", pendingContextNames,
	)
	clearGitSyncPending(agent)
	return annotations, shortestInterval, nil
}

// gitLsRemote executes `git ls-remote` and returns the commit hash for the given ref.
// Uses a bounded timeout to prevent blocking the controller reconcile loop.
func gitLsRemote(ctx context.Context, repo, ref, secretName string) (string, error) {
	if secretName != "" {
		return "", fmt.Errorf("git ls-remote with Secret authentication is not yet implemented for Rollout policy; use HotReload policy for private repos")
	}

	args := []string{"ls-remote", repo}
	if ref != "" && ref != "HEAD" {
		args = append(args, ref)
	} else {
		args = append(args, "HEAD")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, gitLsRemoteTimeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "git", args...) //nolint:gosec // args from controlled CRD spec inputs
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return "", fmt.Errorf("git ls-remote failed for %s: %w (stderr: %s)", repo, err, stderrStr)
		}
		return "", fmt.Errorf("git ls-remote failed for %s: %w", repo, err)
	}

	// Parse output: "<hash>\t<ref>"
	lines := strings.TrimSpace(string(output))
	if lines == "" {
		return "", fmt.Errorf("git ls-remote returned empty output for %s ref %s", repo, ref)
	}

	// Take the first line's hash
	parts := strings.Fields(strings.Split(lines, "\n")[0])
	if len(parts) == 0 {
		return "", fmt.Errorf("git ls-remote returned unexpected output: %s", lines)
	}

	return parts[0], nil
}

// getStatusCommitHash returns the commit hash for a context from Agent status.
func getStatusCommitHash(agent *kubeopenv1alpha1.Agent, contextName string) string {
	for _, s := range agent.Status.GitSyncStatuses {
		if s.Name == contextName {
			return s.CommitHash
		}
	}
	return ""
}

// updateSyncStatus updates or creates a GitSyncStatus entry for the given context.
func updateSyncStatus(agent *kubeopenv1alpha1.Agent, contextName, commitHash string, lastSynced *metav1.Time) {
	for i, s := range agent.Status.GitSyncStatuses {
		if s.Name == contextName {
			if commitHash != "" {
				agent.Status.GitSyncStatuses[i].CommitHash = commitHash
			}
			if lastSynced != nil {
				agent.Status.GitSyncStatuses[i].LastSynced = lastSynced
			}
			return
		}
	}
	// Not found — create new entry
	status := kubeopenv1alpha1.GitSyncStatus{
		Name:       contextName,
		CommitHash: commitHash,
		LastSynced: lastSynced,
	}
	agent.Status.GitSyncStatuses = append(agent.Status.GitSyncStatuses, status)
}

// setGitSyncPending sets the GitSyncPending condition on the Agent.
func setGitSyncPending(agent *kubeopenv1alpha1.Agent, activeTasks int, pendingContexts []string) {
	message := fmt.Sprintf("Waiting for %d active task(s) to complete before rollout (contexts: %s)",
		activeTasks, strings.Join(pendingContexts, ", "))
	setAgentCondition(agent, AgentConditionGitSyncPending, metav1.ConditionTrue, "WaitingForTasks", message)
}

// clearGitSyncPending removes the GitSyncPending condition from the Agent.
func clearGitSyncPending(agent *kubeopenv1alpha1.Agent) {
	setAgentCondition(agent, AgentConditionGitSyncPending, metav1.ConditionFalse, "Synced", "")
}

// truncateHash safely truncates a hash string for logging.
func truncateHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
