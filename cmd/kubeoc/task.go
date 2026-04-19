// Copyright Contributors to the KubeOpenCode project

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubeopenv1alpha1 "github.com/kubeopencode/kubeopencode/api/v1alpha1"
)

func init() {
	rootCmd.AddCommand(newTaskCmd())
}

func newTaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage KubeOpenCode tasks",
	}
	cmd.AddCommand(newTaskStopCmd())
	cmd.AddCommand(newTaskLogsCmd())
	return cmd
}

func newTaskStopCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "stop <task-name>",
		Short: "Stop a running task",
		Long: `Stop a running Task by setting the kubeopencode.io/stop=true annotation.

The controller will gracefully terminate the Task's Pod and mark the Task
as Completed with a Stopped condition.

Examples:
  kubeoc task stop my-task -n test
  kubeoc task stop long-running-task -n production`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskName := args[0]

			cfg, err := getKubeConfig()
			if err != nil {
				return fmt.Errorf("cannot connect to cluster: %w", err)
			}

			k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			var task kubeopenv1alpha1.Task
			if err := k8sClient.Get(cmd.Context(), types.NamespacedName{
				Name:      taskName,
				Namespace: namespace,
			}, &task); err != nil {
				return fmt.Errorf("task %q not found in namespace %q: %w", taskName, namespace, err)
			}

			// Check if already stopped or completed
			phase := task.Status.Phase
			if phase == kubeopenv1alpha1.TaskPhaseCompleted || phase == kubeopenv1alpha1.TaskPhaseFailed {
				return fmt.Errorf("task %q is already in %s phase", taskName, phase)
			}

			// Set the stop annotation
			if task.Annotations == nil {
				task.Annotations = make(map[string]string)
			}
			task.Annotations["kubeopencode.io/stop"] = "true"

			if err := k8sClient.Update(cmd.Context(), &task); err != nil {
				return fmt.Errorf("failed to stop task %q: %w", taskName, err)
			}

			fmt.Printf("Task %s/%s stop requested\n", namespace, taskName)
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Task namespace")
	return cmd
}

func newTaskLogsCmd() *cobra.Command {
	var (
		namespace string
		follow    bool
	)

	cmd := &cobra.Command{
		Use:   "logs <task-name>",
		Short: "View logs from a task's pod",
		Long: `Stream or view logs from the Pod associated with a Task.

Examples:
  kubeoc task logs my-task -n test
  kubeoc task logs my-task -n test -f`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskName := args[0]

			cfg, err := getKubeConfig()
			if err != nil {
				return fmt.Errorf("cannot connect to cluster: %w", err)
			}

			k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			var task kubeopenv1alpha1.Task
			if err := k8sClient.Get(cmd.Context(), types.NamespacedName{
				Name:      taskName,
				Namespace: namespace,
			}, &task); err != nil {
				return fmt.Errorf("task %q not found in namespace %q: %w", taskName, namespace, err)
			}

			podName := task.Status.PodName
			if podName == "" {
				return fmt.Errorf("task %q has no associated pod (phase: %s)", taskName, task.Status.Phase)
			}

			clientset, err := kubernetes.NewForConfig(cfg)
			if err != nil {
				return fmt.Errorf("failed to create clientset: %w", err)
			}

			opts := &corev1.PodLogOptions{
				Container: "agent",
				Follow:    follow,
			}

			req := clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
			stream, err := req.Stream(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to stream logs for pod %q: %w", podName, err)
			}
			defer func() { _ = stream.Close() }()

			_, err = io.Copy(os.Stdout, stream)
			if err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("error reading logs: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Task namespace")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	return cmd
}

// newGetTasksCmd creates the "get tasks" subcommand.
func newGetTasksCmd() *cobra.Command {
	var (
		namespace string
		wide      bool
		output    string
	)

	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "List tasks",
		Long: `List tasks across all namespaces (or a specific namespace with -n).

Use --wide to show additional columns (pod name).
Use -o json or -o yaml to output in structured format.

Examples:
  kubeoc get tasks
  kubeoc get tasks -n production
  kubeoc get tasks --wide
  kubeoc get tasks -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := getKubeConfig()
			if err != nil {
				return fmt.Errorf("cannot connect to cluster: %w", err)
			}

			k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			var tasks kubeopenv1alpha1.TaskList
			listOpts := []client.ListOption{}
			if namespace != "" {
				listOpts = append(listOpts, client.InNamespace(namespace))
			}

			if err := k8sClient.List(cmd.Context(), &tasks, listOpts...); err != nil {
				return fmt.Errorf("failed to list tasks: %w", err)
			}

			// Handle structured output formats
			if handled, err := outputFormat(output, tasks); handled {
				return err
			}

			if len(tasks.Items) == 0 {
				if namespace != "" {
					fmt.Printf("No tasks found in namespace %q\n", namespace)
				} else {
					fmt.Println("No tasks found")
				}
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			if wide {
				_, _ = fmt.Fprintln(w, "NAMESPACE\tNAME\tAGENT\tPHASE\tAGE\tPOD")
			} else {
				_, _ = fmt.Fprintln(w, "NAMESPACE\tNAME\tAGENT\tPHASE\tAGE")
			}

			for _, task := range tasks.Items {
				agent := "-"
				if task.Spec.AgentRef != nil {
					agent = task.Spec.AgentRef.Name
				}

				phase := string(task.Status.Phase)
				if phase == "" {
					phase = "Pending"
				}

				age := formatAge(task.CreationTimestamp.Time)

				if wide {
					pod := task.Status.PodName
					if pod == "" {
						pod = "-"
					}
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
						task.Namespace, task.Name, agent, phase, age, pod)
				} else {
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
						task.Namespace, task.Name, agent, phase, age)
				}
			}

			_ = w.Flush()
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace (default: all namespaces)")
	cmd.Flags().BoolVar(&wide, "wide", false, "Show additional columns (pod)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: json, yaml")
	return cmd
}

// formatAge returns a human-readable age string.
func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
