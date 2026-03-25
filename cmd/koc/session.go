// Copyright Contributors to the KubeOpenCode project

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubeopenv1alpha1 "github.com/kubeopencode/kubeopencode/api/v1alpha1"
)

func init() {
	rootCmd.AddCommand(newSessionCmd())
}

func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Interact with agent sessions",
	}
	cmd.AddCommand(newSessionWatchCmd())
	cmd.AddCommand(newSessionAttachCmd())
	return cmd
}

func newSessionWatchCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "watch <task-name>",
		Short: "Watch agent events for a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSession(cmd.Context(), namespace, args[0], false)
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "kubeopencode-system", "Task namespace")
	return cmd
}

func newSessionAttachCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "attach <task-name>",
		Short: "Interactively attach to an agent session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSession(cmd.Context(), namespace, args[0], true)
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "kubeopencode-system", "Task namespace")
	return cmd
}

func runSession(ctx context.Context, namespace, taskName string, interactive bool) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	serverURL, err := resolveServerURL(ctx, k8sClient, namespace, taskName)
	if err != nil {
		return err
	}

	fmt.Printf("\033[90m[info]\033[0m Connected to: %s\n", serverURL)
	if interactive {
		fmt.Printf("\033[90m[info]\033[0m Interactive mode. Ctrl+C to disconnect.\n\n")
	} else {
		fmt.Printf("\033[90m[info]\033[0m Watch mode. Ctrl+C to stop.\n\n")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/event", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect: %w\nHint: port-forward first:\n  kubectl port-forward svc/<agent> -n %s 4096:4096", err, namespace)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	scanner := bufio.NewScanner(os.Stdin)

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("\n\033[90m[info]\033[0m Disconnected.\n")
			return nil
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					fmt.Printf("\033[90m[info]\033[0m Stream ended.\n")
					return nil
				}
				return fmt.Errorf("read error: %w", err)
			}

			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			eventType, _ := event["type"].(string)
			props, _ := event["properties"].(map[string]interface{})
			if props == nil {
				props = event
			}

			switch eventType {
			case "server.connected":
				fmt.Printf("\033[32m[connected]\033[0m Session established\n")

			case "server.heartbeat":
				// silent

			case "permission.asked":
				permission, _ := props["permission"].(string)
				id, _ := props["id"].(string)
				patterns := toStringSlice(props["patterns"])
				fmt.Printf("\033[33m[permission]\033[0m %s on %s\n", permission, strings.Join(patterns, ", "))

				if interactive && id != "" {
					fmt.Printf("  \033[33m[a]llow once / [A]lways / [r]eject: \033[0m")
					if scanner.Scan() {
						reply := parseReply(scanner.Text())
						if err := postJSON(ctx, serverURL+"/permission/"+id+"/reply", map[string]string{"reply": reply}); err != nil {
							fmt.Printf("  \033[31m[error]\033[0m %s\n", err)
						} else {
							fmt.Printf("  \033[32m[replied]\033[0m %s\n", reply)
						}
					}
				}

			case "question.asked":
				id, _ := props["id"].(string)
				questions, _ := props["questions"].([]interface{})
				fmt.Printf("\033[34m[question]\033[0m Agent asks:\n")
				for qi, q := range questions {
					qMap, _ := q.(map[string]interface{})
					question, _ := qMap["question"].(string)
					options, _ := qMap["options"].([]interface{})
					fmt.Printf("  %d. %s\n", qi+1, question)
					for oi, opt := range options {
						optMap, _ := opt.(map[string]interface{})
						label, _ := optMap["label"].(string)
						desc, _ := optMap["description"].(string)
						fmt.Printf("     %d) %s", oi+1, label)
						if desc != "" {
							fmt.Printf(" - %s", desc)
						}
						fmt.Println()
					}
				}

				if interactive && id != "" {
					fmt.Printf("  \033[34mEnter choice (number/text, 's' to skip): \033[0m")
					if scanner.Scan() {
						input := strings.TrimSpace(scanner.Text())
						if strings.ToLower(input) == "s" {
							_ = postEmpty(ctx, serverURL+"/question/"+id+"/reject")
							fmt.Printf("  \033[32m[skipped]\033[0m\n")
						} else {
							_ = postJSON(ctx, serverURL+"/question/"+id+"/reply", map[string]interface{}{"answers": [][]string{{input}}})
							fmt.Printf("  \033[32m[answered]\033[0m %s\n", input)
						}
					}
				}

			case "session.status":
				if status, ok := props["status"].(map[string]interface{}); ok {
					if t, _ := status["type"].(string); t != "" {
						fmt.Printf("\033[90m[status]\033[0m %s\n", t)
					}
				}

			case "message.part.delta":
				if delta, _ := props["delta"].(string); delta != "" {
					fmt.Print(delta)
				}

			case "message.updated":
				fmt.Println()

			case "session.error":
				if errMsg, _ := props["error"].(string); errMsg != "" {
					fmt.Printf("\033[31m[error]\033[0m %s\n", errMsg)
				}
			}
		}
	}
}

func resolveServerURL(ctx context.Context, k8sClient client.Client, namespace, taskName string) (string, error) {
	var task kubeopenv1alpha1.Task
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, &task); err != nil {
		return "", fmt.Errorf("task %q not found in namespace %q: %w", taskName, namespace, err)
	}

	agentName := ""
	if task.Status.AgentRef != nil {
		agentName = task.Status.AgentRef.Name
	} else if task.Spec.AgentRef != nil {
		agentName = task.Spec.AgentRef.Name
	}
	if agentName == "" {
		return "", fmt.Errorf("task %q has no agent reference", taskName)
	}

	var agent kubeopenv1alpha1.Agent
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: agentName, Namespace: namespace}, &agent); err != nil {
		return "", fmt.Errorf("agent %q not found: %w", agentName, err)
	}

	if agent.Spec.ServerConfig == nil {
		return "", fmt.Errorf("agent %q is not in Server mode (requires serverConfig)", agentName)
	}

	if agent.Status.ServerStatus == nil || agent.Status.ServerStatus.URL == "" {
		return "", fmt.Errorf("agent %q server is not ready", agentName)
	}

	return agent.Status.ServerStatus.URL, nil
}

func toStringSlice(v interface{}) []string {
	arr, _ := v.([]interface{})
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func parseReply(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "aa", "always":
		return "always"
	case "r", "reject":
		return "reject"
	default:
		return "once"
	}
}

func postJSON(ctx context.Context, url string, payload interface{}) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

func postEmpty(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}
