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
	commit        = "unknown"
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
	root.AddCommand(storageCmd())
	root.AddCommand(storageCredentialsCmd())
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

func storageCmd() *cobra.Command {
	storage := &cobra.Command{
		Use:   "storage",
		Short: "Manage storage buckets for a site",
	}

	// list
	list := &cobra.Command{
		Use:   "list <site>",
		Short: "List storage buckets for a site",
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

			buckets, err := c.ListBuckets(siteID)
			if err != nil {
				return err
			}

			if len(buckets) == 0 {
				fmt.Println("No storage buckets found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tBINDING\tBUCKET NAME\tPUBLIC\tCREATED")
			for _, b := range buckets {
				pub := "no"
				if b.Public {
					pub = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					b.ID, b.Name, b.BucketName, pub, b.CreatedAt.Format("2006-01-02"))
			}
			w.Flush()
			return nil
		},
	}

	// create
	var bindingName, bucketName string
	var publicFlag bool
	create := &cobra.Command{
		Use:   "create <site>",
		Short: "Create a storage bucket for a site",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if bindingName == "" || bucketName == "" {
				return fmt.Errorf("both --name and --bucket are required")
			}

			c, err := newClient()
			if err != nil {
				return err
			}

			siteID, err := c.ResolveSiteID(args[0])
			if err != nil {
				return err
			}

			bucket, err := c.CreateBucket(siteID, bindingName, bucketName, publicFlag)
			if err != nil {
				return err
			}

			fmt.Printf("Storage bucket created!\n")
			fmt.Printf("  ID:          %s\n", bucket.ID)
			fmt.Printf("  Binding:     %s\n", bucket.Name)
			fmt.Printf("  Bucket name: %s\n", bucket.BucketName)
			fmt.Printf("  Public:      %v\n", bucket.Public)
			return nil
		},
	}
	create.Flags().StringVar(&bindingName, "name", "", "binding name (e.g. IMAGES)")
	create.Flags().StringVar(&bucketName, "bucket", "", "S3 bucket name (must start with site ID)")
	create.Flags().BoolVar(&publicFlag, "public", false, "allow unauthenticated read access")

	// update
	var setPublic, setPrivate bool
	update := &cobra.Command{
		Use:   "update <site> <bucket-id>",
		Short: "Update storage bucket settings",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if setPublic == setPrivate {
				return fmt.Errorf("exactly one of --public or --private is required")
			}

			c, err := newClient()
			if err != nil {
				return err
			}

			siteID, err := c.ResolveSiteID(args[0])
			if err != nil {
				return err
			}

			public := setPublic
			bucket, err := c.UpdateBucket(siteID, args[1], public)
			if err != nil {
				return err
			}

			fmt.Println("Storage bucket updated!")
			fmt.Printf("  ID:          %s\n", bucket.ID)
			fmt.Printf("  Binding:     %s\n", bucket.Name)
			fmt.Printf("  Bucket name: %s\n", bucket.BucketName)
			fmt.Printf("  Public:      %v\n", bucket.Public)
			return nil
		},
	}
	update.Flags().BoolVar(&setPublic, "public", false, "set bucket to public read")
	update.Flags().BoolVar(&setPrivate, "private", false, "set bucket to private")
	update.MarkFlagsMutuallyExclusive("public", "private")

	// delete
	var yes bool
	del := &cobra.Command{
		Use:   "delete <site> <bucket-id>",
		Short: "Delete a storage bucket",
		Args:  cobra.ExactArgs(2),
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
				fmt.Printf("Delete storage bucket %s? This will remove all stored data. [y/N] ", args[1])
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(answer)) != "y" {
					fmt.Println("Aborted.")
					return nil
				}
			}

			if err := c.DeleteBucket(siteID, args[1]); err != nil {
				return err
			}

			fmt.Println("Storage bucket deleted.")
			return nil
		},
	}
	del.Flags().BoolVar(&yes, "yes", false, "skip confirmation")

	// upload
	var uploadKey string
	upload := &cobra.Command{
		Use:   "upload <site> <bucket-id> <file>",
		Short: "Upload a file to a storage bucket",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}

			siteID, err := c.ResolveSiteID(args[0])
			if err != nil {
				return err
			}

			data, err := os.ReadFile(args[2])
			if err != nil {
				return fmt.Errorf("reading file: %w", err)
			}

			key := uploadKey
			if key == "" {
				key = filepath.Base(args[2])
			}

			if err := c.UploadToBucket(siteID, args[1], key, data); err != nil {
				return err
			}

			fmt.Printf("Uploaded %s as %s\n", args[2], key)
			return nil
		},
	}
	upload.Flags().StringVar(&uploadKey, "key", "", "object key (defaults to filename)")

	storage.AddCommand(list, create, update, del, upload)
	return storage
}

func storageCredentialsCmd() *cobra.Command {
	creds := &cobra.Command{
		Use:     "storage-credentials",
		Aliases: []string{"storage-creds"},
		Short:   "Manage S3-compatible storage credentials",
	}

	// list
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List your storage credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}

			credentials, err := c.ListStorageCredentials()
			if err != nil {
				return err
			}

			if len(credentials) == 0 {
				fmt.Println("No storage credentials found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tACCESS KEY ID\tLAST USED\tCREATED")
			for _, cr := range credentials {
				lastUsed := "Never"
				if cr.LastUsedAt != nil {
					lastUsed = cr.LastUsedAt.Format("2006-01-02")
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					cr.ID, cr.Name, cr.AccessKeyID, lastUsed, cr.CreatedAt.Format("2006-01-02"))
			}
			w.Flush()
			return nil
		},
	}

	// create
	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a storage credential (secret is shown once)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}

			cred, err := c.CreateStorageCredential(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Storage credential created!\n")
			fmt.Printf("  Name:              %s\n", cred.Name)
			fmt.Printf("  Access Key ID:     %s\n", cred.AccessKeyID)
			fmt.Printf("  Secret Access Key: %s\n", cred.SecretAccessKey)
			fmt.Println()
			fmt.Println("Save the secret access key now — it will not be shown again.")
			return nil
		},
	}

	// delete
	var yes bool
	del := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a storage credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}

			if !yes {
				fmt.Printf("Delete storage credential %s? Any integrations using it will stop working. [y/N] ", args[0])
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(answer)) != "y" {
					fmt.Println("Aborted.")
					return nil
				}
			}

			if err := c.DeleteStorageCredential(args[0]); err != nil {
				return err
			}

			fmt.Println("Storage credential deleted.")
			return nil
		},
	}
	del.Flags().BoolVar(&yes, "yes", false, "skip confirmation")

	creds.AddCommand(list, create, del)
	return creds
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("hostedat %s (%s)\n", version, commit)
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
