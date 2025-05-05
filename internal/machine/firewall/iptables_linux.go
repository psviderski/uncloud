package firewall

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/docker/libnetwork/iptables"
	"github.com/psviderski/uncloud/internal/machine/network"
)

const (
	DockerUserChain   = "DOCKER-USER"
	UncloudInputChain = "UNCLOUD-INPUT"
)

// ConfigureIptablesChains sets up custom iptables chains and initial firewall rules for Uncloud networking.
func ConfigureIptablesChains() error {
	// Ensure iptables UNCLOUD-INPUT chain with a RETURN rule exists. All existing rules are flushed.
	ipt := iptables.GetIptable(iptables.IPv4)
	if _, err := ipt.NewChain(UncloudInputChain, iptables.Filter); err != nil {
		return fmt.Errorf("create iptables chain '%s': %w", UncloudInputChain, err)
	}
	if err := ipt.RawCombinedOutput("-t", string(iptables.Filter), "-F", UncloudInputChain); err != nil {
		return fmt.Errorf("flush iptables chain '%s': %w", UncloudInputChain, err)
	}
	if err := ipt.AddReturnRule(UncloudInputChain); err != nil {
		return fmt.Errorf("add the RETURN rule for iptables chain '%s': %w", UncloudInputChain, err)
	}

	// Ensure the main iptables INPUT chain has a jump rule to the UNCLOUD-INPUT chain before any DROP/REJECT rules.
	jumpRule := []string{"-m", "comment", "--comment", "Uncloud-managed", "-j", UncloudInputChain}
	if !ipt.Exists(iptables.Filter, "INPUT", jumpRule...) {
		// Look for the first DROP/REJECT rule in the INPUT chain.
		out, err := ipt.Raw("-t", string(iptables.Filter), "-L", "INPUT", "--line-numbers")
		if err != nil {
			return fmt.Errorf("get iptables rules for chain '%s': %w", UncloudInputChain, err)
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
			addJumpRule = append([]string{"-t", string(iptables.Filter), "-I", "INPUT", strconv.Itoa(firstRejectRuleNum)},
				jumpRule...)
		} else {
			addJumpRule = append([]string{"-t", string(iptables.Filter), "-A", "INPUT"}, jumpRule...)
		}
		if err = ipt.RawCombinedOutput(addJumpRule...); err != nil {
			return fmt.Errorf("add iptables rule '%s': %w", strings.Join(addJumpRule, " "), err)
		}
	}

	// Allow WireGuard traffic to the machine.
	acceptWireGuardRule := []string{"-p", "udp", "--dport", strconv.Itoa(network.WireGuardPort), "-j", "ACCEPT"}
	err := ipt.ProgramRule(iptables.Filter, UncloudInputChain, iptables.Insert, acceptWireGuardRule)
	if err != nil {
		return fmt.Errorf("insert iptables rule '%s': %w", strings.Join(acceptWireGuardRule, " "), err)
	}

	return nil
}
