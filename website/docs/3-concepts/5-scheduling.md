# Scheduling

How Uncloud decides where to run your containers, and what to do when resources or constraints get in the way.

## Overview

- One container is placed at a time on the "best" eligible machine.
- Eligibility comes from constraints (machines, volumes, resources).
- Ranking depends on whether you request CPU/memory reservations:
  - **With reservations:** prefer machines with fewer total containers (existing + already scheduled in this plan).
  - **Without reservations:** round-robin across eligible machines (creating a HA setup), ignoring existing containers to keep the spread even.

## Eligibility checks

Before each placement Uncloud filters machines:

- **`x-machines`:** only listed machines are allowed.
- **Volumes:** required Docker volumes must exist on, or be scheduled for, the machine.
- **Resources:** if CPU/memory reservations are set, the machine must have enough available capacity.

If a machine fails any constraint mid-plan (for example, runs out of CPU), it drops out for the remaining placements.

## Resource reservations

When you set reservations, Uncloud only places a container on machines with enough headroom:

```
available = total - reserved_by_running - reserved_by_containers_already_scheduled_in_this_plan
```

Example:

```yaml
services:
  api:
    image: myapp:latest
    deploy:
      replicas: 3
      resources:
        reservations:
          cpus: '0.5'
          memory: 512M
```

If a machine runs out partway through scheduling, remaining replicas are placed on other eligible machines.

## Service modes

- **Replicated (`deploy.mode: replicated`):** run `replicas` containers across eligible machines. If some machines are ineligible, others are still used.
- **Global (`deploy.mode: global`):** run exactly one container on every eligible machine. If any machine is ineligible, the deployment fails.

## Volumes

Services that mount named Docker volumes can only run on machines where those volumes exist or are scheduled to be created. Volume constraints are applied together with other placement rules.

## Port conflicts and replacements

- If a running container conflicts with requested host ports, Uncloud stops it before starting the new one and removes it afterward.
- `force_recreate` replaces containers even when they already match the requested spec.

## Troubleshooting

- **"No eligible machines":** check `x-machines`, required volumes on target machines, and CPU/memory reservations versus capacity.
- **Uneven spread:** happens when some machines are ineligible (constraints or capacity) or already host more containers; otherwise the ranker evens out placements.
- **Reservations ignored?:** reservations are opt-in; set `resources.reservations` to make capacity part of eligibility and ranking.
