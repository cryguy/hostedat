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

var (
	version       = "dev"
	defaultServer = "" // set via -ldflags at build time
)

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

			apiKey, err := client.BrowserLogin(serverURL, version)
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
		Use:   "delete <site>",
		Short: "Delete a site (by name, subdomain, or ID)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}

			siteID, err := c.ResolveSiteID(args[0])
			if err != nil {
				return err
			}

			if !yes {
				fmt.Printf("Delete site %s? This cannot be undone. [y/N] ", args[0])
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(answer)) != "y" {
					fmt.Println("Aborted.")
					return nil
				}
			}

			if err := c.DeleteSite(siteID); err != nil {
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
	var spaFlag bool
	cmd := &cobra.Command{
		Use:   "deploy <site> <directory>",
		Short: "Deploy a directory to a site (by name, subdomain, or ID)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[1]

			c, err := newClient()
			if err != nil {
				return err
			}

			site, err := c.ResolveSite(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Deploying %s to site %s...\n", dir, args[0])
			deployment, err := c.Deploy(site.ID, dir)
			if err != nil {
				return err
			}

			fmt.Printf("Deployed! Version: v%d\n", deployment.Version)

			if spaFlag && !site.SPAMode {
				spaOn := true
				if _, err := c.UpdateSite(site.ID, nil, &spaOn); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: deployed OK but failed to enable SPA mode: %s\n", err)
				} else {
					fmt.Println("SPA mode enabled.")
				}
			} else if !spaFlag && !site.SPAMode && detectSPA(dir) {
				fmt.Println("\nThis looks like a single-page app (SPA).")
				fmt.Println("Client-side routing won't work without SPA mode or a _redirects file.")
				fmt.Println("To enable SPA mode, re-deploy with --spa or toggle it in the dashboard.")
			}

			return nil
		},
	}
	cmd.Flags().BoolVar(&spaFlag, "spa", false, "enable SPA mode for this site")
	return cmd
}

// detectSPA checks if a directory looks like a single-page application.
// Heuristic: has index.html with <script> tags, few other .html files,
// and no _redirects file with a catch-all rewrite.
func detectSPA(dir string) bool {
	// Must have index.html
	indexPath := filepath.Join(dir, "index.html")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return false
	}

	// index.html should reference JS bundles
	content := strings.ToLower(string(indexData))
	if !strings.Contains(content, "<script") {
		return false
	}

	// Check for _redirects with SPA fallback — if present, no need to warn
	if redirectsData, err := os.ReadFile(filepath.Join(dir, "_redirects")); err == nil {
		for _, line := range strings.Split(string(redirectsData), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "/*") && strings.Contains(line, "200") {
				return false
			}
		}
	}

	// Count .html files — SPAs typically have very few (index.html + maybe 404.html)
	htmlCount := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".html") {
			htmlCount++
			if htmlCount > 2 {
				return filepath.SkipAll
			}
		}
		return nil
	})

	return htmlCount <= 2
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("hostedat %s\n", version)
			if defaultServer != "" {
				fmt.Printf("server:  %s\n", defaultServer)
			}
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
	c := client.New(serverURL, apiKey)
	c.Version = version
	return c, nil
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
	if defaultServer != "" {
		return defaultServer
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
