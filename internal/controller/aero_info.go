package controller

import (
	"fmt"
	"strconv"
	"strings"

	aero "github.com/aerospike/aerospike-client-go/v8"
	"github.com/go-logr/logr"
)

// asinfoCommand executes an asinfo command on a specific node.
func asinfoCommand(client *aero.Client, cmd string) (string, error) {
	nodes := client.GetNodes()
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes available")
	}
	if nodes[0] == nil {
		return "", fmt.Errorf("first node is nil")
	}

	policy := aero.NewInfoPolicy()
	policy.Timeout = aeroInfoTimeout

	result, err := nodes[0].RequestInfo(policy, cmd)
	if err != nil {
		return "", fmt.Errorf("asinfo command %q failed: %w", cmd, err)
	}

	if val, ok := result[cmd]; ok {
		return val, nil
	}

	return "", fmt.Errorf("no result for command %q", cmd)
}

// asinfoCommandOnNode executes an asinfo command on a specific node by address.
func asinfoCommandOnNode(node *aero.Node, cmd string) (string, error) {
	policy := aero.NewInfoPolicy()
	policy.Timeout = aeroInfoTimeout

	result, err := node.RequestInfo(policy, cmd)
	if err != nil {
		return "", fmt.Errorf("asinfo command %q on node %s failed: %w", cmd, node.GetName(), err)
	}

	if val, ok := result[cmd]; ok {
		return val, nil
	}

	return "", fmt.Errorf("no result for command %q on node %s", cmd, node.GetName())
}

// isMigratingOnAnyNode checks whether any node in the cluster has outstanding
// partition migrations. Uses migrate_partitions_remaining which is supported
// in Aerospike CE 7.x and 8.x (migrate_progress_send/recv are removed in 8.x).
func isMigratingOnAnyNode(client *aero.Client) (bool, error) {
	nodes := client.GetNodes()
	if len(nodes) == 0 {
		return false, fmt.Errorf("no nodes available in Aerospike cluster")
	}

	for _, node := range nodes {
		if node == nil {
			continue
		}
		stats, err := asinfoCommandOnNode(node, "statistics")
		if err != nil {
			return true, fmt.Errorf("statistics command on node %s failed: %w", node.GetName(), err)
		}
		remaining, ok := parseMigrateStat(stats, "migrate_partitions_remaining")
		if !ok {
			// An unparseable migration statistic is treated conservatively as
			// "migrating" — the same way a statistics-command error is handled
			// above — so a destructive scale-down or rolling restart never
			// proceeds on a value we could not verify as zero.
			return true, fmt.Errorf("could not parse migrate_partitions_remaining on node %s", node.GetName())
		}
		if remaining > 0 {
			return true, nil
		}
	}
	return false, nil
}

// parseMigrateStat extracts a numeric migration statistic from the asinfo
// "statistics" response. The response is semicolon-delimited key=value pairs.
// The second return value is false when the key is absent OR when its value
// cannot be parsed as an integer; callers must not treat a false result as
// "migration complete".
func parseMigrateStat(stats, key string) (int64, bool) {
	for pair := range strings.SplitSeq(stats, ";") {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(k) == key {
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return 0, false
			}
			return n, true
		}
	}
	return 0, false
}

// migrateUncertainSentinel is the migrate_partitions_remaining value recorded for
// a node whose migration state could not be confirmed (the statistics command
// errored, or its value was absent/unparseable). A positive sentinel makes the
// node count as "migrating" downstream so a destructive scale-down or rolling
// restart never proceeds on a value we could not verify as zero.
const migrateUncertainSentinel int64 = 1

// migrateRemainingForNode decides the migrate_partitions_remaining value to
// record for a single node, given the raw statistics response and the error (if
// any) from the statistics command. A failed command, an absent key, or an
// unparseable value all resolve to migrateUncertainSentinel so the node is
// reported as migrating rather than treated as complete. ok reports whether the
// value was confirmed (true) or is the uncertainty sentinel (false); callers use
// it for logging only.
func migrateRemainingForNode(stats string, statsErr error) (remaining int64, confirmed bool) {
	if statsErr != nil {
		return migrateUncertainSentinel, false
	}
	n, ok := parseMigrateStat(stats, "migrate_partitions_remaining")
	if !ok {
		return migrateUncertainSentinel, false
	}
	return n, true
}

// migrateStatsPerNode returns the migrate_partitions_remaining count for each node
// in the cluster. The map is keyed by the node's host IP address.
//
// A node whose statistics command fails (or returns an absent/unparseable
// migration stat) is recorded with a positive sentinel — NOT silently dropped —
// so that an unreachable subset of nodes cannot make the cluster look like
// migration is complete. Previously such nodes were skipped, so if every
// reachable node happened to report 0 remaining partitions while one node was
// unreachable, the status path (applyMigrationStats) would publish
// MigrationComplete=True / RemainingPartitions=0 even though migration state was
// unknown on that node — the same false-negative that isMigratingOnAnyNode
// guards against. A node is only dropped when its host IP cannot be resolved (so
// there is no map key to record it under); those drops are counted toward the
// all-nodes-failed error below.
func migrateStatsPerNode(log logr.Logger, client *aero.Client) (map[string]int64, error) {
	nodes := client.GetNodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available in Aerospike cluster")
	}

	result := make(map[string]int64, len(nodes))
	var droppedCount int
	for _, node := range nodes {
		if node == nil {
			continue
		}
		host := node.GetHost()
		if host == nil {
			// No IP to key this node under; cannot record it. Count it so a
			// cluster where every node lacks a host still surfaces an error.
			droppedCount++
			log.V(1).Info("Skipping node with no host info for migration stats", "node", node.GetName())
			continue
		}

		stats, err := asinfoCommandOnNode(node, "statistics")
		remaining, confirmed := migrateRemainingForNode(stats, err)
		if !confirmed {
			log.V(1).Info("Migration state unconfirmed, reporting node as migrating",
				"node", node.GetName(), "error", err)
		}
		result[host.Name] = remaining
	}

	// If no node could be recorded at all, surface an error so the caller keeps
	// the previous (stale-but-safe) MigrationStatus instead of treating an empty
	// map as "migration complete".
	if droppedCount > 0 && len(result) == 0 {
		return nil, fmt.Errorf("all %d node(s) lacked host info or were unreachable for statistics", droppedCount)
	}
	return result, nil
}

// clusterSize returns the number of nodes in the Aerospike cluster as reported by asinfo.
// Returns 0 and an error if the cluster is unreachable or the response cannot be parsed.
func clusterSize(client *aero.Client) (int, error) {
	result, err := asinfoCommand(client, "cluster-size")
	if err != nil {
		return 0, err
	}
	size, err := strconv.Atoi(strings.TrimSpace(result))
	if err != nil {
		return 0, fmt.Errorf("parsing cluster-size response %q: %w", result, err)
	}
	return size, nil
}
