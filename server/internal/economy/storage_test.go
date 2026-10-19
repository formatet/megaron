package economy

import "testing"

func TestStorageProjectionRespectsArrivalTimeAndNetDrain(t *testing.T) {
	cases := []struct {
		name       string
		stock, net float64
		receipts   []StorageReceipt
		want       *float64
		warning    bool
	}{
		{"not filling", 0, 0, nil, nil, false},
		{"full now", 60, -5, nil, storageTime(0), true},
		{"production at horizon", 12, 2, nil, storageTime(24), true},
		{"production outside horizon", 10, 2, nil, storageTime(25), false},
		{"distant cargo is not already stock", 0, 0, []StorageReceipt{{30, 150}}, storageTime(30), false},
		{"known cargo before horizon", 0, 0, []StorageReceipt{{20, 150}}, storageTime(20), true},
		{"drain before arrival", 50, -5, []StorageReceipt{{4, 40}}, storageTime(4), true},
		{"drain can consume a small arrival", 50, -5, []StorageReceipt{{4, 5}}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ProjectStorage("silver", tc.stock, 60, tc.net, tc.receipts)
			if got.Warning != tc.warning || (got.FullInTicks == nil) != (tc.want == nil) {
				t.Fatalf("forecast%+v want%v warning%v", got, tc.want, tc.warning)
			}
			if tc.want != nil && *got.FullInTicks != *tc.want {
				t.Fatalf("fills%g want%g", *got.FullInTicks, *tc.want)
			}
		})
	}
}
func storageTime(n float64) *float64 { return &n }
