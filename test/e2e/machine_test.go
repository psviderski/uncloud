package e2e

import (
	"context"
	"errors"
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMachineRename(t *testing.T) {
	t.Parallel()

	name := "ucind-test.machine-rename"
	ctx := context.Background()
	c, _ := createTestCluster(t, name, ucind.CreateClusterOptions{Machines: 3}, true)

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)
	defer cli.Close()

	t.Run("rename machine by name", func(t *testing.T) {
		// Get initial machine state
		machines, err := cli.ListMachines(ctx, nil)
		require.NoError(t, err)
		require.Len(t, machines, 3)

		// Select the second machine to rename
		originalMachine := machines[1]
		originalName := originalMachine.Machine.Name
		newName := "renamed-machine-1"

		// Rename the machine
		updatedMachine, err := cli.RenameMachine(ctx, originalName, newName)
		require.NoError(t, err)
		assert.Equal(t, newName, updatedMachine.Name)
		assert.Equal(t, originalMachine.Machine.Id, updatedMachine.Id)

		// Verify the machine list reflects the change
		machines, err = cli.ListMachines(ctx, nil)
		require.NoError(t, err)
		require.Len(t, machines, 3)

		// Find the renamed machine
		var found bool
		for _, m := range machines {
			if m.Machine.Id == originalMachine.Machine.Id {
				assert.Equal(t, newName, m.Machine.Name)
				found = true
			} else {
				// Ensure other machines are unaffected
				assert.NotEqual(t, newName, m.Machine.Name)
			}
		}
		assert.True(t, found, "Renamed machine should be in the list")

		// Verify we can inspect the machine by its new name
		inspectedMachine, err := cli.InspectMachine(ctx, newName)
		require.NoError(t, err)
		assert.Equal(t, newName, inspectedMachine.Machine.Name)
		assert.Equal(t, originalMachine.Machine.Id, inspectedMachine.Machine.Id)

		// Verify the old name no longer works
		_, err = cli.InspectMachine(ctx, originalName)
		assert.ErrorIs(t, err, api.ErrNotFound)
	})

	t.Run("rename machine by ID", func(t *testing.T) {
		// Get the third machine
		machines, err := cli.ListMachines(ctx, nil)
		require.NoError(t, err)
		require.Len(t, machines, 3)

		// Find a machine that hasn't been renamed yet
		var targetMachine *pb.MachineMember
		for _, m := range machines {
			if m.Machine.Name != "renamed-machine-1" {
				targetMachine = m
				break
			}
		}
		require.NotNil(t, targetMachine)

		originalName := targetMachine.Machine.Name
		machineID := targetMachine.Machine.Id
		newName := "renamed-machine-2"

		// Rename using ID instead of name
		updatedMachine, err := cli.RenameMachine(ctx, machineID, newName)
		require.NoError(t, err)
		assert.Equal(t, newName, updatedMachine.Name)
		assert.Equal(t, machineID, updatedMachine.Id)

		// Verify the rename was successful
		inspectedMachine, err := cli.InspectMachine(ctx, newName)
		require.NoError(t, err)
		assert.Equal(t, newName, inspectedMachine.Machine.Name)
		assert.Equal(t, machineID, inspectedMachine.Machine.Id)

		// Verify the old name no longer works
		_, err = cli.InspectMachine(ctx, originalName)
		assert.ErrorIs(t, err, api.ErrNotFound)
	})

	t.Run("rename non-existent machine", func(t *testing.T) {
		// Try to rename a machine that doesn't exist
		_, err := cli.RenameMachine(ctx, "non-existent-machine", "new-name")
		assert.ErrorIs(t, err, api.ErrNotFound)

		// Try with a non-existent ID
		_, err = cli.RenameMachine(ctx, "non-existent-id-12345", "new-name")
		assert.ErrorIs(t, err, api.ErrNotFound)
	})

	t.Run("rename to existing name", func(t *testing.T) {
		// Get current machines
		machines, err := cli.ListMachines(ctx, nil)
		require.NoError(t, err)
		require.Len(t, machines, 3)

		// Try to rename machine 0 to the name of machine 1
		machine0Name := machines[0].Machine.Name
		machine1Name := machines[1].Machine.Name

		// This should fail because the name is already taken
		_, err = cli.RenameMachine(ctx, machine0Name, machine1Name)
		assert.Error(t, err)
	})

	t.Run("rename with empty name", func(t *testing.T) {
		// Get a machine to rename
		machines, err := cli.ListMachines(ctx, nil)
		require.NoError(t, err)
		require.Len(t, machines, 3)

		machineName := machines[0].Machine.Name

		// Try to rename with empty string
		_, err = cli.RenameMachine(ctx, machineName, "")
		assert.Error(t, err)
	})

	t.Run("service continuity after rename", func(t *testing.T) {
		// Deploy a service on a specific machine
		machines, err := cli.ListMachines(ctx, nil)
		require.NoError(t, err)

		// Find a machine that hasn't been renamed to test with
		var targetMachine *pb.MachineMember
		for _, m := range machines {
			if m.Machine.Name != "renamed-machine-1" && m.Machine.Name != "renamed-machine-2" {
				targetMachine = m
				break
			}
		}
		require.NotNil(t, targetMachine)

		originalMachineName := targetMachine.Machine.Name
		serviceName := "test-service-rename-continuity"

		// Create a service on the specific machine
		spec := api.ServiceSpec{
			Name: serviceName,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
			Placement: api.Placement{
				Machines: []string{originalMachineName},
			},
		}

		_, err = cli.RunService(ctx, spec)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := cli.RemoveService(ctx, serviceName)
			if err != nil && !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}
		})

		// Verify service is running on the machine
		svc, err := cli.InspectService(ctx, serviceName)
		require.NoError(t, err)
		assert.Len(t, svc.Containers, 1)
		assert.Equal(t, targetMachine.Machine.Id, svc.Containers[0].MachineID)

		// Rename the machine
		newMachineName := "renamed-for-service-test"
		_, err = cli.RenameMachine(ctx, originalMachineName, newMachineName)
		require.NoError(t, err)

		// Verify service is still running on the renamed machine
		svc, err = cli.InspectService(ctx, serviceName)
		require.NoError(t, err)
		assert.Len(t, svc.Containers, 1)
		assert.Equal(t, targetMachine.Machine.Id, svc.Containers[0].MachineID)

		// The service spec's placement still references the old name,
		// but the service should continue to run on the same machine id
	})
}

