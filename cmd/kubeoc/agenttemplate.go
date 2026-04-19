// Copyright Contributors to the KubeOpenCode project

package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubeopenv1alpha1 "github.com/kubeopencode/kubeopencode/api/v1alpha1"
)

// newGetAgentTemplatesCmd creates the "get agenttemplates" subcommand.
func newGetAgentTemplatesCmd() *cobra.Command {
	var (
		namespace string
		wide      bool
		output    string
	)

	cmd := &cobra.Command{
		Use:     "agenttemplates",
		Aliases: []string{"agt"},
		Short:   "List agent templates",
		Long: `List agent templates across all namespaces (or a specific namespace with -n).

Use --wide to show additional columns (executor image, workspace, service account).
Use -o json or -o yaml to output in structured format.

Examples:
  kubeoc get agenttemplates
  kubeoc get agenttemplates -n production
  kubeoc get agenttemplates --wide
  kubeoc get agenttemplates -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := getKubeConfig()
			if err != nil {
				return fmt.Errorf("cannot connect to cluster: %w", err)
			}

			k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			var templates kubeopenv1alpha1.AgentTemplateList
			listOpts := []client.ListOption{}
			if namespace != "" {
				listOpts = append(listOpts, client.InNamespace(namespace))
			}

			if err := k8sClient.List(cmd.Context(), &templates, listOpts...); err != nil {
				return fmt.Errorf("failed to list agent templates: %w", err)
			}

			// Handle structured output formats
			if handled, err := outputFormat(output, templates); handled {
				return err
			}

			if len(templates.Items) == 0 {
				if namespace != "" {
					fmt.Printf("No agent templates found in namespace %q\n", namespace)
				} else {
					fmt.Println("No agent templates found")
				}
				return nil
			}

			// Count referencing agents per template
			refCounts := countReferencingAgents(cmd.Context(), k8sClient, templates.Items)

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			if wide {
				_, _ = fmt.Fprintln(w, "NAMESPACE\tNAME\tAGENTS\tEXECUTOR IMAGE\tWORKSPACE\tSERVICE ACCOUNT")
			} else {
				_, _ = fmt.Fprintln(w, "NAMESPACE\tNAME\tAGENTS")
			}

			for i, tmpl := range templates.Items {
				if wide {
					image := tmpl.Spec.ExecutorImage
					if len(image) > 50 {
						image = "..." + image[len(image)-47:]
					}

					_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\n",
						tmpl.Namespace, tmpl.Name, refCounts[i],
						image, tmpl.Spec.WorkspaceDir, tmpl.Spec.ServiceAccountName)
				} else {
					_, _ = fmt.Fprintf(w, "%s\t%s\t%d\n",
						tmpl.Namespace, tmpl.Name, refCounts[i])
				}
			}

			_ = w.Flush()
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace (default: all namespaces)")
	cmd.Flags().BoolVar(&wide, "wide", false, "Show additional columns (executor image, workspace, service account)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: json, yaml")
	return cmd
}

// countReferencingAgents counts how many Agents reference each AgentTemplate.
func countReferencingAgents(ctx context.Context, k8sClient client.Client, templates []kubeopenv1alpha1.AgentTemplate) []int {
	counts := make([]int, len(templates))

	type nsName struct {
		ns, name string
	}
	templateIndex := make(map[nsName]int, len(templates))
	for i, tmpl := range templates {
		templateIndex[nsName{tmpl.Namespace, tmpl.Name}] = i
	}

	var agents kubeopenv1alpha1.AgentList
	if err := k8sClient.List(ctx, &agents); err != nil {
		return counts
	}

	for _, agent := range agents.Items {
		if agent.Spec.TemplateRef != nil {
			key := nsName{agent.Namespace, agent.Spec.TemplateRef.Name}
			if idx, ok := templateIndex[key]; ok {
				counts[idx]++
			}
		}
	}

	return counts
}
