package controller

import (
	"errors"
	"testing"
)

func TestParseMigrateStat(t *testing.T) {
	tests := []struct {
		name   string
		stats  string
		key    string
		want   int64
		wantOK bool
	}{
		{
			name:   "migrate_partitions_remaining non-zero",
			stats:  "cluster_size=3;migrate_partitions_remaining=42;objects=1000000",
			key:    "migrate_partitions_remaining",
			want:   42,
			wantOK: true,
		},
		{
			name:   "migrate_partitions_remaining zero",
			stats:  "cluster_size=3;migrate_partitions_remaining=0;objects=1000000",
			key:    "migrate_partitions_remaining",
			want:   0,
			wantOK: true,
		},
		{
			name:   "key not found reports not-ok",
			stats:  "cluster_size=3;other_stat=10",
			key:    "migrate_partitions_remaining",
			want:   0,
			wantOK: false,
		},
		{
			name:   "empty stats reports not-ok",
			stats:  "",
			key:    "migrate_partitions_remaining",
			want:   0,
			wantOK: false,
		},
		{
			name:   "non-numeric value reports not-ok",
			stats:  "migrate_partitions_remaining=abc",
			key:    "migrate_partitions_remaining",
			want:   0,
			wantOK: false,
		},
		{
			name:   "key with spaces around value",
			stats:  "migrate_partitions_remaining = 10 ",
			key:    "migrate_partitions_remaining",
			want:   10,
			wantOK: true,
		},
		{
			name:   "partial key match does not match",
			stats:  "migrate_partitions_remaining_extra=5;migrate_partitions_remaining=3",
			key:    "migrate_partitions_remaining",
			want:   3,
			wantOK: true,
		},
		{
			name:   "large value",
			stats:  "migrate_partitions_remaining=999999999",
			key:    "migrate_partitions_remaining",
			want:   999999999,
			wantOK: true,
		},
		{
			name:   "realistic aerospike CE 8.x statistics response",
			stats:  "cluster_size=3;cluster_key=ABC123;cluster_integrity=true;migrate_partitions_remaining=100;objects=1000000",
			key:    "migrate_partitions_remaining",
			want:   100,
			wantOK: true,
		},
		{
			name:   "empty value reports not-ok",
			stats:  "migrate_partitions_remaining=;objects=10",
			key:    "migrate_partitions_remaining",
			want:   0,
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseMigrateStat(tc.stats, tc.key)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("parseMigrateStat(%q, %q) = (%d, %t), want (%d, %t)",
					tc.stats, tc.key, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestMigrateRemainingForNode(t *testing.T) {
	tests := []struct {
		name          string
		stats         string
		statsErr      error
		wantRemaining int64
		wantConfirmed bool
	}{
		{
			name:          "statistics command error reports node as migrating",
			stats:         "",
			statsErr:      errors.New("connection reset"),
			wantRemaining: migrateUncertainSentinel,
			wantConfirmed: false,
		},
		{
			name:          "confirmed zero is migration complete for this node",
			stats:         "cluster_size=3;migrate_partitions_remaining=0;objects=10",
			statsErr:      nil,
			wantRemaining: 0,
			wantConfirmed: true,
		},
		{
			name:          "confirmed non-zero is reported verbatim",
			stats:         "migrate_partitions_remaining=42",
			statsErr:      nil,
			wantRemaining: 42,
			wantConfirmed: true,
		},
		{
			name:          "absent key reports node as migrating",
			stats:         "cluster_size=3;objects=10",
			statsErr:      nil,
			wantRemaining: migrateUncertainSentinel,
			wantConfirmed: false,
		},
		{
			name:          "unparseable value reports node as migrating",
			stats:         "migrate_partitions_remaining=abc",
			statsErr:      nil,
			wantRemaining: migrateUncertainSentinel,
			wantConfirmed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			remaining, confirmed := migrateRemainingForNode(tc.stats, tc.statsErr)
			if remaining != tc.wantRemaining || confirmed != tc.wantConfirmed {
				t.Errorf("migrateRemainingForNode(%q, %v) = (%d, %t), want (%d, %t)",
					tc.stats, tc.statsErr, remaining, confirmed, tc.wantRemaining, tc.wantConfirmed)
			}
			// A node that is not confirmed must never read as "0 remaining",
			// which downstream (applyMigrationStats) would treat as complete.
			if !confirmed && remaining <= 0 {
				t.Errorf("unconfirmed node must report >0 remaining (got %d) so it counts as migrating", remaining)
			}
		})
	}
}
