// Copyright Contributors to the KubeOpenCode project

package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubeopenv1alpha1 "github.com/kubeopencode/kubeopencode/api/v1alpha1"
)

func init() {
	rootCmd.AddCommand(newCronTaskCmd())
}

func newCronTaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "crontask",
		Aliases: []string{"ct"},
		Short:   "Manage CronTasks",
	}
	cmd.AddCommand(newCronTaskTriggerCmd())
	cmd.AddCommand(newCronTaskSuspendCmd())
	cmd.AddCommand(newCronTaskResumeCmd())
	return cmd
}

func newCronTaskTriggerCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "trigger <crontask-name>",
		Short: "Manually trigger a CronTask to create a Task immediately",
		Long: `Trigger a CronTask by adding the kubeopencode.io/trigger=true annotation.
The controller will create a new Task immediately regardless of the schedule.

Examples:
  kubeoc crontask trigger daily-vuln-scan -n test
  kubeoc ct trigger my-crontask -n production`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			cfg, err := getKubeConfig()
			if err != nil {
				return fmt.Errorf("cannot connect to cluster: %w", err)
			}

			k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			var cronTask kubeopenv1alpha1.CronTask
			if err := k8sClient.Get(cmd.Context(), types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			}, &cronTask); err != nil {
				return fmt.Errorf("CronTask %q not found in namespace %q: %w", name, namespace, err)
			}

			patch := client.MergeFrom(cronTask.DeepCopy())
			if cronTask.Annotations == nil {
				cronTask.Annotations = make(map[string]string)
			}
			cronTask.Annotations[kubeopenv1alpha1.CronTaskTriggerAnnotation] = "true"

			if err := k8sClient.Patch(cmd.Context(), &cronTask, patch); err != nil {
				return fmt.Errorf("failed to trigger CronTask %q: %w", name, err)
			}

			fmt.Printf("CronTask %s/%s triggered\n", namespace, name)
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "CronTask namespace")
	return cmd
}

func newCronTaskSuspendCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "suspend <crontask-name>",
		Short: "Suspend a CronTask (pause scheduling)",
		Long: `Suspend a CronTask to stop creating new Tasks on schedule.
Existing running Tasks are not affected.

Examples:
  kubeoc crontask suspend daily-vuln-scan -n test
  kubeoc ct suspend my-crontask`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setCronTaskSuspend(cmd, args[0], namespace, true)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "CronTask namespace")
	return cmd
}

func newCronTaskResumeCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "resume <crontask-name>",
		Short: "Resume a suspended CronTask",
		Long: `Resume a suspended CronTask to restart scheduling.

Examples:
  kubeoc crontask resume daily-vuln-scan -n test
  kubeoc ct resume my-crontask`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setCronTaskSuspend(cmd, args[0], namespace, false)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "CronTask namespace")
	return cmd
}

func setCronTaskSuspend(cmd *cobra.Command, name, namespace string, suspend bool) error {
	cfg, err := getKubeConfig()
	if err != nil {
		return fmt.Errorf("cannot connect to cluster: %w", err)
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	var cronTask kubeopenv1alpha1.CronTask
	if err := k8sClient.Get(cmd.Context(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &cronTask); err != nil {
		return fmt.Errorf("CronTask %q not found in namespace %q: %w", name, namespace, err)
	}

	cronTask.Spec.Suspend = &suspend
	if err := k8sClient.Update(cmd.Context(), &cronTask); err != nil {
		return fmt.Errorf("failed to update CronTask %q: %w", name, err)
	}

	action := "suspended"
	if !suspend {
		action = "resumed"
	}
	fmt.Printf("CronTask %s/%s %s\n", namespace, name, action)
	return nil
}

// newGetCronTasksCmd creates the "get crontasks" subcommand.
func newGetCronTasksCmd() *cobra.Command {
	var (
		namespace string
		wide      bool
		output    string
	)

	cmd := &cobra.Command{
		Use:     "crontasks",
		Aliases: []string{"ct"},
		Short:   "List CronTasks",
		Long: `List CronTasks across all namespaces (or a specific namespace with -n).

Use --wide to show additional columns (timezone, concurrency policy, max retained).
Use -o json or -o yaml to output in structured format.

Examples:
  kubeoc get crontasks
  kubeoc get ct -n production
  kubeoc get crontasks --wide
  kubeoc get crontasks -o yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := getKubeConfig()
			if err != nil {
				return fmt.Errorf("cannot connect to cluster: %w", err)
			}

			k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			var cronTasks kubeopenv1alpha1.CronTaskList
			listOpts := []client.ListOption{}
			if namespace != "" {
				listOpts = append(listOpts, client.InNamespace(namespace))
			}

			if err := k8sClient.List(cmd.Context(), &cronTasks, listOpts...); err != nil {
				return fmt.Errorf("failed to list CronTasks: %w", err)
			}

			// Handle structured output formats
			if handled, err := outputFormat(output, cronTasks); handled {
				return err
			}

			if len(cronTasks.Items) == 0 {
				if namespace != "" {
					fmt.Printf("No CronTasks found in namespace %q\n", namespace)
				} else {
					fmt.Println("No CronTasks found")
				}
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			if wide {
				_, _ = fmt.Fprintln(w, "NAMESPACE\tNAME\tSCHEDULE\tSUSPEND\tACTIVE\tLAST SCHEDULE\tTZ\tPOLICY\tMAX\tAGE")
			} else {
				_, _ = fmt.Fprintln(w, "NAMESPACE\tNAME\tSCHEDULE\tSUSPEND\tACTIVE\tLAST SCHEDULE\tAGE")
			}

			for _, ct := range cronTasks.Items {
				suspend := "False"
				if ct.Spec.Suspend != nil && *ct.Spec.Suspend {
					suspend = "True"
				}

				lastSchedule := "<none>"
				if ct.Status.LastScheduleTime != nil {
					lastSchedule = formatAge(ct.Status.LastScheduleTime.Time)
				}

				age := formatAge(ct.CreationTimestamp.Time)

				if wide {
					tz := "UTC"
					if ct.Spec.TimeZone != nil {
						tz = *ct.Spec.TimeZone
					}
					policy := string(ct.Spec.ConcurrencyPolicy)
					maxRetained := int32(10)
					if ct.Spec.MaxRetainedTasks != nil {
						maxRetained = *ct.Spec.MaxRetainedTasks
					}
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%d\t%s\n",
						ct.Namespace, ct.Name, ct.Spec.Schedule, suspend,
						ct.Status.Active, lastSchedule, tz, policy, maxRetained, age)
				} else {
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
						ct.Namespace, ct.Name, ct.Spec.Schedule, suspend,
						ct.Status.Active, lastSchedule, age)
				}
			}

			_ = w.Flush()
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace (default: all namespaces)")
	cmd.Flags().BoolVar(&wide, "wide", false, "Show additional columns (timezone, policy, max retained)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: json, yaml")
	return cmd
}
