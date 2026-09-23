package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// passwordCmd handles `keryx password` — change your password (current → new).
// No flags for either password, same as login: a flag lands in shell history.
// Current comes from POLEIA_PASSWORD or an echo-free prompt; new from
// POLEIA_NEW_PASSWORD or an echo-free prompt entered twice.
//
// Afterwards every refresh token is revoked server-side; this CLI's saved
// access token keeps working until it expires, then `keryx login` again.
func passwordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "password",
		Short: "Change your password (asks for the current one first)",
		Long: "Change your password. Reads the current password from POLEIA_PASSWORD and the new one\n" +
			"from POLEIA_NEW_PASSWORD, or prompts for both (the new one twice) in a terminal.",
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			tty := term.IsTerminal(int(os.Stdin.Fd()))
			oldPw, err := resolvePassword(os.Getenv("POLEIA_PASSWORD"), tty, func() (string, error) {
				return promptSecret("Current password: ")
			})
			if err != nil {
				return err
			}
			newPw, err := resolveNewPassword(os.Getenv("POLEIA_NEW_PASSWORD"), tty, promptSecret)
			if err != nil {
				return err
			}
			if _, err := newClient(cfg).post("/api/v1/auth/password", map[string]string{
				"old_password": oldPw,
				"new_password": newPw,
			}); err != nil {
				return err
			}
			if jsonMode {
				printJSON(map[string]string{"status": "ok"})
				return nil
			}
			fmt.Println("Password changed. Other sessions can no longer renew themselves; log in again with the new password when this one expires.")
			return nil
		},
	}
}

// resolveNewPassword mirrors resolvePassword for the new password, but a
// prompted one is asked for twice — a typo here locks the player out, and
// there is no self-service reset.
func resolveNewPassword(env string, stdinIsTerminal bool, prompt func(label string) (string, error)) (string, error) {
	if env != "" {
		return env, nil
	}
	if !stdinIsTerminal {
		return "", fmt.Errorf("no new password available: set POLEIA_NEW_PASSWORD or run 'keryx password' in a terminal")
	}
	first, err := prompt("New password: ")
	if err != nil {
		return "", err
	}
	second, err := prompt("New password again: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", fmt.Errorf("the two new passwords differ — nothing was changed")
	}
	return first, nil
}

func promptSecret(label string) (string, error) {
	fmt.Print(label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(b), nil
}
