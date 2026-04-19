// Copyright Contributors to the KubeOpenCode project

// KubeOpenCode CLI for interactive agent sessions.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	kubeopenv1alpha1 "github.com/kubeopencode/kubeopencode/api/v1alpha1"
)

// Version information set via ldflags at build time.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var scheme = runtime.NewScheme()

// Persistent flags for kubeconfig/context override
var (
	kubeconfigFlag string
	contextFlag    string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kubeopenv1alpha1.AddToScheme(scheme))
}

var rootCmd = &cobra.Command{
	Use:   "kubeoc",
	Short: "KubeOpenCode CLI",
	Long: `kubeoc is the KubeOpenCode CLI for managing agents and tasks.

Commands:
  get agents|tasks|crontasks|agenttemplates   List resources
  agent attach|suspend|resume|share|unshare   Interact with agents
  task stop|logs                               Manage tasks
  crontask trigger|suspend|resume             Manage CronTasks
  completion bash|zsh|fish|powershell         Generate shell completion
  version                                      Print version information

Kubeconfig resolution (in priority order):
  1. --kubeconfig flag
  2. KUBEOPENCODE_KUBECONFIG environment variable
  3. --context flag (selects context within kubeconfig)
  4. KUBECONFIG environment variable
  5. Default ~/.kube/config

Examples:
  kubeoc get agents
  kubeoc get tasks -n production -o json
  kubeoc agent attach my-agent -n test
  kubeoc task logs my-task -n test -f
  kubeoc crontask trigger daily-scan -n production`,
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfigFlag, "kubeconfig", "", "Path to kubeconfig file")
	rootCmd.PersistentFlags().StringVar(&contextFlag, "context", "", "Kubernetes context to use")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newCompletionCmd())
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kubeoc version %s\n", Version)
			fmt.Printf("  git commit: %s\n", GitCommit)
			fmt.Printf("  build date: %s\n", BuildDate)
		},
	}
}

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Generate shell completion script for kubeoc.

To load completions:

Bash:
  $ source <(kubeoc completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ kubeoc completion bash > /etc/bash_completion.d/kubeoc
  # macOS:
  $ kubeoc completion bash > $(brew --prefix)/etc/bash_completion.d/kubeoc

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  # To load completions for each session, execute once:
  $ kubeoc completion zsh > "${fpath[1]}/_kubeoc"

Fish:
  $ kubeoc completion fish | source
  # To load completions for each session, execute once:
  $ kubeoc completion fish > ~/.config/fish/completions/kubeoc.fish

PowerShell:
  PS> kubeoc completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run and restart:
  PS> kubeoc completion powershell > kubeoc.ps1`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return rootCmd.GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				return rootCmd.GenZshCompletion(os.Stdout)
			case "fish":
				return rootCmd.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	}
	return cmd
}

// getKubeConfig returns a rest.Config with the following priority:
//  1. --kubeconfig flag
//  2. KUBEOPENCODE_KUBECONFIG env var (dedicated agent cluster config)
//  3. --context flag (selects context within kubeconfig)
//  4. KUBECONFIG env var
//  5. Default ~/.kube/config
func getKubeConfig() (*rest.Config, error) {
	kubeconfigPath := kubeconfigFlag
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBEOPENCODE_KUBECONFIG")
	}

	if kubeconfigPath != "" {
		if contextFlag != "" {
			loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
			configOverrides := &clientcmd.ConfigOverrides{CurrentContext: contextFlag}
			return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
		}
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	// Falls back to KUBECONFIG env var, then default path
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	if contextFlag != "" {
		configOverrides.CurrentContext = contextFlag
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
