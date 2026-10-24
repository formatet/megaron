package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"net/url"
)

func correspondencePrivateCmd() *cobra.Command {
	var caseID, draft, note string
	var version int64
	cmd := &cobra.Command{Use: "correspondence-private", Short: "Read or save your private case draft and note", Args: noPositionalArgs(), RunE: func(cmd *cobra.Command, _ []string) error {
		if caseID == "" {
			return fmt.Errorf("--case required")
		}
		saving := cmd.Flags().Changed("draft") || cmd.Flags().Changed("note") || cmd.Flags().Changed("version")
		if saving && (!cmd.Flags().Changed("draft") || !cmd.Flags().Changed("note") || !cmd.Flags().Changed("version")) {
			return fmt.Errorf("saving requires --draft, --note and --version together; read the saved version first")
		}
		if version < 0 {
			return fmt.Errorf("--version cannot be negative")
		}
		path := fmt.Sprintf("/api/v1/worlds/%s/correspondence/cases/%s/private", cfg.WorldID, url.PathEscape(caseID))
		c := newClient(cfg)
		var data []byte
		var err error
		if saving {
			data, err = c.put(path, map[string]any{"draft": draft, "note": note, "expected_version": version})
		} else {
			data, err = c.get(path)
		}
		if err != nil {
			return err
		}
		if jsonMode {
			printRawJSON(data)
			return nil
		}
		var doc struct {
			Draft   string `json:"draft"`
			Note    string `json:"note"`
			Version int64  `json:"version"`
		}
		if err = json.Unmarshal(data, &doc); err != nil {
			return err
		}
		fmt.Printf("Private case text · version %d · only you can read this\nDraft:\n%s\nNote:\n%s\n", doc.Version, doc.Draft, doc.Note)
		return nil
	}}
	cmd.Flags().StringVar(&caseID, "case", "", "case ID")
	cmd.Flags().StringVar(&draft, "draft", "", "complete draft text to save (empty clears it)")
	cmd.Flags().StringVar(&note, "note", "", "complete private note to save (empty clears it)")
	cmd.Flags().Int64Var(&version, "version", 0, "saved version returned by the last read")
	return cmd
}
