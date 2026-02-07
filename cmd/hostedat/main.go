package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/cryguy/hostedat/internal/client"
	"github.com/spf13/cobra"
)

var version = "dev"

type cliConfig struct {
	Server string `json:"server"`
	APIKey string `json:"api_key"`
}

var (
	flagServer string
	flagAPIKey string
)

func main() {
	root := &cobra.Command{
		Use:   "hostedat",
		Short: "CLI client for hostedat.ditto.moe",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&flagServer, "server", "", "server URL (env: HOSTEDAT_SERVER)")
	root.PersistentFlags().StringVar(&flagAPIKey, "api-key", "", "API key (env: HOSTEDAT_API_KEY)")

	root.AddCommand(loginCmd())
	root.AddCommand(sitesCmd())
	root.AddCommand(deployCmd())
	root.AddCommand(versionCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func loginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate with the hostedat server via browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			serverURL := resolveServer()
			if serverURL == "" {
				return fmt.Errorf("server URL is required (use --server or HOSTEDAT_SERVER)")
			}

			apiKey, err := client.BrowserLogin(serverURL)
			if err != nil {
				return err
			}

			cfg := cliConfig{
				Server: serverURL,
				APIKey: apiKey,
			}
			if err := saveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Println("Login successful! API key saved.")
			return nil
		},
	}
}

func sitesCmd() *cobra.Command {
	sites := &cobra.Command{
		Use:   "sites",
		Short: "Manage sites",
	}

	// list
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List your sites",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}

			sites, err := c.ListSites()
			if err != nil {
				return err
			}

			if len(sites) == 0 {
				fmt.Println("No sites found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tSUBDOMAIN\tVERSION\tCREATED")
			for _, s := range sites {
				ver := "-"
				if s.ActiveVersion != nil {
					ver = fmt.Sprintf("v%d", *s.ActiveVersion)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					s.ID, s.Name, s.SubdomainSlug, ver, s.CreatedAt.Format("2006-01-02"))
			}
			w.Flush()
			return nil
		},
	}

	// create
	var subdomain string
	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new site",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}

			site, err := c.CreateSite(args[0], subdomain)
			if err != nil {
				return err
			}

			fmt.Printf("Site created!\n")
			fmt.Printf("  ID:        %s\n", site.ID)
			fmt.Printf("  Name:      %s\n", site.Name)
			fmt.Printf("  Subdomain: %s\n", site.SubdomainSlug)
			return nil
		},
	}
	create.Flags().StringVar(&subdomain, "subdomain", "", "custom subdomain slug")

	// delete
	var yes bool
	del := &cobra.Command{
		Use:   "delete <site-id>",
		Short: "Delete a site",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				fmt.Printf("Delete site %s? This cannot be undone. [y/N] ", args[0])
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(answer)) != "y" {
					fmt.Println("Aborted.")
					return nil
				}
			}

			c, err := newClient()
			if err != nil {
				return err
			}

			if err := c.DeleteSite(args[0]); err != nil {
				return err
			}

			fmt.Println("Site deleted.")
			return nil
		},
	}
	del.Flags().BoolVar(&yes, "yes", false, "skip confirmation")

	sites.AddCommand(list, create, del)
	return sites
}

func deployCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deploy <site-id> <directory>",
		Short: "Deploy a directory to a site",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			siteID := args[0]
			dir := args[1]

			c, err := newClient()
			if err != nil {
				return err
			}

			fmt.Printf("Deploying %s to site %s...\n", dir, siteID)
			deployment, err := c.Deploy(siteID, dir)
			if err != nil {
				return err
			}

			fmt.Printf("Deployed! Version: v%d\n", deployment.Version)
			return nil
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("hostedat %s\n", version)
		},
	}
}

// newClient creates an API client from resolved config.
func newClient() (*client.Client, error) {
	apiKey := resolveAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("not authenticated — run 'hostedat login' or set HOSTEDAT_API_KEY")
	}
	serverURL := resolveServer()
	if serverURL == "" {
		return nil, fmt.Errorf("server URL is required (use --server or HOSTEDAT_SERVER)")
	}
	return client.New(serverURL, apiKey), nil
}

// resolveAPIKey: env > flag > config file
func resolveAPIKey() string {
	if v := os.Getenv("HOSTEDAT_API_KEY"); v != "" {
		return v
	}
	if flagAPIKey != "" {
		return flagAPIKey
	}
	cfg, err := loadConfig()
	if err == nil && cfg.APIKey != "" {
		return cfg.APIKey
	}
	return ""
}

// resolveServer: flag > env > config file
func resolveServer() string {
	if flagServer != "" {
		return flagServer
	}
	if v := os.Getenv("HOSTEDAT_SERVER"); v != "" {
		return v
	}
	cfg, err := loadConfig()
	if err == nil && cfg.Server != "" {
		return cfg.Server
	}
	return ""
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hostedat", "config.json")
}

func loadConfig() (cliConfig, error) {
	var cfg cliConfig
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

func saveConfig(cfg cliConfig) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
