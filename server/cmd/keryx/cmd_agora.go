package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"net/http"
)

type agoraStatus struct {
	Enabled    bool   `json:"enabled"`
	State      string `json:"state"`
	Localpart  string `json:"localpart"`
	Homeserver string `json:"homeserver"`
	UserID     string `json:"user_id"`
}

func agoraCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "agora", Short: "Show your community chat account (outside the game)", Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := newClient(cfg).get("/api/v1/agora")
			if err != nil {
				return err
			}
			var status agoraStatus
			if err := json.Unmarshal(data, &status); err != nil {
				return fmt.Errorf("read chat account: %w", err)
			}
			if jsonMode {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return err
			}
			switch {
			case !status.Enabled:
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Community chat is not enabled on this server.")
			case status.State == "ready":
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Community chat: %s\nHomeserver: %s\nRun 'keryx agora password' to set and display a new chat password once. This replaces the previous chat password.\n", status.UserID, status.Homeserver)
			case status.State == "provisioning":
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Your community chat account is being created. Run 'keryx agora' again later.")
			default:
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Your community chat account is created after you own your first city. If you already do, account creation is pending; check again later.")
			}
			return err
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use: "password", Short: "Set and display a new chat password once (replaces the old one)", Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			// No generic API error-body printer: upstream text must not leak a secret.
			data, status, err := newClient(cfg).do(http.MethodPost, "/api/v1/agora/password", nil)
			if err != nil {
				return fmt.Errorf("chat password request failed; try again later")
			}
			if status >= 400 {
				if status == http.StatusConflict {
					return fmt.Errorf("chat account is not ready; run 'keryx agora' first")
				}
				return fmt.Errorf("chat password request failed (HTTP %d); try again later", status)
			}
			var secret struct {
				Password   string `json:"password"`
				UserID     string `json:"user_id"`
				Homeserver string `json:"homeserver"`
			}
			if err := json.Unmarshal(data, &secret); err != nil || secret.Password == "" {
				return fmt.Errorf("chat server returned no password; try again later")
			}
			// Only this explicitly invoked command emits the password; never save config.
			if jsonMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(secret)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Chat ID: %s\nHomeserver: %s\nNew chat password (shown once): %s\nThe previous chat password no longer works. Save this password before closing your terminal.\n", secret.UserID, secret.Homeserver, secret.Password)
			return err
		},
	})
	return cmd
}