func TestSetMachine(t *testing.T) {
	t.Parallel()

	name := "ucind-test.machine-set"
	ctx := context.Background()
	c, _ := createTestCluster(t, name, ucind.CreateClusterOptions{Machines: 3}, true)

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)
	defer cli.Close()

	t.Run("set machine name", func(t *testing.T) {
		// Get initial machine state
		machines, err := cli.ListMachines(ctx, nil)
		require.NoError(t, err)
		require.Len(t, machines, 3)

		// Select a machine to update
		targetMachine := machines[1]
		originalName := targetMachine.Machine.Name
		newName := "updated-machine-name"

		// Set the machine name using SetMachine directly
		req := &pb.SetMachineRequest{
			MachineId: targetMachine.Machine.Id,
			Name:      &newName,
		}
		updatedMachine, err := cli.SetMachine(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, newName, updatedMachine.Name)
		assert.Equal(t, targetMachine.Machine.Id, updatedMachine.Id)

		// Verify the change persisted
		inspected, err := cli.InspectMachine(ctx, updatedMachine.Id)
		require.NoError(t, err)
		assert.Equal(t, newName, inspected.Machine.Name)

		// Verify old name no longer works
		_, err = cli.InspectMachine(ctx, originalName)
		assert.ErrorIs(t, err, api.ErrNotFound)
	})

	t.Run("set machine public IP", func(t *testing.T) {
		// Get a machine to update
		machines, err := cli.ListMachines(ctx, nil)
		require.NoError(t, err)

		// Find a machine that hasn't been renamed
		var targetMachine *pb.MachineMember
		for _, m := range machines {
			if m.Machine.Name != "updated-machine-name" {
				targetMachine = m
				break
			}
		}
		require.NotNil(t, targetMachine)

		// Create a new public IP (must be a valid public IP address)
		newPublicIP := &pb.IP{
			Ip: []byte{8, 8, 8, 8},
		}

		// Set the public IP
		req := &pb.SetMachineRequest{
			MachineId: targetMachine.Machine.Id,
			PublicIp:  newPublicIP,
		}
		updatedMachine, err := cli.SetMachine(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, newPublicIP.Ip, updatedMachine.PublicIp.Ip)

		// Verify the change persisted
		inspected, err := cli.InspectMachine(ctx, targetMachine.Machine.Id)
		require.NoError(t, err)
		assert.Equal(t, newPublicIP.Ip, inspected.Machine.PublicIp.Ip)
	})

	t.Run("set machine network config", func(t *testing.T) {
		// Get a machine to update
		machines, err := cli.ListMachines(ctx, nil)
		require.NoError(t, err)

		var targetMachine *pb.MachineMember
		for _, m := range machines {
			targetMachine = m
			break
		}
		require.NotNil(t, targetMachine)

		// Store original network config
		originalNetwork := targetMachine.Machine.Network

		// Create new network config with updated endpoints
		newEndpoints := []*pb.IPPort{
			{
				Ip:   &pb.IP{Ip: []byte{10, 0, 0, 10}},
				Port: 8080,
			},
			{
				Ip:   &pb.IP{Ip: []byte{10, 0, 0, 10}},
				Port: 8443,
			},
		}

		newNetwork := &pb.NetworkConfig{
			Subnet:       originalNetwork.Subnet,
			ManagementIp: originalNetwork.ManagementIp,
			Endpoints:    newEndpoints,
			PublicKey:    originalNetwork.PublicKey,
		}

		// Set the network config
		req := &pb.SetMachineRequest{
			MachineId: targetMachine.Machine.Id,
			Network:   newNetwork,
		}
		updatedMachine, err := cli.SetMachine(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, len(newEndpoints), len(updatedMachine.Network.Endpoints))

		// Verify endpoints were updated
		for i, endpoint := range updatedMachine.Network.Endpoints {
			assert.Equal(t, newEndpoints[i].Ip.Ip, endpoint.Ip.Ip)
			assert.Equal(t, newEndpoints[i].Port, endpoint.Port)
		}

		// Verify other network fields remain unchanged
		assert.Equal(t, originalNetwork.Subnet.Ip.Ip, updatedMachine.Network.Subnet.Ip.Ip)
		assert.Equal(t, originalNetwork.Subnet.Bits, updatedMachine.Network.Subnet.Bits)
		assert.Equal(t, originalNetwork.ManagementIp.Ip, updatedMachine.Network.ManagementIp.Ip)
		assert.Equal(t, originalNetwork.PublicKey, updatedMachine.Network.PublicKey)
	})

	t.Run("set multiple fields simultaneously", func(t *testing.T) {
		// Get a machine to update
		machines, err := cli.ListMachines(ctx, nil)
		require.NoError(t, err)

		var targetMachine *pb.MachineMember
		for _, m := range machines {
			if m.Machine.Name != "updated-machine-name" {
				targetMachine = m
				break
			}
		}
		require.NotNil(t, targetMachine)

		// Set both name and public IP
		newName := "multi-update-machine"
		newPublicIP := &pb.IP{
			Ip: []byte{1, 1, 1, 1},
		}

		req := &pb.SetMachineRequest{
			MachineId: targetMachine.Machine.Id,
			Name:      &newName,
			PublicIp:  newPublicIP,
		}
		updatedMachine, err := cli.SetMachine(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, newName, updatedMachine.Name)
		assert.Equal(t, newPublicIP.Ip, updatedMachine.PublicIp.Ip)

		// Verify both changes persisted
		inspected, err := cli.InspectMachine(ctx, updatedMachine.Id)
		require.NoError(t, err)
		assert.Equal(t, newName, inspected.Machine.Name)
		assert.Equal(t, newPublicIP.Ip, inspected.Machine.PublicIp.Ip)
	})

	t.Run("set non-existent machine", func(t *testing.T) {
		// Try to set properties on a machine that doesn't exist
		nonExistentName := "should-be-updated"
		req := &pb.SetMachineRequest{
			MachineId: "non-existent-machine-id",
			Name:      &nonExistentName,
		}
		_, err := cli.SetMachine(ctx, req)
		assert.Error(t, err)
	})

	t.Run("set to duplicate name", func(t *testing.T) {
		// Get two machines
		machines, err := cli.ListMachines(ctx, nil)
		require.NoError(t, err)
		require.Len(t, machines, 3)

		machine1 := machines[0]
		machine2 := machines[1]

		// Try to set machine2 with machine1's name
		req := &pb.SetMachineRequest{
			MachineId: machine2.Machine.Id,
			Name:      &machine1.Machine.Name,
		}
		_, err = cli.SetMachine(ctx, req)
		assert.Error(t, err)
	})

	t.Run("set with empty request", func(t *testing.T) {
		// Get a machine
		machines, err := cli.ListMachines(ctx, nil)
		require.NoError(t, err)

		targetMachine := machines[0]

		// Set with no fields set (should be a no-op)
		req := &pb.SetMachineRequest{
			MachineId: targetMachine.Machine.Id,
		}
		updatedMachine, err := cli.SetMachine(ctx, req)
		require.NoError(t, err)

		// Machine should remain unchanged
		assert.Equal(t, targetMachine.Machine.Name, updatedMachine.Name)
		if targetMachine.Machine.PublicIp != nil && updatedMachine.PublicIp != nil {
			assert.Equal(t, targetMachine.Machine.PublicIp.Ip, updatedMachine.PublicIp.Ip)
		}
		assert.Equal(t, len(targetMachine.Machine.Network.Endpoints), len(updatedMachine.Network.Endpoints))
	})
}
