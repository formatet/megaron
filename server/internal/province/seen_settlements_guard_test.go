package province

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryMemoryWriterRecordsSeenSettlements guards §S0's single owner: a
// hex entering player_scouted_tiles without RecordSeenSettlements beside it
// would hide the city standing there (memory shows no city without a
// snapshot). File-level source check over the server's production code.
func TestEveryMemoryWriterRecordsSeenSettlements(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(src), "INSERT INTO player_scouted_tiles") &&
			!strings.Contains(string(src), "RecordSeenSettlements(") {
			t.Errorf("%s writes player_scouted_tiles without province.RecordSeenSettlements", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
