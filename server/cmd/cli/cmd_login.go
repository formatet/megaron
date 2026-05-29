package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func loginCmd() *cobra.Command {
	var server, username, password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and save credentials",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if server == "" {
				server = "http://localhost:8080"
			}

			if username == "" {
				fmt.Print("Username or email: ")
				fmt.Scan(&username)
			}
			if password == "" {
				fmt.Print("Password: ")
				b, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println()
				if err != nil {
					// fallback for piped input (MCP/scripts)
					fmt.Scan(&password)
				} else {
					password = string(b)
				}
			}

			c := &Client{server: server, http: newClient(&Config{Server: server}).http}
			data, err := c.post("/api/v1/auth/login", map[string]string{
				"username_or_email": username,
				"password":          password,
			})
			if err != nil {
				return err
			}

			var resp struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}

			cfg := &Config{
				Server:   server,
				Token:    resp.AccessToken,
				Username: username,
			}

			// Auto-detect world and province.
			authed := &Client{server: server, token: resp.AccessToken, http: c.http}
			worlds, worldID, err := autoDetectWorld(authed)
			if err == nil && worldID != "" {
				cfg.WorldID = worldID
				if len(worlds) > 1 {
					fmt.Fprintf(os.Stderr, "Multiple worlds found — using %q. Run 'poleia worlds' to see all.\n", worldID)
				}
				// Find player's own province.
				if pid := autoDetectProvince(authed, worldID); pid != "" {
					cfg.ProvinceID = pid
				}
			}

			if err := saveConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			if jsonMode {
				printJSON(map[string]string{"status": "ok", "server": server, "world_id": cfg.WorldID, "province_id": cfg.ProvinceID})
				return nil
			}
			fmt.Printf("Logged in to %s\n", server)
			if cfg.WorldID != "" {
				fmt.Printf("Active world:   %s\n", cfg.WorldID)
				fmt.Printf("Your province:  %s\n", cfg.ProvinceID)
			} else {
				fmt.Println("No active world found — run 'poleia worlds' to list available worlds.")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&server, "server", "s", "", "server URL (default: http://localhost:8080)")
	cmd.Flags().StringVarP(&username, "username", "u", "", "username or email")
	cmd.Flags().StringVarP(&password, "password", "p", "", "password (use env var POLEIA_PASSWORD for scripts)")
	return cmd
}

func autoDetectWorld(c *Client) ([]map[string]any, string, error) {
	data, err := c.get("/api/v1/worlds")
	if err != nil {
		return nil, "", err
	}
	var worlds []map[string]any
	if err := json.Unmarshal(data, &worlds); err != nil || len(worlds) == 0 {
		return nil, "", fmt.Errorf("no worlds")
	}
	for _, w := range worlds {
		if w["state"] == "active" {
			if id, ok := w["id"].(string); ok {
				return worlds, id, nil
			}
		}
	}
	if id, ok := worlds[0]["id"].(string); ok {
		return worlds, id, nil
	}
	return nil, "", fmt.Errorf("no world id")
}

func autoDetectProvince(c *Client, worldID string) string {
	data, err := c.get("/api/v1/worlds/" + worldID + "/provinces")
	if err != nil {
		return ""
	}
	var markers []map[string]any
	if err := json.Unmarshal(data, &markers); err != nil {
		return ""
	}
	for _, m := range markers {
		if own, _ := m["own"].(bool); own {
			if id, ok := m["id"].(string); ok {
				return id
			}
		}
	}
	return ""
}
