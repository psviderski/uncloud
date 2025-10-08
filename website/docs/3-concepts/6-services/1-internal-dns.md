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
