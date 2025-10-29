# Internal DNS

Services can be addressed on the internal WireGuard network by service name, service ID, or a machine-scoped service name:

## Service name
```
$ nslookup nats.internal
Server:         127.0.0.11
Address:        127.0.0.11#53

Name:   nats.internal
Address: 10.210.0.2
Name:   nats.internal
Address: 10.210.1.2
```

```
$ nslookup worker.internal
Server:         127.0.0.11
Address:        127.0.0.11#53

Name:   worker.internal
Address: 10.210.0.3
Name:   worker.internal
Address: 10.210.0.4
Name:   worker.internal
Address: 10.210.1.3
Name:   worker.internal
Address: 10.210.1.4
```

## Service ID
```
$ nslookup 3ecb3a8bbec5fd3f46efb056a934714a.internal
Server:         127.0.0.11
Address:        127.0.0.11#53

Name:   3ecb3a8bbec5fd3f46efb056a934714a.internal
Address: 10.210.0.4
```

## Machine ID scoped service name
```
$ nslookup 0903f0ee483aa97d559eeeaac5e22283.m.nats.internal
Server:         127.0.0.11
Address:        127.0.0.11#53

Name:   nats.internal
Address: 10.210.1.2
```

```
$ nslookup 0903f0ee483aa97d559eeeaac5e22283.m.worker.internal
Server:         127.0.0.11
Address:        127.0.0.11#53

Name:   worker.internal
Address: 10.210.1.3
Name:   worker.internal
Address: 10.210.1.4
```

## IP Ordering Mode

Additionally, the IP ordering preference can be specified with a `rr` (round-robin) or `nearest` subdomain prefix.

### `rr` (round-robin) *current default*
Randomly shuffled order on each lookup.

```
$ nslookup rr.worker.internal
Server:         127.0.0.11
Address:        127.0.0.11#53

Name:   rr.worker.internal
Address: 10.210.0.3
Name:   rr.worker.internal
Address: 10.210.0.4
Name:   rr.worker.internal
Address: 10.210.1.3
Name:   rr.worker.internal
Address: 10.210.1.4

$ nslookup rr.worker.internal
Server:         127.0.0.11
Address:        127.0.0.11#53

Name:   rr.worker.internal
Address: 10.210.0.4
Name:   rr.worker.internal
Address: 10.210.1.3
Name:   rr.worker.internal
Address: 10.210.1.4
Name:   rr.worker.internal
Address: 10.210.0.3
```

## Nearest scope
Returns machine-local instances first.

`machine-a`:
```
$ nslookup nearest.worker.internal
Server:         127.0.0.11
Address:        127.0.0.11#53

Name:   nearest.worker.internal
Address: 10.210.0.3
Name:   nearest.worker.internal
Address: 10.210.0.4
Name:   nearest.worker.internal
Address: 10.210.1.3
Name:   nearest.worker.internal
Address: 10.210.1.4
```

`machine-b`:
```
$ nslookup nearest.worker.internal
Server:         127.0.0.11
Address:        127.0.0.11#53

Name:   nearest.worker.internal
Address: 10.210.1.3
Name:   nearest.worker.internal
Address: 10.210.1.4
Name:   nearest.worker.internal
Address: 10.210.0.3
Name:   nearest.worker.internal
Address: 10.210.0.4
```

The prefixes can be used with service ID and machine-scoped service names, as well (e.g. `nearest.3ecb3a8bbec5fd3f46efb056a934714a.internal` or `rr.0903f0ee483aa97d559eeeaac5e22283.m.worker.internal`).
