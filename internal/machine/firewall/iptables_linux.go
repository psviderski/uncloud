package firewall

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/docker/docker/libnetwork/iptables"
	"github.com/psviderski/uncloud/internal/machine/constants"
	"github.com/psviderski/uncloud/internal/machine/corroservice"
	"github.com/psviderski/uncloud/internal/machine/network"
)

const (
	DockerUserChain   = "DOCKER-USER"
	UncloudInputChain = "UNCLOUD-INPUT"
)

// ConfigureIptablesChains sets up custom iptables chains and initial firewall rules for Uncloud networking.
func ConfigureIptablesChains() error {
	if err := createIptablesChains(); err != nil {
		return err
	}

	ipt4 := iptables.GetIptable(iptables.IPv4)
	ipt6 := iptables.GetIptable(iptables.IPv6)

	// Allow WireGuard traffic to the machine.
	acceptWireGuardRule := []string{"-p", "udp", "--dport", strconv.Itoa(network.WireGuardPort), "-j", "ACCEPT"}
	err := ipt4.ProgramRule(iptables.Filter, UncloudInputChain, iptables.Insert, acceptWireGuardRule)
	if err != nil {
		return fmt.Errorf("insert iptables rule '%s': %w", strings.Join(acceptWireGuardRule, " "), err)
	}

	// Allow cluster machines to access Machine API via the management IPv6 WireGuard network.
	acceptMachineAPIRule := []string{
		"-i", network.WireGuardInterfaceName,
		"-s", "fdcc::/16",
		"-p", "tcp",
		"--dport", strconv.Itoa(constants.MachineAPIPort),
		"-j", "ACCEPT",
	}
	// Allow Corrosion gossip traffic from cluster machines via the management IPv6 WireGuard network.
	acceptCorrosionGossipRule := []string{
		"-i", network.WireGuardInterfaceName,
		"-s", "fdcc::/16",
		"-p", "udp",
		"--dport", strconv.Itoa(corroservice.DefaultGossipPort),
		"-j", "ACCEPT",
	}
	for _, rule := range [][]string{acceptMachineAPIRule, acceptCorrosionGossipRule} {
		if err = ipt6.ProgramRule(iptables.Filter, UncloudInputChain, iptables.Insert, rule); err != nil {
			return fmt.Errorf("insert ip6tables rule '%s': %w", strings.Join(rule, " "), err)
		}
	}

	return nil
}

// createIptablesChains ensures UNCLOUD-INPUT iptables and ip6tables chains exist and
// there are jump rules from the main INPUT chains.
func createIptablesChains() error {
	ipt4 := iptables.GetIptable(iptables.IPv4)
	ipt6 := iptables.GetIptable(iptables.IPv6)

	for i, ipt := range []*iptables.IPTable{ipt4, ipt6} {
		iptBin := "iptables"
		if i == 1 {
			iptBin = "ip6tables"
		}

		// Ensure UNCLOUD-INPUT chain exists. All existing rules are flushed.
		if _, err := ipt.NewChain(UncloudInputChain, iptables.Filter); err != nil {
			return fmt.Errorf("create %s chain '%s': %w", iptBin, UncloudInputChain, err)
		}
		if err := ipt.RawCombinedOutput("-t", string(iptables.Filter), "-F", UncloudInputChain); err != nil {
			return fmt.Errorf("flush %s chain '%s': %w", iptBin, UncloudInputChain, err)
		}

		// Ensure the main INPUT chain has a jump rule to the UNCLOUD-INPUT chain before any DROP/REJECT rules.
		jumpRule := []string{"-m", "comment", "--comment", "Uncloud-managed", "-j", UncloudInputChain}
		if !ipt.Exists(iptables.Filter, "INPUT", jumpRule...) {
			// Look for the first DROP/REJECT rule in the INPUT chain.
			out, err := ipt.Raw("-t", string(iptables.Filter), "-L", "INPUT", "--line-numbers")
			if err != nil {
				return fmt.Errorf("get %s rules for chain '%s': %w", iptBin, UncloudInputChain, err)
			}

			firstRejectRuleNum := 0
			for _, line := range strings.Split(string(out), "\n") {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					continue
				}
				if fields[1] == "DROP" || fields[1] == "REJECT" {
					if ruleNum, err := strconv.Atoi(fields[0]); err == nil {
						firstRejectRuleNum = ruleNum
						break
					}
				}
			}

			var addJumpRule []string
			if firstRejectRuleNum > 0 {
				addJumpRule = append([]string{"-t", string(iptables.Filter), "-I", "INPUT",
					strconv.Itoa(firstRejectRuleNum)}, jumpRule...)
			} else {
				addJumpRule = append([]string{"-t", string(iptables.Filter), "-A", "INPUT"}, jumpRule...)
			}
			if err = ipt.RawCombinedOutput(addJumpRule...); err != nil {
				return fmt.Errorf("add %s rule '%s': %w", iptBin, strings.Join(addJumpRule, " "), err)
			}
		}
	}

	return nil
}

// CleanupIptablesChains removes the custom iptables chains and rules created by ConfigureIptablesChains.
func CleanupIptablesChains() error {
	ipt4 := iptables.GetIptable(iptables.IPv4)
	ipt6 := iptables.GetIptable(iptables.IPv6)

	for i, ipt := range []*iptables.IPTable{ipt4, ipt6} {
		iptBin := "iptables"
		if i == 1 {
			iptBin = "ip6tables"
		}

		// First, remove the jump rule from INPUT chain to UNCLOUD-INPUT.
		jumpRule := []string{"-m", "comment", "--comment", "Uncloud-managed", "-j", UncloudInputChain}
		if err := ipt.ProgramRule(iptables.Filter, "INPUT", iptables.Delete, jumpRule); err != nil {
			return fmt.Errorf("delete %s jump rule from INPUT: %w", iptBin, err)
		}

		// Flush all rules from UNCLOUD-INPUT chain as it must be empty before deletion.
		if err := ipt.RawCombinedOutput("-t", string(iptables.Filter), "-F", UncloudInputChain); err != nil {
			// Chain might not exist which is fine.
			if !strings.Contains(err.Error(), "No chain") {
				return fmt.Errorf("flush %s chain '%s': %w", iptBin, UncloudInputChain, err)
			}
		}

		// Delete the UNCLOUD-INPUT chain.
		if err := ipt.RawCombinedOutput("-t", string(iptables.Filter), "-X", UncloudInputChain); err != nil {
			// Chain might not exist which is fine.
			if !strings.Contains(err.Error(), "No chain") {
				return fmt.Errorf("delete %s chain '%s': %w", iptBin, UncloudInputChain, err)
			}
		} else {
			slog.Info(fmt.Sprintf("Deleted %s chain.", iptBin), "chain", UncloudInputChain)
		}
	}

	return nil
}
