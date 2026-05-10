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
		remaining := parseMigrateStat(stats, "migrate_partitions_remaining")
		if remaining > 0 {
			return true, nil
		}
	}
	return false, nil
}

// parseMigrateStat extracts a numeric migration statistic from the asinfo
// "statistics" response. The response is semicolon-delimited key=value pairs.
// Returns 0 if the key is not found or cannot be parsed.
func parseMigrateStat(stats, key string) int64 {
	for pair := range strings.SplitSeq(stats, ";") {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(k) == key {
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return 0
			}
			return n
		}
	}
	return 0
}

// migrateStatsPerNode returns the migrate_partitions_remaining count for each node
// in the cluster. The map is keyed by the node's host IP address.
// If a single node is unreachable, the error is logged and that node is skipped.
// An error is returned only if ALL nodes fail.
func migrateStatsPerNode(log logr.Logger, client *aero.Client) (map[string]int64, error) {
	nodes := client.GetNodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available in Aerospike cluster")
	}

	result := make(map[string]int64, len(nodes))
	var errCount int
	for _, node := range nodes {
		if node == nil {
			continue
		}
		stats, err := asinfoCommandOnNode(node, "statistics")
		if err != nil {
			errCount++
			log.V(1).Info("Skipping node: statistics command failed", "node", node.GetName(), "error", err)
			continue
		}
		remaining := parseMigrateStat(stats, "migrate_partitions_remaining")
		host := node.GetHost()
		if host != nil {
			result[host.Name] = remaining
		}
	}

	// If every node failed, return an error.
	if errCount > 0 && len(result) == 0 {
		return nil, fmt.Errorf("all %d node(s) failed to respond to statistics command", errCount)
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
